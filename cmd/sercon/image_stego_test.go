// cmd/sercon/image_stego_test.go
package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestStegoCapacity(t *testing.T) {
	// 10x10 → 100px × 3 channels = 300 bits = 37 bytes total; minus 10-byte header = 27.
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if got := stegoCapacity(img); got != 27 {
		t.Fatalf("capacity(10x10) = %d, want 27", got)
	}
	// Tiny image → clamps to 0, never negative.
	if got := stegoCapacity(image.NewRGBA(image.Rect(0, 0, 2, 2))); got != 0 {
		t.Fatalf("capacity(2x2) = %d, want 0", got)
	}
}

func TestLSBStreamRoundTrip(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(0, 0, 16, 16))
	// Fill with an opaque gradient so we're not writing into all-zero bytes.
	for i := range rgba.Pix {
		rgba.Pix[i] = byte(i)
	}
	stream := []byte("hello, stego world!")
	if err := writeLSBStream(rgba, stream); err != nil {
		t.Fatal(err)
	}
	got, err := readLSBBytes(rgba, len(stream))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, stream) {
		t.Fatalf("round-trip = %q, want %q", got, stream)
	}
}

func TestLSBStreamAlphaUntouched(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range rgba.Pix {
		rgba.Pix[i] = 200
	}
	if err := writeLSBStream(rgba, bytes.Repeat([]byte{0xFF}, 5)); err != nil {
		t.Fatal(err)
	}
	for i := 3; i < len(rgba.Pix); i += 4 { // every alpha byte
		if rgba.Pix[i] != 200 {
			t.Fatalf("alpha byte at %d = %d, want 200 (untouched)", i, rgba.Pix[i])
		}
	}
}

func TestWriteLSBStreamTooLarge(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(0, 0, 2, 2)) // 4px×3 = 12 bits = 1 byte capacity
	if err := writeLSBStream(rgba, []byte("way too long")); err == nil {
		t.Fatal("expected error when stream exceeds channel capacity")
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
	out, err := stegoEmbed(carrier, []byte("attack at dawn"), true, "")
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
	out, err := stegoEmbed(carrier, payload, false, "")
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
	out, err := stegoEmbed(carrier, []byte("top secret"), true, "hunter2")
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
	out, _ := stegoEmbed(carrier, []byte("top secret"), true, "hunter2")
	if _, _, err := stegoExtract(out, "wrong"); err == nil {
		t.Fatal("expected auth failure on wrong password")
	}
}

func TestStegoExtract_EncryptedNoPassword(t *testing.T) {
	carrier := stegoCarrierPNG(t, 64, 64)
	out, _ := stegoEmbed(carrier, []byte("x"), false, "pw")
	if _, _, err := stegoExtract(out, ""); err == nil {
		t.Fatal("expected error: encrypted payload needs a password")
	}
}

func TestStegoEmbed_TooLarge(t *testing.T) {
	carrier := stegoCarrierPNG(t, 8, 8) // capacity = 8*8*3/8 - 10 = 14 bytes
	if _, err := stegoEmbed(carrier, bytes.Repeat([]byte{1}, 100), false, ""); err == nil {
		t.Fatal("expected payload-too-large error")
	}
}

func TestStegoExtract_NoMagic(t *testing.T) {
	// A plain PNG with no embedded payload.
	if _, _, err := stegoExtract(stegoCarrierPNG(t, 32, 32), ""); err == nil {
		t.Fatal("expected 'no sercon stego payload found'")
	}
}

func TestStegoCapacityOf(t *testing.T) {
	n, err := stegoCapacityOf(stegoCarrierPNG(t, 10, 10))
	if err != nil {
		t.Fatal(err)
	}
	if n != 27 {
		t.Fatalf("capacity = %d, want 27", n)
	}
}
