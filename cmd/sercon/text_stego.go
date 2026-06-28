// cmd/sercon/text_stego.go
package main

import (
	"fmt"

	"github.com/dop251/goja"
)

const (
	zwZero = '​' // zero-width space      → bit 0
	zwOne  = '‌' // zero-width non-joiner → bit 1
)

// stegoTextEncode renders a byte stream as a run of zero-width runes (MSB-first
// per byte).
func stegoTextEncode(stream []byte) string {
	out := make([]rune, 0, len(stream)*8)
	for _, by := range stream {
		for j := 0; j < 8; j++ {
			if (by>>(7-j))&1 == 1 {
				out = append(out, zwOne)
			} else {
				out = append(out, zwZero)
			}
		}
	}
	return string(out)
}

// stegoTextDecodeBytes scans s for the zero-width carrier runes (ignoring all
// other runes) and reconstructs the byte stream (whole bytes only).
func stegoTextDecodeBytes(s string) []byte {
	bits := make([]byte, 0, len(s))
	for _, r := range s {
		switch r {
		case zwZero:
			bits = append(bits, 0)
		case zwOne:
			bits = append(bits, 1)
		}
	}
	out := make([]byte, len(bits)/8)
	for i := range out {
		var by byte
		for j := 0; j < 8; j++ {
			by = (by << 1) | bits[i*8+j]
		}
		out[i] = by
	}
	return out
}

// textStegoEmbed appends payload as an invisible zero-width run to cover.
func textStegoEmbed(cover string, payload []byte, isText bool, password string) (string, error) {
	stream, err := stegoEncodePayload(payload, isText, password)
	if err != nil {
		return "", fmt.Errorf("text.stego.embed: %w", err)
	}
	return cover + stegoTextEncode(stream), nil
}

// textStegoExtract recovers a payload hidden by textStegoEmbed.
func textStegoExtract(stego, password string) ([]byte, bool, error) {
	all := stegoTextDecodeBytes(stego)
	if len(all) < stegoHeaderLen {
		return nil, false, fmt.Errorf("text.stego.extract: no sercon stego payload found")
	}
	data, isText, err := stegoDecodeStream(func(n int) ([]byte, error) {
		if n > len(all) {
			return nil, fmt.Errorf("not enough hidden data")
		}
		return all[:n], nil
	}, password)
	if err != nil {
		return nil, false, fmt.Errorf("text.stego.extract: %w", err)
	}
	return data, isText, nil
}

// textStegoNamespace returns the text.stego sub-namespace.
func textStegoNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"embed": func(call goja.FunctionCall) goja.Value {
			cover := call.Argument(0).String()
			payload, isText := stegoPayloadArg(vm, call.Argument(1), "text.stego.embed")
			password := ""
			if o := call.Argument(2); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				if p := o.ToObject(vm).Get("password"); p != nil && !goja.IsUndefined(p) {
					password = p.String()
				}
			}
			out, err := textStegoEmbed(cover, payload, isText, password)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(out)
		},
		"extract": func(call goja.FunctionCall) goja.Value {
			stego := call.Argument(0).String()
			password := ""
			if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
				if p := o.ToObject(vm).Get("password"); p != nil && !goja.IsUndefined(p) {
					password = p.String()
				}
			}
			data, isText, err := textStegoExtract(stego, password)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			if isText {
				return vm.ToValue(string(data))
			}
			return vm.ToValue(data) // []byte → Uint8Array
		},
	}
}
