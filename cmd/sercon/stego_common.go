// cmd/sercon/stego_common.go
package main

import (
	"fmt"
	"os"

	"github.com/dop251/goja"
)

// stegoEncodePayload builds the carrier-agnostic byte stream: a 10-byte ScSt
// header followed by the blob. blob = payload (plaintext) or AES-256-GCM(payload)
// when password != "". flagText marks UTF-8 text; flagEncrypted marks sealed.
func stegoEncodePayload(payload []byte, isText bool, password string) ([]byte, error) {
	flags := byte(0)
	blob := payload
	if isText {
		flags |= flagText
	}
	if password != "" {
		sealed, err := stegoSeal(password, payload)
		if err != nil {
			return nil, err
		}
		blob = sealed
		flags |= flagEncrypted
	}
	return append(marshalStegoHeader(flags, uint32(len(blob))), blob...), nil
}

// stegoDecodeStream reads a stream via readN (which must return the first n
// bytes of the embedded stream, or an error if fewer are available): the
// header (readN(10)), then the full stream (readN(10+length)). It parses the
// header, slices the blob, and decrypts if flagged. Errors are unprefixed —
// each carrier wraps them with its own "<ns>.stego.<op>:" prefix.
func stegoDecodeStream(readN func(n int) ([]byte, error), password string) ([]byte, bool, error) {
	header, err := readN(stegoHeaderLen)
	if err != nil {
		return nil, false, err
	}
	flags, length, err := parseStegoHeader(header)
	if err != nil {
		return nil, false, err
	}
	all, err := readN(stegoHeaderLen + int(length))
	if err != nil {
		return nil, false, fmt.Errorf("truncated payload: %w", err)
	}
	blob := all[stegoHeaderLen:]
	if flags&flagEncrypted != 0 {
		if password == "" {
			return nil, false, fmt.Errorf("payload is encrypted — password required")
		}
		pt, oerr := stegoOpen(password, blob)
		if oerr != nil {
			return nil, false, oerr
		}
		blob = pt
	}
	return blob, flags&flagText != 0, nil
}

// stegoPayloadArg coerces a payload argument: a JS string → UTF-8 bytes (text),
// a Uint8Array → raw bytes (binary). op is the binding name for the error
// message (e.g. "text.stego.embed").
func stegoPayloadArg(vm *goja.Runtime, arg goja.Value, op string) ([]byte, bool) {
	switch v := arg.Export().(type) {
	case string:
		return []byte(v), true
	case []byte:
		return v, false
	default:
		panic(vm.NewTypeError(op + ": payload must be a string or Uint8Array"))
	}
}

// stegoSrcBytes reads a carrier argument that is a path string or a Uint8Array
// into bytes. op is the binding name for error messages.
func stegoSrcBytes(vm *goja.Runtime, arg goja.Value, op string) []byte {
	if s, ok := arg.Export().(string); ok {
		b, err := os.ReadFile(s) //nolint:gosec // user-provided path is intentional
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("%s: %w", op, err)))
		}
		return b
	}
	if b, ok := arg.Export().([]byte); ok {
		return b
	}
	panic(vm.NewTypeError(op + ": expected a path string or Uint8Array"))
}
