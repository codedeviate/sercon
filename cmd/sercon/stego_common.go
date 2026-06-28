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
// n is the payload bit-depth (1..4); it is packed into flags bits 2–4 as (n-1).
func stegoEncodePayload(payload []byte, isText bool, password string, n int) ([]byte, error) {
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
	flags |= byte(n-1) << flagBitsShift
	return append(marshalStegoHeader(flags, uint32(len(blob))), blob...), nil
}

// headerUnits is the number of carrier units the fixed header occupies when
// written at 1 bit/unit (10 bytes × 8 bits).
const headerUnits = stegoHeaderLen * 8

// lsbCarrier abstracts a carrier as an addressable sequence of `count` units
// whose low bits carry data; at(k) maps unit k to its byte index in pix.
type lsbCarrier struct {
	pix   []byte
	count int
	at    func(k int) int
}

// lsbCapacityBytes is the payload byte capacity of a carrier of `count` units
// at n bits per unit, after the fixed header. Never negative.
func lsbCapacityBytes(count, n int) int {
	avail := count - headerUnits
	if avail < 0 {
		return 0
	}
	return (avail * n) / 8
}

// lsbWriteStream writes header at 1 bit/unit (units 0..headerUnits-1), then
// payload at n bits/unit immediately after, MSB-first. Errors if the carrier
// has too few units.
func lsbWriteStream(c lsbCarrier, header, payload []byte, n int) error {
	payUnits := (len(payload)*8 + n - 1) / n
	if headerUnits+payUnits > c.count {
		return fmt.Errorf("stego: stream exceeds carrier capacity (%d units needed, %d available)", headerUnits+payUnits, c.count)
	}
	// Header: 1 bit per unit, MSB-first.
	for i, b := range header {
		for j := 0; j < 8; j++ {
			bit := (b >> (7 - j)) & 1
			idx := c.at(i*8 + j)
			c.pix[idx] = (c.pix[idx] &^ 1) | bit
		}
	}
	// Payload: n bits per unit. The first payload bit becomes the high bit of
	// each n-bit group; a trailing partial group is zero-padded in its low bits.
	mask := byte((1 << n) - 1)
	total := len(payload) * 8
	bitPos := 0
	unit := headerUnits
	for bitPos < total {
		var group byte
		for k := 0; k < n; k++ {
			var bit byte
			if bitPos < total {
				bit = (payload[bitPos/8] >> (7 - (bitPos % 8))) & 1
				bitPos++
			}
			group = (group << 1) | bit
		}
		idx := c.at(unit)
		c.pix[idx] = (c.pix[idx] &^ mask) | group
		unit++
	}
	return nil
}

// lsbReadStream returns the first byteCount bytes of the logical stream: bytes
// within the header region are read at 1 bit/unit; bytes beyond it at n bits/
// unit, where n is parsed from the header's flags byte (self-describing, so the
// caller need not know n). Errors if the carrier has too few units.
func lsbReadStream(c lsbCarrier, byteCount int) ([]byte, error) {
	if byteCount <= 0 {
		return nil, nil
	}
	if headerUnits > c.count {
		return nil, fmt.Errorf("stego: carrier too small for header")
	}
	hdr := make([]byte, stegoHeaderLen)
	for i := 0; i < stegoHeaderLen; i++ {
		var b byte
		for j := 0; j < 8; j++ {
			b = (b << 1) | (c.pix[c.at(i*8+j)] & 1)
		}
		hdr[i] = b
	}
	if byteCount <= stegoHeaderLen {
		return hdr[:byteCount], nil
	}
	n := int((hdr[5]&flagBitsMask)>>flagBitsShift) + 1
	payBytes := byteCount - stegoHeaderLen
	total := payBytes * 8
	payUnits := (total + n - 1) / n
	if headerUnits+payUnits > c.count {
		return nil, fmt.Errorf("stego: not enough carrier units for %d payload bytes", payBytes)
	}
	out := make([]byte, byteCount)
	copy(out, hdr)
	payload := out[stegoHeaderLen:]
	mask := byte((1 << n) - 1)
	bitPos := 0
	unit := headerUnits
	for bitPos < total {
		group := c.pix[c.at(unit)] & mask
		unit++
		for k := n - 1; k >= 0 && bitPos < total; k-- {
			bit := (group >> k) & 1
			payload[bitPos/8] |= bit << (7 - (bitPos % 8))
			bitPos++
		}
	}
	return out, nil
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
