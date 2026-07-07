// cmd/sercon/image_stego.go
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
	"os"

	"github.com/dop251/goja"
)

const (
	stegoMagic     = "ScSt"
	stegoHeaderLen = 10 // magic(4) + version(1) + flags(1) + length(4)
	stegoVersion   = byte(1)

	flagEncrypted = byte(1 << 0)
	flagText      = byte(1 << 1)

	// flags bits 2–4 hold (N − 1), the payload bit-depth (1..4). Legacy
	// headers have these bits zero, decoding as N=1.
	flagBitsShift = 2
	flagBitsMask  = byte(0x1C) // bits 2,3,4
)

// toRGBA returns img as an *image.RGBA. If img already is one it is returned
// as-is (callers only mutate freshly-decoded images); otherwise it is copied.
func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	r := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(r, r.Bounds(), img, b.Min, draw.Src)
	return r
}

// stegoCapacity is the payload byte capacity of img at n bits per R,G,B channel
// (alpha excluded), after the fixed header.
func stegoCapacity(img image.Image, n int) int {
	b := img.Bounds()
	return lsbCapacityBytes(b.Dx()*b.Dy()*3, n)
}

// pixChannelIndex maps the k-th usable channel (0-based over R,G,B, skipping
// alpha) to its byte index in rgba.Pix (which is R,G,B,A per pixel).
func pixChannelIndex(k int) int { return (k/3)*4 + (k % 3) }

// imageCarrier builds an lsbCarrier over rgba's R,G,B channels.
func imageCarrier(rgba *image.RGBA) lsbCarrier {
	return lsbCarrier{pix: rgba.Pix, count: len(rgba.Pix) / 4 * 3, at: pixChannelIndex}
}

// marshalStegoHeader builds the fixed 10-byte header.
func marshalStegoHeader(flags byte, length uint32) []byte {
	h := make([]byte, stegoHeaderLen)
	copy(h[0:4], stegoMagic)
	h[4] = stegoVersion
	h[5] = flags
	binary.BigEndian.PutUint32(h[6:10], length)
	return h
}

// parseStegoHeader validates the magic + version and returns flags and length.
func parseStegoHeader(b []byte) (flags byte, length uint32, err error) {
	if len(b) < stegoHeaderLen {
		return 0, 0, fmt.Errorf("stego: header too short")
	}
	if string(b[0:4]) != stegoMagic {
		return 0, 0, fmt.Errorf("no sercon stego payload found")
	}
	if b[4] != stegoVersion {
		return 0, 0, fmt.Errorf("stego: unsupported version %d", b[4])
	}
	return b[5], binary.BigEndian.Uint32(b[6:10]), nil
}

const (
	stegoPBKDF2Iter = 210000
	stegoSaltLen    = 16
	stegoNonceLen   = 12
	stegoKeyLen     = 32 // AES-256
)

// stegoDeriveKey derives a 32-byte AES key from password + salt via PBKDF2.
func stegoDeriveKey(password string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, stegoPBKDF2Iter, stegoKeyLen)
}

// stegoSeal encrypts plaintext with AES-256-GCM, returning salt‖nonce‖sealed.
func stegoSeal(password string, plaintext []byte) ([]byte, error) {
	salt := make([]byte, stegoSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	key, err := stegoDeriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, stegoNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, stegoSaltLen+stegoNonceLen+len(sealed))
	out = append(out, salt...)
	out = append(out, nonce...)
	out = append(out, sealed...)
	return out, nil
}

// stegoOpen reverses stegoSeal. A wrong password fails the GCM auth check.
func stegoOpen(password string, blob []byte) ([]byte, error) {
	if len(blob) < stegoSaltLen+stegoNonceLen {
		return nil, fmt.Errorf("decryption failed (wrong password or corrupt data)")
	}
	salt := blob[:stegoSaltLen]
	nonce := blob[stegoSaltLen : stegoSaltLen+stegoNonceLen]
	ct := blob[stegoSaltLen+stegoNonceLen:]
	key, err := stegoDeriveKey(password, salt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed (wrong password or corrupt data)")
	}
	return pt, nil
}

// stegoEmbed hides payload in carrier and returns PNG bytes. isText marks the
// payload as UTF-8 text. A non-empty password triggers AES-256-GCM. bits is the
// payload depth (1..4); the header is always embedded at 1 bit/channel.
func stegoEmbed(carrier, payload []byte, isText bool, password string, bits int) ([]byte, error) {
	img, _, err := decodeImage(carrier)
	if err != nil {
		return nil, err
	}
	rgba := toRGBA(img)
	stream, err := stegoEncodePayload(payload, isText, password, bits)
	if err != nil {
		return nil, fmt.Errorf("image.stego.embed: %w", err)
	}
	blob := stream[stegoHeaderLen:]
	c := imageCarrier(rgba)
	if avail := lsbCapacityBytes(c.count, bits); len(blob) > avail {
		return nil, fmt.Errorf("image.stego.embed: payload too large (need %d bytes, capacity %d)", len(blob), avail)
	}
	if werr := lsbWriteStream(c, stream[:stegoHeaderLen], blob, bits); werr != nil {
		return nil, fmt.Errorf("image.stego.embed: %w", werr)
	}
	out, err := encodeImage(rgba, "png", encodeOpts{})
	if err != nil {
		return nil, fmt.Errorf("image.stego.embed: %w", err)
	}
	return out, nil
}

// stegoExtract recovers a payload previously embedded by stegoEmbed. The bit
// depth is read from the header, so no depth argument is needed.
func stegoExtract(carrier []byte, password string) ([]byte, bool, error) {
	img, _, err := decodeImage(carrier)
	if err != nil {
		return nil, false, err
	}
	c := imageCarrier(toRGBA(img))
	data, isText, err := stegoDecodeStream(func(n int) ([]byte, error) {
		return lsbReadStream(c, n)
	}, password)
	if err != nil {
		return nil, false, fmt.Errorf("image.stego.extract: %w", err)
	}
	return data, isText, nil
}

// stegoCapacityOf decodes carrier and returns its payload capacity in bytes at
// the given bit depth.
func stegoCapacityOf(carrier []byte, bits int) (int, error) {
	img, _, err := decodeImage(carrier)
	if err != nil {
		return 0, err
	}
	return stegoCapacity(img, bits), nil
}

// stegoNamespace returns the image.stego sub-namespace.
func stegoNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"embed": func(call goja.FunctionCall) goja.Value {
			carrier := imageSrcBytes(vm, call.Argument(0), "stego.embed")
			payload, isText := stegoPayloadArg(vm, call.Argument(1), "image.stego.embed")

			password, dest, bits := "", "", 1
			if o := call.Argument(2); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				obj := o.ToObject(vm)
				if p := obj.Get("password"); p != nil && !goja.IsUndefined(p) {
					password = p.String()
				}
				if d := obj.Get("dest"); d != nil && !goja.IsUndefined(d) {
					dest = d.String()
				}
				if bv := obj.Get("bits"); bv != nil && !goja.IsUndefined(bv) {
					n := bv.ToInteger()
					if float64(n) != bv.ToFloat() || n < 1 || n > 4 {
						panic(vm.NewTypeError("image.stego.embed: bits must be an integer 1..4"))
					}
					bits = int(n)
				}
			}
			out, err := stegoEmbed(carrier, payload, isText, password, bits)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			if dest != "" {
				if werr := os.WriteFile(dest, out, 0o644); werr != nil { //nolint:gosec
					panic(vm.NewGoError(fmt.Errorf("image.stego.embed: %w", werr)))
				}
				return vm.ToValue(map[string]any{"path": dest})
			}
			return vm.ToValue(map[string]any{"bytes": out})
		},
		"extract": func(call goja.FunctionCall) goja.Value {
			carrier := imageSrcBytes(vm, call.Argument(0), "stego.extract")
			password := ""
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				if p := o.ToObject(vm).Get("password"); p != nil && !goja.IsUndefined(p) {
					password = p.String()
				}
			}
			data, isText, err := stegoExtract(carrier, password)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			if isText {
				return vm.ToValue(string(data))
			}
			return vm.ToValue(data) // []byte → Uint8Array
		},
		"capacity": func(call goja.FunctionCall) goja.Value {
			carrier := imageSrcBytes(vm, call.Argument(0), "stego.capacity")
			bits := 1
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				if bv := o.ToObject(vm).Get("bits"); bv != nil && !goja.IsUndefined(bv) {
					n := bv.ToInteger()
					if float64(n) != bv.ToFloat() || n < 1 || n > 4 {
						panic(vm.NewTypeError("image.stego.capacity: bits must be an integer 1..4"))
					}
					bits = int(n)
				}
			}
			n, err := stegoCapacityOf(carrier, bits)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(map[string]any{"bytes": n, "bits": bits})
		},
		"detect": func(call goja.FunctionCall) goja.Value {
			carrier := imageSrcBytes(vm, call.Argument(0), "stego.detect")
			m, err := stegoDetect(carrier)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(m)
		},
		"analyze": func(call goja.FunctionCall) goja.Value {
			carrier := imageSrcBytes(vm, call.Argument(0), "stego.analyze")
			m, err := stegoAnalyze(carrier)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(m)
		},
		"bitplane": func(call goja.FunctionCall) goja.Value {
			carrier := imageSrcBytes(vm, call.Argument(0), "stego.bitplane")
			channel, plane, dest := "rgb", 0, ""
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				obj := o.ToObject(vm)
				if cv := obj.Get("channel"); cv != nil && !goja.IsUndefined(cv) {
					channel = cv.String()
				}
				if pv := obj.Get("plane"); pv != nil && !goja.IsUndefined(pv) {
					plane = int(pv.ToInteger())
				}
				if dv := obj.Get("dest"); dv != nil && !goja.IsUndefined(dv) {
					dest = dv.String()
				}
			}
			switch channel {
			case "r", "g", "b", "rgb":
			default:
				panic(vm.NewTypeError(`image.stego.bitplane: channel must be "r", "g", "b", or "rgb"`))
			}
			if plane < 0 || plane > 7 {
				panic(vm.NewTypeError("image.stego.bitplane: plane must be 0..7"))
			}
			out, err := stegoBitplane(carrier, channel, plane)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			if dest != "" {
				if werr := os.WriteFile(dest, out, 0o644); werr != nil { //nolint:gosec
					panic(vm.NewGoError(fmt.Errorf("image.stego.bitplane: %w", werr)))
				}
				return vm.ToValue(map[string]any{"path": dest})
			}
			return vm.ToValue(map[string]any{"bytes": out})
		},
	}
}
