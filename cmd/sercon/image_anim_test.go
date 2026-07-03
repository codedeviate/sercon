// cmd/sercon/image_anim_test.go
package main

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
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

// apngFcTLBomb builds a minimal-but-well-formed APNG whose IHDR canvas is
// canvasW x canvasH but whose single fcTL chunk declares a frame of
// frameW x frameH — the "small canvas, huge frame" decode-bomb shape.
// checkFramesPixelBudget (which reads image.DecodeConfig, i.e. the IHDR
// canvas) does not catch this: kettek/apng's decoder overwrites frame 0's
// working width/height from the fcTL chunk (reader.go parsefcTL) before
// allocating that frame's pixel buffer in readImagePass — off the fcTL
// dimensions, not the IHDR canvas — during apng.DecodeAll.
func apngFcTLBomb(t *testing.T, canvasW, canvasH, frameW, frameH uint32) []byte {
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
	binary.BigEndian.PutUint32(ihdr[0:4], canvasW)
	binary.BigEndian.PutUint32(ihdr[4:8], canvasH)
	ihdr[8] = 8  // bit depth
	ihdr[9] = 6  // color type: RGBA
	ihdr[10] = 0 // compression
	ihdr[11] = 0 // filter
	ihdr[12] = 0 // interlace
	writeChunk("IHDR", ihdr)

	actl := make([]byte, 8)
	binary.BigEndian.PutUint32(actl[0:4], 1) // num_frames
	binary.BigEndian.PutUint32(actl[4:8], 0) // num_plays (0 = loop forever)
	writeChunk("acTL", actl)

	fctl := make([]byte, 26)
	binary.BigEndian.PutUint32(fctl[0:4], 0)       // sequence_number
	binary.BigEndian.PutUint32(fctl[4:8], frameW)  // width
	binary.BigEndian.PutUint32(fctl[8:12], frameH) // height
	binary.BigEndian.PutUint32(fctl[12:16], 0)     // x_offset
	binary.BigEndian.PutUint32(fctl[16:20], 0)     // y_offset
	binary.BigEndian.PutUint16(fctl[20:22], 1)     // delay_num
	binary.BigEndian.PutUint16(fctl[22:24], 100)   // delay_den
	fctl[24] = 0 // dispose_op
	fctl[25] = 0 // blend_op
	writeChunk("fcTL", fctl)

	// Placeholder IDAT: not valid zlib data. The guard must reject this
	// file before apng.DecodeAll ever attempts to inflate/allocate a frame
	// buffer from it, so the contents don't matter here.
	writeChunk("IDAT", []byte{0x00})
	writeChunk("IEND", nil)

	return buf.Bytes()
}

// TestDecodeFramesAPNG_PerFrameFcTLBombGuard verifies decodeFramesAPNG
// rejects an APNG whose IHDR canvas is small (10x10, passes
// checkFramesPixelBudget) but whose fcTL chunk declares a huge frame
// (9000x9000 = 81,000,000 px, over the 64,000,000 cap). kettek/apng
// allocates each frame's pixel buffer from the fcTL-declared dimensions
// during apng.DecodeAll (see readImagePass in reader.go), not the IHDR
// canvas, so the canvas-only guard added in v0.85.0 lets this bomb through.
func TestDecodeFramesAPNG_PerFrameFcTLBombGuard(t *testing.T) {
	bomb := apngFcTLBomb(t, 10, 10, 9000, 9000)
	doc, err := decodeFramesAPNG(bomb)
	if err == nil {
		t.Fatal("decodeFramesAPNG should reject an oversized fcTL frame declaration")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("exceed")) {
		t.Fatalf("decodeFramesAPNG error = %q, want a pixel-limit message", err)
	}
	if len(doc.frames) != 0 {
		t.Fatalf("doc.frames = %d, want 0 (no decode should have happened)", len(doc.frames))
	}
}

// TestCheckAPNGFramePixelBudget_Int64OverflowGuard verifies
// checkAPNGFramePixelBudget still rejects an fcTL frame whose width x height
// individually fit in uint32 (as the raw chunk bytes require) but whose
// product overflows int64 when computed naively as int64(w)*int64(h): both
// dimensions here are individually valid (well under uint32's ~4.29e9 max)
// yet 3_200_000_000 * 3_200_000_000 = 1.024e19, past int64's ~9.22e18 max, so
// a naive multiply-then-compare wraps to a small/negative value and the
// ">DefaultMaxImagePixels" check silently passes — reopening the exact
// decode-bomb checkAPNGFramePixelBudget exists to close. Exercises the guard
// function directly (not through decodeFramesAPNG/apng.DecodeAll): a real
// 3.2-billion-pixel-per-side decode attempt would try to allocate a
// slice far past the runtime's max allocation size, which is unnecessary
// risk for a check that never needs to reach the decoder if the guard works.
func TestCheckAPNGFramePixelBudget_Int64OverflowGuard(t *testing.T) {
	const big = 3_200_000_000 // uint32-valid; big*big overflows int64
	bomb := apngFcTLBomb(t, 10, 10, big, big)
	err := checkAPNGFramePixelBudget(bomb)
	if err == nil {
		t.Fatal("checkAPNGFramePixelBudget should reject a fcTL frame whose w*h overflows int64, not silently pass it")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("exceed")) {
		t.Fatalf("checkAPNGFramePixelBudget error = %q, want a pixel-limit message", err)
	}
}

// TestCheckAPNGFramePixelBudget_ZeroDimensionNoFalsePositive confirms the
// division-based rewrite (int64(w) > DefaultMaxImagePixels/int64(h)) doesn't
// divide by zero and doesn't false-trip on a zero-area fcTL frame — 0 pixels
// is not a bomb, regardless of how large the other dimension declares.
func TestCheckAPNGFramePixelBudget_ZeroDimensionNoFalsePositive(t *testing.T) {
	bomb := apngFcTLBomb(t, 10, 10, 0, 4_000_000_000)
	if err := checkAPNGFramePixelBudget(bomb); err != nil {
		t.Fatalf("checkAPNGFramePixelBudget(width=0) = %v, want nil (zero-area frame is not a bomb)", err)
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
