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

func audioCapacity(cover []byte) (int, error) {
	w, err := parseWAV(cover)
	if err != nil {
		return 0, err
	}
	n := w.numSamples()/8 - stegoHeaderLen
	if n < 0 {
		n = 0
	}
	return n, nil
}

// audioStegoEmbed writes the payload stream into the WAV's sample LSBs and
// returns a new WAV (the input is not mutated).
func audioStegoEmbed(cover, payload []byte, isText bool, password string) ([]byte, error) {
	w, err := parseWAV(cover)
	if err != nil {
		return nil, fmt.Errorf("audio.stego.embed: %w", err)
	}
	stream, err := stegoEncodePayload(payload, isText, password)
	if err != nil {
		return nil, fmt.Errorf("audio.stego.embed: %w", err)
	}
	avail := w.numSamples()/8 - stegoHeaderLen
	if avail < 0 {
		avail = 0
	}
	if len(stream)-stegoHeaderLen > avail {
		return nil, fmt.Errorf("audio.stego.embed: payload too large (need %d bytes, capacity %d)", len(stream)-stegoHeaderLen, avail)
	}
	out := make([]byte, len(cover))
	copy(out, cover)
	bitIdx := 0
	for _, by := range stream {
		for j := 0; j < 8; j++ {
			bit := (by >> (7 - j)) & 1
			idx := w.sampleByteIndex(bitIdx)
			out[idx] = (out[idx] &^ 1) | bit
			bitIdx++
		}
	}
	return out, nil
}

// audioStegoExtract recovers a payload embedded by audioStegoEmbed.
func audioStegoExtract(cover []byte, password string) ([]byte, bool, error) {
	w, err := parseWAV(cover)
	if err != nil {
		return nil, false, fmt.Errorf("audio.stego.extract: %w", err)
	}
	readN := func(n int) ([]byte, error) {
		if n*8 > w.numSamples() {
			return nil, fmt.Errorf("not enough samples")
		}
		b := make([]byte, n)
		bitIdx := 0
		for i := 0; i < n; i++ {
			var by byte
			for j := 0; j < 8; j++ {
				by = (by << 1) | (cover[w.sampleByteIndex(bitIdx)] & 1)
				bitIdx++
			}
			b[i] = by
		}
		return b, nil
	}
	data, isText, err := stegoDecodeStream(readN, password)
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
			out, err := audioStegoEmbed(cover, payload, isText, password)
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
			n, err := audioCapacity(cover)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(map[string]any{"bytes": n})
		},
	}
}
