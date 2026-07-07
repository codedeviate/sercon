// cmd/sercon/audio_format_8bit_test.go
package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// make8bitWAV builds a minimal mono 8-bit PCM WAV. 8-bit WAV samples are
// UNSIGNED bytes (0x80 == silence), per the WAV spec.
func make8bitWAV(samples []byte) []byte {
	var b bytes.Buffer
	le := func(v uint32) { _ = binary.Write(&b, binary.LittleEndian, v) }
	le16 := func(v uint16) { _ = binary.Write(&b, binary.LittleEndian, v) }
	dataLen := uint32(len(samples))
	b.WriteString("RIFF")
	le(36 + dataLen)
	b.WriteString("WAVE")
	b.WriteString("fmt ")
	le(16)   // PCM fmt chunk size
	le16(1)  // audioFormat = PCM
	le16(1)  // channels = 1
	le(8000) // sample rate
	le(8000) // byte rate = rate * channels * bytesPerSample
	le16(1)  // block align
	le16(8)  // bits per sample
	b.WriteString("data")
	le(dataLen)
	b.Write(samples)
	return b.Bytes()
}

// An 8-bit WAV of pure silence (all 0x80) must decode to ~0, not full-scale
// negative. go-audio returns 8-bit WAV PCM as unsigned 0..255; treating it
// as signed (v<<8) wraps every sample >= 128, so silence becomes -32768 and
// audio.convert produces severely distorted output.
func TestDecodeWAV_8bitUnsignedSilenceCentersToZero(t *testing.T) {
	wavBytes := make8bitWAV([]byte{0x80, 0x80, 0x80, 0x80})
	pcm, err := decodeWAV(wavBytes)
	if err != nil {
		t.Fatalf("decodeWAV: %v", err)
	}
	if len(pcm.samples) == 0 {
		t.Fatal("no samples decoded")
	}
	for i, s := range pcm.samples {
		if s != 0 {
			t.Fatalf("sample %d = %d, want 0 (8-bit unsigned silence 0x80 must center to 0)", i, s)
		}
	}
}

// Full-scale 8-bit unsigned extremes map to the 16-bit signed extremes.
func TestDecodeWAV_8bitUnsignedExtremes(t *testing.T) {
	pcm, err := decodeWAV(make8bitWAV([]byte{0x00, 0xFF}))
	if err != nil {
		t.Fatalf("decodeWAV: %v", err)
	}
	if pcm.samples[0] != -128<<8 {
		t.Errorf("0x00 -> %d, want %d", pcm.samples[0], int16(-128<<8))
	}
	if pcm.samples[1] != 127<<8 {
		t.Errorf("0xFF -> %d, want %d", pcm.samples[1], int16(127<<8))
	}
}
