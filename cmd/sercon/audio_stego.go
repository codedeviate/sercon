// cmd/sercon/audio_stego.go
package main

import (
	"encoding/binary"
	"fmt"
	"os"

	"github.com/dop251/goja"
)

// wavInfo describes the PCM data chunk located in a WAV file.
type wavInfo struct {
	bitsPerSample int
	dataStart     int // byte offset of the data chunk payload
	dataLen       int
}

func (w wavInfo) numSamples() int { return w.dataLen / (w.bitsPerSample / 8) }

// sampleByteIndex returns the file byte index of sample i's LSB carrier byte
// (16-bit little-endian → the low byte; 8-bit → the sample byte).
func (w wavInfo) sampleByteIndex(i int) int {
	return w.dataStart + i*(w.bitsPerSample/8)
}

// parseWAV validates a RIFF/WAVE PCM file and locates its data chunk.
func parseWAV(data []byte) (wavInfo, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return wavInfo{}, fmt.Errorf("not a RIFF/WAVE file")
	}
	bits := -1
	dataStart, dataLen := -1, 0
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		sz := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if body+sz > len(data) {
			sz = len(data) - body // tolerate a truncated final chunk
		}
		switch id {
		case "fmt ":
			if sz < 16 {
				return wavInfo{}, fmt.Errorf("malformed fmt chunk")
			}
			if af := binary.LittleEndian.Uint16(data[body : body+2]); af != 1 {
				return wavInfo{}, fmt.Errorf("unsupported WAV encoding %d (only PCM)", af)
			}
			bits = int(binary.LittleEndian.Uint16(data[body+14 : body+16]))
		case "data":
			dataStart, dataLen = body, sz
		}
		pos = body + sz
		if sz%2 == 1 {
			pos++ // chunks are word-aligned
		}
	}
	if bits != 8 && bits != 16 {
		return wavInfo{}, fmt.Errorf("unsupported bit depth %d (only 8/16-bit PCM)", bits)
	}
	if dataStart < 0 || dataLen == 0 {
		return wavInfo{}, fmt.Errorf("no PCM data chunk")
	}
	return wavInfo{bitsPerSample: bits, dataStart: dataStart, dataLen: dataLen}, nil
}

// audioCarrier builds an lsbCarrier over the WAV's PCM sample LSB-carrier bytes,
// writing into dst (a copy of the cover, or the cover itself for read-only).
func audioCarrier(w wavInfo, dst []byte) lsbCarrier {
	return lsbCarrier{pix: dst, count: w.numSamples(), at: w.sampleByteIndex}
}

func audioCapacity(cover []byte, bits int) (int, error) {
	w, err := parseWAV(cover)
	if err != nil {
		return 0, err
	}
	return lsbCapacityBytes(w.numSamples(), bits), nil
}

// audioStegoEmbed writes the payload into the WAV's sample LSBs at the given bit
// depth and returns a new WAV (the input is not mutated). The header is always
// embedded at 1 bit/sample.
func audioStegoEmbed(cover, payload []byte, isText bool, password string, bits int) ([]byte, error) {
	w, err := parseWAV(cover)
	if err != nil {
		return nil, fmt.Errorf("audio.stego.embed: %w", err)
	}
	stream, err := stegoEncodePayload(payload, isText, password, bits)
	if err != nil {
		return nil, fmt.Errorf("audio.stego.embed: %w", err)
	}
	blob := stream[stegoHeaderLen:]
	if avail := lsbCapacityBytes(w.numSamples(), bits); len(blob) > avail {
		return nil, fmt.Errorf("audio.stego.embed: payload too large (need %d bytes, capacity %d)", len(blob), avail)
	}
	out := make([]byte, len(cover))
	copy(out, cover)
	c := audioCarrier(w, out)
	if werr := lsbWriteStream(c, stream[:stegoHeaderLen], blob, bits); werr != nil {
		return nil, fmt.Errorf("audio.stego.embed: %w", werr)
	}
	return out, nil
}

// audioStegoExtract recovers a payload embedded by audioStegoEmbed. The bit
// depth is read from the header.
func audioStegoExtract(cover []byte, password string) ([]byte, bool, error) {
	w, err := parseWAV(cover)
	if err != nil {
		return nil, false, fmt.Errorf("audio.stego.extract: %w", err)
	}
	c := audioCarrier(w, cover)
	data, isText, err := stegoDecodeStream(func(n int) ([]byte, error) {
		return lsbReadStream(c, n)
	}, password)
	if err != nil {
		return nil, false, fmt.Errorf("audio.stego.extract: %w", err)
	}
	return data, isText, nil
}

// audioStegoNamespace returns the audio.stego sub-namespace.
func audioStegoNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"embed": func(call goja.FunctionCall) goja.Value {
			cover := stegoSrcBytes(vm, call.Argument(0), "audio.stego.embed")
			payload, isText := stegoPayloadArg(vm, call.Argument(1), "audio.stego.embed")
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
						panic(vm.NewTypeError("audio.stego.embed: bits must be an integer 1..4"))
					}
					bits = int(n)
				}
			}
			out, err := audioStegoEmbed(cover, payload, isText, password, bits)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			if dest != "" {
				if werr := os.WriteFile(dest, out, 0o644); werr != nil { //nolint:gosec
					panic(vm.NewGoError(fmt.Errorf("audio.stego.embed: %w", werr)))
				}
				return vm.ToValue(map[string]any{"path": dest})
			}
			return vm.ToValue(map[string]any{"bytes": out})
		},
		"extract": func(call goja.FunctionCall) goja.Value {
			cover := stegoSrcBytes(vm, call.Argument(0), "audio.stego.extract")
			password := ""
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				if p := o.ToObject(vm).Get("password"); p != nil && !goja.IsUndefined(p) {
					password = p.String()
				}
			}
			data, isText, err := audioStegoExtract(cover, password)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			if isText {
				return vm.ToValue(string(data))
			}
			return vm.ToValue(data)
		},
		"capacity": func(call goja.FunctionCall) goja.Value {
			cover := stegoSrcBytes(vm, call.Argument(0), "audio.stego.capacity")
			bits := 1
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				if bv := o.ToObject(vm).Get("bits"); bv != nil && !goja.IsUndefined(bv) {
					n := bv.ToInteger()
					if float64(n) != bv.ToFloat() || n < 1 || n > 4 {
						panic(vm.NewTypeError("audio.stego.capacity: bits must be an integer 1..4"))
					}
					bits = int(n)
				}
			}
			n, err := audioCapacity(cover, bits)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(map[string]any{"bytes": n, "bits": bits})
		},
	}
}
