// cmd/sercon/image_stego_test.go
package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestStegoCapacity(t *testing.T) {
	// 10x10 → 300 channels. At 1 bit: (300-80)*1/8 = 27. At 4 bits: (300-80)*4/8 = 110.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if got := stegoCapacity(img, 1); got != 27 {
		t.Fatalf("capacity(10x10, 1) = %d, want 27", got)
	}
	if got := stegoCapacity(img, 4); got != 110 {
		t.Fatalf("capacity(10x10, 4) = %d, want 110", got)
	}
	// Tiny image (12 channels < 80-unit header) → clamps to 0.
	if got := stegoCapacity(image.NewRGBA(image.Rect(0, 0, 2, 2)), 1); got != 0 {
		t.Fatalf("capacity(2x2, 1) = %d, want 0", got)
	}
}

func TestStegoCapacityOf(t *testing.T) {
	n, err := stegoCapacityOf(stegoCarrierPNG(t, 10, 10), 1)
	if err != nil {
		t.Fatal(err)
	}
	if n != 27 {
		t.Fatalf("capacity = %d, want 27", n)
	}
}

func TestStegoEmbedExtract_MultiBit(t *testing.T) {
	carrier := stegoCarrierPNG(t, 64, 64)
	for n := 1; n <= 4; n++ {
		out, err := stegoEmbed(carrier, []byte("multi-bit secret"), true, "pw", n)
		if err != nil {
			t.Fatalf("n=%d embed: %v", n, err)
		}
		data, isText, err := stegoExtract(out, "pw") // extract auto-detects N
		if err != nil {
			t.Fatalf("n=%d extract: %v", n, err)
		}
		if !isText || string(data) != "multi-bit secret" {
			t.Fatalf("n=%d round-trip got %q isText=%v", n, data, isText)
		}
	}
}

func TestStegoEmbed_AlphaUntouched(t *testing.T) {
	carrier := stegoCarrierPNG(t, 32, 32)
	out, err := stegoEmbed(carrier, bytes.Repeat([]byte{0xFF}, 20), false, "", 4)
	if err != nil {
		t.Fatal(err)
	}
	dec, _, err := decodeImage(out)
	if err != nil {
		t.Fatal(err)
	}
	rgba := toRGBA(dec)
	for i := 3; i < len(rgba.Pix); i += 4 {
		if rgba.Pix[i] != 255 {
			t.Fatalf("alpha byte at %d = %d, want 255 (untouched)", i, rgba.Pix[i])
		}
	}
}

func TestStegoExtract_LegacyOneBitLayout(t *testing.T) {
	// Simulate the pre-multibit writer: the whole stream (header+blob) written
	// at 1 bit/channel sequentially. New extract must read it (backward compat).
	carrier := stegoCarrierPNG(t, 64, 64)
	img, _, err := decodeImage(carrier)
	if err != nil {
		t.Fatal(err)
	}
	rgba := toRGBA(img)
	stream, _ := stegoEncodePayload([]byte("legacy"), true, "", 1)
	for i, b := range stream {
		for j := 0; j < 8; j++ {
			bit := (b >> (7 - j)) & 1
			idx := pixChannelIndex(i*8 + j)
			rgba.Pix[idx] = (rgba.Pix[idx] &^ 1) | bit
		}
	}
	out, err := encodeImage(rgba, "png", encodeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	data, isText, err := stegoExtract(out, "")
	if err != nil || !isText || string(data) != "legacy" {
		t.Fatalf("legacy layout extract failed: %q isText=%v err=%v", data, isText, err)
	}
}

func TestStegoEmbed_TooLargeMultiBit(t *testing.T) {
	carrier := stegoCarrierPNG(t, 8, 8) // 192 channels; at 1 bit cap = (192-80)/8 = 14
	if _, err := stegoEmbed(carrier, bytes.Repeat([]byte{1}, 15), false, "", 1); err == nil {
		t.Fatal("expected payload-too-large at 1 bit")
	}
	// At 4 bits the same carrier holds (192-80)*4/8 = 56 bytes, so 15 fits.
	if _, err := stegoEmbed(carrier, bytes.Repeat([]byte{1}, 15), false, "", 4); err != nil {
		t.Fatalf("15 bytes should fit at 4 bits: %v", err)
	}
}

func TestStegoHeaderRoundTrip(t *testing.T) {
	h := marshalStegoHeader(flagEncrypted|flagText, 12345)
	if len(h) != stegoHeaderLen {
		t.Fatalf("header len = %d, want %d", len(h), stegoHeaderLen)
	}
	flags, length, err := parseStegoHeader(h)
	if err != nil {
		t.Fatal(err)
	}
	if flags != flagEncrypted|flagText || length != 12345 {
		t.Fatalf("parsed flags=%d length=%d", flags, length)
	}
}

func TestParseStegoHeaderBadMagic(t *testing.T) {
	bad := make([]byte, stegoHeaderLen)
	copy(bad, "XXXX")
	if _, _, err := parseStegoHeader(bad); err == nil {
		t.Fatal("expected error on bad magic")
	}
}

func TestParseStegoHeaderTooShort(t *testing.T) {
	short := make([]byte, stegoHeaderLen-1) // one byte short; must not panic on b[6:10].
	copy(short, stegoMagic)
	if _, _, err := parseStegoHeader(short); err == nil {
		t.Fatal("expected error on short header")
	}
}

func TestParseStegoHeaderBadVersion(t *testing.T) {
	h := marshalStegoHeader(0, 1)
	h[4] = 2 // unsupported version
	if _, _, err := parseStegoHeader(h); err == nil {
		t.Fatal("expected error on bad version")
	}
}

// helper: a fresh opaque PNG carrier of the given size.
func stegoCarrierPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 120, 90, 200, 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestStegoEmbedExtract_Plaintext(t *testing.T) {
	carrier := stegoCarrierPNG(t, 64, 64)
	out, err := stegoEmbed(carrier, []byte("attack at dawn"), true, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	data, isText, err := stegoExtract(out, "")
	if err != nil {
		t.Fatal(err)
	}
	if !isText || string(data) != "attack at dawn" {
		t.Fatalf("got %q isText=%v", data, isText)
	}
}

func TestStegoEmbedExtract_Binary(t *testing.T) {
	carrier := stegoCarrierPNG(t, 64, 64)
	payload := []byte{0, 1, 2, 250, 255, 0}
	out, err := stegoEmbed(carrier, payload, false, "", 1)
	if err != nil {
		t.Fatal(err)
	}
	data, isText, err := stegoExtract(out, "")
	if err != nil {
		t.Fatal(err)
	}
	if isText || !bytes.Equal(data, payload) {
		t.Fatalf("got %v isText=%v", data, isText)
	}
}

func TestStegoEmbedExtract_Encrypted(t *testing.T) {
	carrier := stegoCarrierPNG(t, 64, 64)
	out, err := stegoEmbed(carrier, []byte("top secret"), true, "hunter2", 1)
	if err != nil {
		t.Fatal(err)
	}
	data, isText, err := stegoExtract(out, "hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if !isText || string(data) != "top secret" {
		t.Fatalf("got %q", data)
	}
}

func TestStegoExtract_WrongPassword(t *testing.T) {
	carrier := stegoCarrierPNG(t, 64, 64)
	out, _ := stegoEmbed(carrier, []byte("top secret"), true, "hunter2", 1)
	if _, _, err := stegoExtract(out, "wrong"); err == nil {
		t.Fatal("expected auth failure on wrong password")
	}
}

func TestStegoExtract_EncryptedNoPassword(t *testing.T) {
	carrier := stegoCarrierPNG(t, 64, 64)
	out, _ := stegoEmbed(carrier, []byte("x"), false, "pw", 1)
	if _, _, err := stegoExtract(out, ""); err == nil {
		t.Fatal("expected error: encrypted payload needs a password")
	}
}

func TestStegoEmbed_TooLarge(t *testing.T) {
	carrier := stegoCarrierPNG(t, 8, 8) // capacity = 8*8*3/8 - 10 = 14 bytes
	if _, err := stegoEmbed(carrier, bytes.Repeat([]byte{1}, 100), false, "", 1); err == nil {
		t.Fatal("expected payload-too-large error")
	}
}

func TestStegoExtract_NoMagic(t *testing.T) {
	// A plain PNG with no embedded payload.
	if _, _, err := stegoExtract(stegoCarrierPNG(t, 32, 32), ""); err == nil {
		t.Fatal("expected 'no sercon stego payload found'")
	}
}
