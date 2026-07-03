package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"testing"
)

// oversizedHeaderPNG builds a minimal-but-valid PNG (signature + IHDR + IEND)
// whose IHDR chunk declares width x height, without any pixel data. This is
// enough for image.DecodeConfig (and image.Decode, if unguarded) to read the
// declared dimensions cheaply from a file that's only a few dozen bytes —
// the classic "decode bomb" shape: tiny file, attacker-chosen huge
// allocation on decode.
func oversizedHeaderPNG(t *testing.T, width, height uint32) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a})

	writeChunk := func(typ string, data []byte) {
		var lenBuf [4]byte
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
		buf.Write(lenBuf[:])
		buf.WriteString(typ)
		buf.Write(data)
		crc := crc32.NewIEEE()
		crc.Write([]byte(typ))
		crc.Write(data)
		var crcBuf [4]byte
		binary.BigEndian.PutUint32(crcBuf[:], crc.Sum32())
		buf.Write(crcBuf[:])
	}

	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], width)
	binary.BigEndian.PutUint32(ihdr[4:8], height)
	ihdr[8] = 8    // bit depth
	ihdr[9] = 6    // color type: RGBA
	ihdr[10] = 0   // compression
	ihdr[11] = 0   // filter
	ihdr[12] = 0   // interlace
	writeChunk("IHDR", ihdr)
	writeChunk("IEND", nil)

	return buf.Bytes()
}

// genImage builds an w×h image with a simple gradient for round-trip tests.
func genImage(w, h int) image.Image {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			m.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
		}
	}
	return m
}

func TestInferFormatFromPath(t *testing.T) {
	cases := map[string]string{
		"a.png": "png", "b.PNG": "png", "c.jpg": "jpeg", "d.jpeg": "jpeg",
		"e.gif": "gif", "f.tif": "tiff", "g.tiff": "tiff", "h.bmp": "bmp",
		"i.webp": "webp",
	}
	for path, want := range cases {
		got, err := inferFormatFromPath(path)
		if err != nil || got != want {
			t.Fatalf("inferFormatFromPath(%q) = %q,%v want %q", path, got, err, want)
		}
	}
	if _, err := inferFormatFromPath("x.txt"); err == nil {
		t.Fatal("unknown extension should error")
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	src := genImage(20, 12)
	for _, f := range []string{"png", "jpeg", "gif", "tiff", "bmp"} {
		raw, err := encodeImage(src, f, encodeOpts{quality: 90})
		if err != nil {
			t.Fatalf("encode %s: %v", f, err)
		}
		img, gotFmt, err := decodeImage(raw)
		if err != nil {
			t.Fatalf("decode %s: %v", f, err)
		}
		if img.Bounds().Dx() != 20 || img.Bounds().Dy() != 12 {
			t.Fatalf("%s dims = %v", f, img.Bounds())
		}
		_ = gotFmt // sniffed format name; jpeg/gif may differ in exact string
	}
}

func TestEncodeUnknownFormat(t *testing.T) {
	if _, err := encodeImage(genImage(2, 2), "qoi", encodeOpts{}); err == nil {
		t.Fatal("unknown encode format should error")
	}
}

func TestDecodeGarbage(t *testing.T) {
	if _, _, err := decodeImage([]byte("not an image")); err == nil {
		t.Fatal("garbage should fail to decode")
	}
}

// TestDecodeImage_PixelBombGuard verifies decodeImage rejects a tiny PNG
// whose IHDR declares dimensions beyond DefaultMaxImagePixels before
// attempting the full pixel decode (which would allocate gigabytes for a
// crafted file of only a few dozen bytes).
func TestDecodeImage_PixelBombGuard(t *testing.T) {
	// 40000 x 40000 = 1.6e9 pixels, well beyond the 64,000,000 cap, but the
	// PNG file itself is under 100 bytes (header only, no pixel data).
	bomb := oversizedHeaderPNG(t, 40000, 40000)
	if _, _, err := decodeImage(bomb); err == nil {
		t.Fatal("decodeImage should reject an oversized declared pixel count")
	} else if !bytes.Contains([]byte(err.Error()), []byte("exceed")) {
		t.Fatalf("decodeImage error = %q, want a pixel-limit message", err)
	}
}

// TestDecodeImage_PixelBombGuard_AllowsNormalImage ensures the guard doesn't
// false-positive on ordinary, well-within-limits images.
func TestDecodeImage_PixelBombGuard_AllowsNormalImage(t *testing.T) {
	raw, err := encodeImage(genImage(20, 12), "png", encodeOpts{quality: 90})
	if err != nil {
		t.Fatal(err)
	}
	img, _, err := decodeImage(raw)
	if err != nil {
		t.Fatalf("decodeImage(normal png): %v", err)
	}
	if img.Bounds().Dx() != 20 || img.Bounds().Dy() != 12 {
		t.Fatalf("dims = %v", img.Bounds())
	}
}

func TestWebPRoundTrip(t *testing.T) {
	raw, err := encodeImage(genImage(16, 16), "webp", encodeOpts{})
	if err != nil {
		t.Skipf("webp encode unavailable: %v", err)
	}
	img, _, err := decodeImage(raw)
	if err != nil || img.Bounds().Dx() != 16 {
		t.Fatalf("webp round-trip: %v", err)
	}
}

func TestImageScript_Transforms(t *testing.T) {
	// Encode a 40x20 source to PNG, hand it to a script as __png, run a chain.
	raw, err := encodeImage(genImage(40, 20), "png", encodeOpts{})
	if err != nil {
		t.Fatal(err)
	}
	got := runCaptureScript(t, `
		const im = image.decode(__png);
		const small = im.resize(20, 0).grayscale();
		__capture({ w0: im.width, h0: im.height, w1: small.width, h1: small.height,
		            png: small.bytes("png").length > 8, fmt: im.format });
	`, map[string]any{"__png": raw})
	m := got.(map[string]any)
	if fmt.Sprintf("%v", m["w0"]) != "40" || fmt.Sprintf("%v", m["w1"]) != "20" {
		t.Fatalf("dims wrong: %#v", m)
	}
	if fmt.Sprintf("%v", m["h1"]) != "10" { // 40x20 → width 20 preserves aspect → 10
		t.Fatalf("aspect wrong: %#v", m)
	}
}
