package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// encodeAndDecode drives a single round-trip through api.barcode.encode
// followed by api.barcode.decode against the engine boundary. The hint
// is passed straight through to decode; pass "" to exercise the
// auto-detect path. Returns (decodedFormat, decodedText) — the format
// label is the snake-case identifier the engine surfaces.
func encodeAndDecode(t *testing.T, encFormat, payload, decodeHint string) (string, string) {
	t.Helper()
	var captured map[string]any
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot: t.TempDir(),
		Timeout:    10 * time.Second,
	})
	if err := eng.RegisterNamespaceFactory("barcode", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return barcodeNamespace(vm, loop)
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			return
		}
		m, _ := v.Export().(map[string]any)
		captured = m
	}); err != nil {
		t.Fatal(err)
	}
	src := "const png = await barcode.encode(\"" + encFormat + "\", \"" + payload + "\");\n"
	if decodeHint == "" {
		src += "const out = await barcode.decode(png);\n"
	} else {
		src += "const out = await barcode.decode(png, \"" + decodeHint + "\");\n"
	}
	src += "__capture(out);\n"
	if _, err := eng.Run(context.Background(), "rt.ts", src); err != nil {
		t.Fatalf("script: %v", err)
	}
	if captured == nil {
		t.Fatal("decode returned no value")
	}
	return captured["format"].(string), captured["text"].(string)
}

// Round-trip every 2D format. QR / DataMatrix / Aztec are the
// gozxing-supported 2D codes; pdf417 is encoder-only.
func TestBarcodeDecode_RoundTrip2D(t *testing.T) {
	for _, format := range []string{"qr", "datamatrix", "aztec"} {
		t.Run(format, func(t *testing.T) {
			// Use a payload that's safe for all 2D formats — no
			// embedded quotes (we string-concat into JS source).
			gotFormat, gotText := encodeAndDecode(t, format, "round trip "+format, format)
			if gotFormat != format {
				t.Errorf("format: %q (want %q)", gotFormat, format)
			}
			if gotText != "round trip "+format {
				t.Errorf("text: %q", gotText)
			}
		})
	}
}

// Round-trip the 1D formats that don't require a quiet zone for
// reliable decoding. Code 128 / Code 39 / Codabar's start/stop
// patterns are robust enough that gozxing finds them without
// extra whitespace around the bars.
//
// EAN-13 / EAN-8 / UPC-A are deliberately NOT in this loop. Those
// symbologies REQUIRE a quiet zone (the empty white margin on
// either side of the barcode) per ISO/IEC 15420 + 15418; without
// it gozxing's row scanner can't lock onto the start guard
// pattern. boombuler's encoder doesn't add a quiet zone, so a
// `barcode.encode("ean13", x) → barcode.decode(...)` round-trip
// fails — that's not a sercon bug, it's the spec asserting itself.
// TestBarcodeDecode_EAN13WithQuietZone exercises the decoder
// against a manually-padded EAN PNG so we still have coverage for
// the family.
func TestBarcodeDecode_RoundTrip1D(t *testing.T) {
	// `wantText` is the value the *decoder* surfaces. It isn't always
	// the byte-for-byte encode input — Code 39 appends a Mod-43
	// checksum char, codabar's start/stop guards (the A...A wrappers
	// in the encoded payload) are stripped on decode. These are
	// documented behaviours of each library; we pin the round-trip
	// shape so future library upgrades that change them get caught.
	cases := []struct{ format, payload, wantText string }{
		{"code128", "ABC-12345", "ABC-12345"},
		{"code39", "HELLO-39", "HELLO-39G"}, // G is the Mod-43 checksum char
		{"codabar", "A123456A", "123456"},   // start/stop guards stripped
	}
	for _, tc := range cases {
		t.Run(tc.format, func(t *testing.T) {
			gotFormat, gotText := encodeAndDecode(t, tc.format, tc.payload, tc.format)
			if gotFormat != tc.format {
				t.Errorf("format: %q (want %q)", gotFormat, tc.format)
			}
			if gotText != tc.wantText {
				t.Errorf("text: %q (want %q)", gotText, tc.wantText)
			}
		})
	}
}

// EAN-13 with a manually-added quiet zone. Encode the bars at a
// natural width, then paste the PNG onto a wider white canvas so the
// barcode has the spec-required clear margin on each side. Proves
// the decoder works for the EAN family — only the encoder's missing
// quiet zone is the gap.
func TestBarcodeDecode_EAN13WithQuietZone(t *testing.T) {
	// Encode at the default 400x120 (no opts), then pad. Sizing through
	// opts wouldn't fix it — what's missing is the white border.
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("barcode", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return barcodeNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	var rawPNG []byte
	if err := eng.Register("__capturePNG", func(v goja.Value) {
		if b, ok := v.Export().([]byte); ok {
			rawPNG = b
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "e.ts",
		`__capturePNG(await barcode.encode("ean13", "5901234123457"));`); err != nil {
		t.Fatalf("encode: %v", err)
	}

	// Decode the bars, paste onto a 200%-wide white canvas, re-encode.
	bars, err := png.Decode(bytes.NewReader(rawPNG))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	bw, bh := bars.Bounds().Dx(), bars.Bounds().Dy()
	canvas := image.NewNRGBA(image.Rect(0, 0, bw*2, bh+40))
	for y := 0; y < canvas.Bounds().Dy(); y++ {
		for x := 0; x < canvas.Bounds().Dx(); x++ {
			canvas.Set(x, y, color.White)
		}
	}
	offsetX := bw / 2
	for y := 0; y < bh; y++ {
		for x := 0; x < bw; x++ {
			canvas.Set(offsetX+x, 20+y, bars.At(x, y))
		}
	}
	var padded bytes.Buffer
	if err := png.Encode(&padded, canvas); err != nil {
		t.Fatalf("re-encode: %v", err)
	}

	if err := eng.Register("__padded", padded.Bytes()); err != nil {
		t.Fatal(err)
	}
	var captured map[string]any
	if err := eng.Register("__capture", func(v goja.Value) {
		if m, ok := v.Export().(map[string]any); ok {
			captured = m
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "d.ts",
		`__capture(await barcode.decode(__padded, "ean13"));`); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if captured == nil {
		t.Fatal("decode returned no value")
	}
	if got := captured["text"].(string); got != "5901234123457" {
		t.Errorf("EAN-13 decoded text: %q (want %q)", got, "5901234123457")
	}
}

// Auto-detect (no format hint) finds the right symbology on its own.
// The priority order in autoDecodeOrder lists 2D first, so a QR
// payload should come back labelled "qr".
func TestBarcodeDecode_AutoDetectQR(t *testing.T) {
	gotFormat, gotText := encodeAndDecode(t, "qr", "auto", "")
	if gotFormat != "qr" {
		t.Errorf("auto-detect picked %q (expected qr)", gotFormat)
	}
	if gotText != "auto" {
		t.Errorf("text: %q", gotText)
	}
}

// Non-image bytes throw a clear `decode image` error (sercon's
// prefix), not the raw image/png panic.
func TestBarcodeDecode_NonImageBytesThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("barcode", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return barcodeNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await barcode.decode("not-an-image-at-all");`)
	if err == nil {
		t.Fatal("expected throw for non-image bytes")
	}
	if !strings.Contains(err.Error(), "decode image") {
		t.Errorf("error wording: %v", err)
	}
}

// A valid PNG with no barcode in it produces a clear "no barcode
// recognised" message (auto-detect path).
func TestBarcodeDecode_BlankImageThrows(t *testing.T) {
	// 100x100 all-white PNG — small and decodes fast.
	blank := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			blank.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, blank); err != nil {
		t.Fatal(err)
	}

	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 5 * time.Second})
	if err := eng.RegisterNamespaceFactory("barcode", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return barcodeNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__png", buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await barcode.decode(__png);`)
	if err == nil {
		t.Fatal("expected throw for blank image")
	}
	if !strings.Contains(err.Error(), "barcode") {
		t.Errorf("error wording: %v", err)
	}
}

// Unknown format hint surfaces a clear "unsupported format" error
// listing the supported set rather than producing a generic miss.
func TestBarcodeDecode_UnknownFormatHint(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("barcode", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return barcodeNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	// Build a valid tiny PNG so we get past image.Decode.
	img := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	if err := eng.Register("__png", buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await barcode.decode(__png, "pdf417");`)
	if err == nil {
		t.Fatal("expected throw for pdf417 hint (decoder unsupported)")
	}
	if !strings.Contains(err.Error(), "unsupported format") {
		t.Errorf("error wording: %v", err)
	}
}

// sniffImageFormat unit-tests the magic-byte recogniser without
// touching gozxing. PNG / JPEG / WebP markers map to the right
// labels; anything else returns "".
func TestSniffImageFormat(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"png magic", []byte{0x89, 'P', 'N', 'G', '\r', '\n'}, "png"},
		{"jpeg magic", []byte{0xFF, 0xD8, 0xFF, 0xE0}, "jpeg"},
		{"webp magic", append(append([]byte("RIFF"), 0, 0, 0, 0), []byte("WEBP")...), "webp"},
		{"plain text", []byte("hello"), ""},
		{"too short", []byte{0x89}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sniffImageFormat(tc.data); got != tc.want {
				t.Errorf("sniff %q: %q (want %q)", tc.name, got, tc.want)
			}
		})
	}
}

// JPEG input round-trips too (not just PNG). Encode a QR as PNG,
// transcode through image/jpeg, then decode. JPEG's lossy
// compression is rough on 2D codes, so we use a higher quality
// setting and a payload that has plenty of error-correction
// headroom (medium QR, short text).
func TestBarcodeDecode_JPEGInput(t *testing.T) {
	// Build a QR PNG via api.barcode.encode, then re-encode as JPEG.
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("barcode", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return barcodeNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	var pngOut []byte
	if err := eng.Register("__capturePNG", func(v goja.Value) {
		if b, ok := v.Export().([]byte); ok {
			pngOut = b
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "e.ts",
		`__capturePNG(await barcode.encode("qr", "jpeg-rt", { width: 400, height: 400 }));`); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if len(pngOut) == 0 {
		t.Fatal("encode produced no bytes")
	}

	// Decode PNG → re-encode as JPEG quality 95.
	img, err := png.Decode(bytes.NewReader(pngOut))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	var jpegBuf bytes.Buffer
	if err := jpeg.Encode(&jpegBuf, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("jpeg encode: %v", err)
	}

	// Feed JPEG bytes into decode — auto-detect path.
	var captured map[string]any
	if err := eng.Register("__jpeg", jpegBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if m, ok := v.Export().(map[string]any); ok {
			captured = m
		}
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "d.ts",
		`__capture(await barcode.decode(__jpeg, "qr"));`); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if captured == nil {
		t.Fatal("decode produced no value")
	}
	if got := captured["text"].(string); got != "jpeg-rt" {
		t.Errorf("JPEG round-trip text: %q (want %q)", got, "jpeg-rt")
	}
}
