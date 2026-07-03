// cmd/sercon/image_anim_test.go
package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"testing"

	"github.com/kettek/apng"
)

// oversizedHeaderGIF builds a minimal GIF: the 6-byte signature plus a
// 7-byte Logical Screen Descriptor declaring width x height, then an
// immediate trailer (no image descriptor, no pixel data at all). This is
// enough for image.DecodeConfig / gif.DecodeConfig to read the declared
// canvas size cheaply from a ~20-byte file — the same "tiny file, huge
// declared dimensions" decode-bomb shape as oversizedHeaderPNG, but for the
// animated GIF container.
func oversizedHeaderGIF(t *testing.T, width, height uint16) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("GIF89a")
	lsd := make([]byte, 7)
	binary.LittleEndian.PutUint16(lsd[0:2], width)
	binary.LittleEndian.PutUint16(lsd[2:4], height)
	buf.Write(lsd)
	buf.WriteByte(0x3B) // trailer
	return buf.Bytes()
}

// makeGIF builds an in-memory animated GIF: 2 frames, 3x2, delays 10 & 20 (1/100s),
// disposal background on frame 2, loop 0.
func makeGIF(t *testing.T) []byte {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	f0 := image.NewPaletted(image.Rect(0, 0, 3, 2), pal)
	f1 := image.NewPaletted(image.Rect(0, 0, 3, 2), pal)
	f1.SetColorIndex(0, 0, 1)
	g := &gif.GIF{
		Image:    []*image.Paletted{f0, f1},
		Delay:    []int{10, 20},
		Disposal: []byte{gif.DisposalNone, gif.DisposalBackground},
		LoopCount: 0,
		Config:   image.Config{ColorModel: pal, Width: 3, Height: 2},
	}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func makeAPNG(t *testing.T) []byte {
	t.Helper()
	mk := func() image.Image {
		m := image.NewNRGBA(image.Rect(0, 0, 3, 2))
		m.Set(0, 0, color.NRGBA{255, 0, 0, 255})
		return m
	}
	a := apng.APNG{
		LoopCount: 0,
		Frames: []apng.Frame{
			{Image: mk(), DelayNumerator: 1, DelayDenominator: 100}, // 10ms
			{Image: mk(), DelayNumerator: 2, DelayDenominator: 100, DisposeOp: 1}, // 20ms, background
		},
	}
	var b bytes.Buffer
	if err := apng.Encode(&b, a); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDecodeFramesGIF(t *testing.T) {
	doc, err := decodeFramesGIF(makeGIF(t))
	if err != nil {
		t.Fatal(err)
	}
	if doc.format != "gif" || len(doc.frames) != 2 {
		t.Fatalf("got format=%q frames=%d", doc.format, len(doc.frames))
	}
	if doc.frames[0].delayMs != 100 || doc.frames[1].delayMs != 200 {
		t.Fatalf("delays = %d, %d (want 100,200)", doc.frames[0].delayMs, doc.frames[1].delayMs)
	}
	if doc.frames[0].disposal != "none" || doc.frames[1].disposal != "background" {
		t.Fatalf("disposal = %q, %q", doc.frames[0].disposal, doc.frames[1].disposal)
	}
	if doc.width != 3 || doc.height != 2 {
		t.Fatalf("dims = %dx%d", doc.width, doc.height)
	}
}

func TestDecodeFramesAPNG(t *testing.T) {
	doc, err := decodeFramesAPNG(makeAPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if doc.format != "apng" || len(doc.frames) != 2 {
		t.Fatalf("got format=%q frames=%d", doc.format, len(doc.frames))
	}
	if doc.frames[0].delayMs != 10 || doc.frames[1].delayMs != 20 {
		t.Fatalf("delays = %d, %d (want 10,20)", doc.frames[0].delayMs, doc.frames[1].delayMs)
	}
	if doc.frames[1].disposal != "background" {
		t.Fatalf("disposal[1] = %q want background", doc.frames[1].disposal)
	}
}

func solidFrame(c color.Color) animFrame {
	m := image.NewNRGBA(image.Rect(0, 0, 3, 2))
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			m.Set(x, y, c)
		}
	}
	return animFrame{img: m, delayMs: 40, disposal: "none", blend: "over"}
}

func TestEncodeDecodeGIF_RoundTrip(t *testing.T) {
	f0 := solidFrame(color.NRGBA{255, 0, 0, 255})
	f1 := solidFrame(color.NRGBA{0, 255, 0, 255})
	f1.disposal = "background"
	doc := animDoc{format: "gif", width: 3, height: 2, loopCount: 0,
		frames: []animFrame{f0, f1}}
	data, err := encodeFramesGIF(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := decodeFramesGIF(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(back.frames) != 2 {
		t.Fatalf("round-trip frames = %d, want 2", len(back.frames))
	}
	if back.frames[0].delayMs != 40 {
		t.Fatalf("delayMs = %d, want 40", back.frames[0].delayMs)
	}
	if back.frames[1].disposal != "background" {
		t.Fatalf("disposal[1] = %q, want background", back.frames[1].disposal)
	}
}

func TestEncodeDecodeAPNG_RoundTrip(t *testing.T) {
	f0 := solidFrame(color.NRGBA{255, 0, 0, 255})
	f1 := solidFrame(color.NRGBA{0, 255, 0, 255})
	f1.disposal = "previous"
	f1.blend = "source"
	doc := animDoc{format: "apng", width: 3, height: 2, loopCount: 0,
		frames: []animFrame{f0, f1}}
	data, err := encodeFramesAPNG(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := decodeFramesAPNG(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(back.frames) != 2 || back.frames[1].delayMs != 40 {
		t.Fatalf("round-trip frames=%d delay=%d", len(back.frames), back.frames[1].delayMs)
	}
	if back.frames[1].disposal != "previous" {
		t.Fatalf("disposal[1] = %q, want previous", back.frames[1].disposal)
	}
	if back.frames[1].blend != "source" {
		t.Fatalf("blend[1] = %q, want source", back.frames[1].blend)
	}
}

func TestEncodeDecodeAPNG_LongDelay(t *testing.T) {
	f := solidFrame(color.NRGBA{255, 0, 0, 255})
	f.delayMs = 70000 // > 65535: forces centisecond rescale to avoid uint16 wrap
	doc := animDoc{format: "apng", width: 3, height: 2, loopCount: 0,
		frames: []animFrame{f}}
	data, err := encodeFramesAPNG(doc)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	back, err := decodeFramesAPNG(data)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(back.frames) != 1 {
		t.Fatalf("frames = %d, want 1", len(back.frames))
	}
	// At centisecond granularity the round-trip is exact to within 10ms.
	if d := back.frames[0].delayMs; d < 69990 || d > 70010 {
		t.Fatalf("delayMs = %d, want ~70000 (±10)", d)
	}
}

func TestEncodeFramesGIF_Empty(t *testing.T) {
	if _, err := encodeFramesGIF(animDoc{format: "gif"}); err == nil {
		t.Fatal("empty frames should error")
	}
}

// TestDecodeFramesGIF_PixelBombGuard verifies decodeFramesGIF rejects a tiny
// GIF whose Logical Screen Descriptor declares dimensions beyond
// DefaultMaxImagePixels before attempting gif.DecodeAll (which would
// allocate a full paletted frame buffer sized from the declaration).
func TestDecodeFramesGIF_PixelBombGuard(t *testing.T) {
	bomb := oversizedHeaderGIF(t, 40000, 40000) // 1.6e9 px, ~20-byte file
	doc, err := decodeFramesGIF(bomb)
	if err == nil {
		t.Fatal("decodeFramesGIF should reject an oversized declared pixel count")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("exceed")) {
		t.Fatalf("decodeFramesGIF error = %q, want a pixel-limit message", err)
	}
	if len(doc.frames) != 0 {
		t.Fatalf("doc.frames = %d, want 0 (no decode should have happened)", len(doc.frames))
	}
}

// TestDecodeFramesAPNG_PixelBombGuard mirrors the GIF case for the APNG
// container, reusing the PNG IHDR-bomb builder from image_test.go (same
// package) since kettek/apng shares PNG's magic bytes and IHDR layout.
func TestDecodeFramesAPNG_PixelBombGuard(t *testing.T) {
	bomb := oversizedHeaderPNG(t, 40000, 40000)
	doc, err := decodeFramesAPNG(bomb)
	if err == nil {
		t.Fatal("decodeFramesAPNG should reject an oversized declared pixel count")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("exceed")) {
		t.Fatalf("decodeFramesAPNG error = %q, want a pixel-limit message", err)
	}
	if len(doc.frames) != 0 {
		t.Fatalf("doc.frames = %d, want 0 (no decode should have happened)", len(doc.frames))
	}
}

// TestDecodeFramesAny_PixelBombGuard confirms the guard is reachable through
// the public image.decodeFrames entry point (decodeFramesAny), not just the
// unexported per-format helpers.
func TestDecodeFramesAny_PixelBombGuard(t *testing.T) {
	bomb := oversizedHeaderGIF(t, 40000, 40000)
	if _, err := decodeFramesAny(bomb); err == nil {
		t.Fatal("decodeFramesAny should reject an oversized declared pixel count")
	} else if !bytes.Contains([]byte(err.Error()), []byte("exceed")) {
		t.Fatalf("decodeFramesAny error = %q, want a pixel-limit message", err)
	}
}

func TestDecodeFramesAny_NonAnimated(t *testing.T) {
	// plainPNG is defined in exif_engine_test.go (same package).
	doc, err := decodeFramesAny(plainPNG(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.frames) != 1 {
		t.Fatalf("non-animated → %d frames, want 1", len(doc.frames))
	}
	if doc.frames[0].delayMs != 0 {
		t.Fatalf("single frame delayMs = %d, want 0", doc.frames[0].delayMs)
	}
}
