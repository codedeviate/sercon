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

// stegoCapacity is the maximum payload byte count for img after the header:
// one bit per R,G,B channel (alpha excluded), minus the 10-byte header.
func stegoCapacity(img image.Image) int {
	b := img.Bounds()
	total := (b.Dx() * b.Dy() * 3) / 8
	n := total - stegoHeaderLen
	if n < 0 {
		return 0
	}
	return n
}

// pixChannelIndex maps the k-th usable channel (0-based over R,G,B, skipping
// alpha) to its byte index in rgba.Pix (which is R,G,B,A per pixel).
func pixChannelIndex(k int) int { return (k/3)*4 + (k % 3) }

// writeLSBStream writes every bit of stream (MSB-first) into the LSBs of the
// R,G,B channels of rgba in order. Errors if stream needs more channels than
// the image has.
func writeLSBStream(rgba *image.RGBA, stream []byte) error {
	channels := len(rgba.Pix) / 4 * 3
	if len(stream)*8 > channels {
		return fmt.Errorf("stego: stream of %d bytes exceeds carrier capacity of %d bits", len(stream), channels)
	}
	for i, b := range stream {
		for j := 0; j < 8; j++ {
			bit := (b >> (7 - j)) & 1
			idx := pixChannelIndex(i*8 + j)
			rgba.Pix[idx] = (rgba.Pix[idx] &^ 1) | bit
		}
	}
	return nil
}

// readLSBBytes reads n bytes (MSB-first) out of the R,G,B LSBs of rgba.
func readLSBBytes(rgba *image.RGBA, n int) ([]byte, error) {
	channels := len(rgba.Pix) / 4 * 3
	if n*8 > channels {
		return nil, fmt.Errorf("stego: need %d bits but carrier only holds %d", n*8, channels)
	}
	out := make([]byte, n)
	for i := 0; i < n; i++ {
		var b byte
		for j := 0; j < 8; j++ {
			idx := pixChannelIndex(i*8 + j)
			b = (b << 1) | (rgba.Pix[idx] & 1)
		}
		out[i] = b
	}
	return out, nil
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
// payload as UTF-8 text so extract can return a string. A non-empty password
// triggers AES-256-GCM encryption.
func stegoEmbed(carrier, payload []byte, isText bool, password string) ([]byte, error) {
	img, _, err := decodeImage(carrier)
	if err != nil {
		return nil, err
	}
	rgba := toRGBA(img)

	flags := byte(0)
	blob := payload
	if isText {
		flags |= flagText
	}
	if password != "" {
		sealed, serr := stegoSeal(password, payload)
		if serr != nil {
			return nil, fmt.Errorf("image.stego.embed: %w", serr)
		}
		blob = sealed
		flags |= flagEncrypted
	}

	if avail := stegoCapacity(rgba); len(blob) > avail {
		return nil, fmt.Errorf("image.stego.embed: payload too large (need %d bytes, capacity %d)", len(blob), avail)
	}

	stream := append(marshalStegoHeader(flags, uint32(len(blob))), blob...)
	if werr := writeLSBStream(rgba, stream); werr != nil {
		return nil, fmt.Errorf("image.stego.embed: %w", werr)
	}
	out, err := encodeImage(rgba, "png", encodeOpts{})
	if err != nil {
		return nil, fmt.Errorf("image.stego.embed: %w", err)
	}
	return out, nil
}

// stegoExtract recovers a payload previously embedded by stegoEmbed.
func stegoExtract(carrier []byte, password string) ([]byte, bool, error) {
	img, _, err := decodeImage(carrier)
	if err != nil {
		return nil, false, err
	}
	rgba := toRGBA(img)

	header, err := readLSBBytes(rgba, stegoHeaderLen)
	if err != nil {
		return nil, false, fmt.Errorf("image.stego.extract: %w", err)
	}
	flags, length, err := parseStegoHeader(header)
	if err != nil {
		return nil, false, fmt.Errorf("image.stego.extract: %w", err)
	}
	all, err := readLSBBytes(rgba, stegoHeaderLen+int(length))
	if err != nil {
		return nil, false, fmt.Errorf("image.stego.extract: truncated payload: %w", err)
	}
	blob := all[stegoHeaderLen:]

	if flags&flagEncrypted != 0 {
		if password == "" {
			return nil, false, fmt.Errorf("image.stego.extract: payload is encrypted — password required")
		}
		pt, oerr := stegoOpen(password, blob)
		if oerr != nil {
			return nil, false, fmt.Errorf("image.stego.extract: %w", oerr)
		}
		blob = pt
	}
	return blob, flags&flagText != 0, nil
}

// stegoCapacityOf decodes carrier and returns its payload capacity in bytes.
func stegoCapacityOf(carrier []byte) (int, error) {
	img, _, err := decodeImage(carrier)
	if err != nil {
		return 0, err
	}
	return stegoCapacity(img), nil
}

// stegoNamespace returns the image.stego sub-namespace.
func stegoNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"embed": func(call goja.FunctionCall) goja.Value {
			carrier := imageSrcBytes(vm, call.Argument(0), "stego.embed")
			payload, isText := stegoPayloadArg(vm, call.Argument(1))

			password, dest := "", ""
			if o := call.Argument(2); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				obj := o.ToObject(vm)
				if p := obj.Get("password"); p != nil && !goja.IsUndefined(p) {
					password = p.String()
				}
				if d := obj.Get("dest"); d != nil && !goja.IsUndefined(d) {
					dest = d.String()
				}
			}
			out, err := stegoEmbed(carrier, payload, isText, password)
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
			n, err := stegoCapacityOf(carrier)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(map[string]any{"bytes": n})
		},
	}
}

// stegoPayloadArg coerces the payload argument: a JS string → UTF-8 bytes
// (text), a Uint8Array → raw bytes (binary). Anything else is a TypeError.
func stegoPayloadArg(vm *goja.Runtime, arg goja.Value) ([]byte, bool) {
	switch v := arg.Export().(type) {
	case string:
		return []byte(v), true
	case []byte:
		return v, false
	default:
		panic(vm.NewTypeError("image.stego.embed: payload must be a string or Uint8Array"))
	}
}
