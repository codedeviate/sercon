package main

import (
	"fmt"
	"image"
	"image/color"
	"testing"
)

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
