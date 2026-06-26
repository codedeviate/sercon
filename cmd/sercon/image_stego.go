// cmd/sercon/image_stego.go
package main

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/draw"
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
