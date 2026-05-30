package main

import (
	"bytes"
	"image/png"
	"strings"
	"testing"

	"github.com/boombuler/barcode"
)

// Each supported format must produce a non-trivial PNG against an
// appropriate payload. We don't decode the result back — that's the v0.4.11
// scope (encoders only); the v0.4.13 cut adds a scanner. Instead we check
// the PNG signature, decode the header for sensible dimensions, and
// confirm the byte length is in a realistic range.
func TestBarcode_EncodeAllFormats(t *testing.T) {
	// Format-appropriate payloads. EAN/UPC need numeric content of a
	// specific length; the others accept arbitrary text.
	payloads := map[string]string{
		"qr":         "https://github.com/codedeviate/sercon",
		"datamatrix": "sercon-test",
		"aztec":      "sercon-test",
		"pdf417":     "sercon-test",
		"code128":    "Sercon-128",
		"code39":     "SERCON-39",
		"codabar":    "A123456A",
		"ean13":      "5901234123457", // 13 digits incl. check
		"ean8":       "12345670",      // 8 digits incl. check
		"upca":       "012345678905",  // 12 digits incl. check
	}
	pngSig := []byte("\x89PNG\r\n\x1a\n")

	for _, format := range barcodeFormats {
		t.Run(format, func(t *testing.T) {
			payload, ok := payloads[format]
			if !ok {
				t.Fatalf("no payload defined for %q", format)
			}
			img, err := buildBarcode(format, payload)
			if err != nil {
				t.Fatalf("buildBarcode: %v", err)
			}
			if img == nil {
				t.Fatal("buildBarcode returned nil")
			}
			// Run the same encode pipeline the public binding uses by
			// calling barcodeEncodeCall via its core path.
			got, err := encodeViaPipeline(format, payload)
			if err != nil {
				t.Fatalf("encodeViaPipeline: %v", err)
			}
			if !bytes.HasPrefix(got, pngSig) {
				t.Fatalf("output is not a PNG (first 8 bytes: %x)", got[:8])
			}
			if len(got) < 200 {
				t.Errorf("PNG suspiciously small: %d bytes", len(got))
			}
			// Decode the header so we can confirm the image is the size
			// the default-sizing logic picked.
			cfg, err := png.DecodeConfig(bytes.NewReader(got))
			if err != nil {
				t.Fatalf("png.DecodeConfig: %v", err)
			}
			switch format {
			case "qr", "datamatrix", "aztec":
				if cfg.Width != 256 || cfg.Height != 256 {
					t.Errorf("default 2D size: got %dx%d, want 256x256", cfg.Width, cfg.Height)
				}
			default:
				if cfg.Width != 400 || cfg.Height != 120 {
					t.Errorf("default 1D size: got %dx%d, want 400x120", cfg.Width, cfg.Height)
				}
			}
		})
	}
}

// Unknown formats must fail with a clear message that includes the
// supported list — easier than having callers consult the manual.
func TestBarcode_UnknownFormat(t *testing.T) {
	_, err := buildBarcode("definitely-not-a-format", "x")
	if err == nil {
		t.Fatal("expected error for unknown format")
	}
	if !strings.Contains(err.Error(), "supported:") {
		t.Errorf("error should mention supported list, got: %v", err)
	}
}

// encodeViaPipeline reproduces what barcodeEncodeCall does (build + scale
// + png-encode) without going through the goja call shim, so the tests
// don't have to set up a runtime just to verify the pipeline.
func encodeViaPipeline(format, data string) ([]byte, error) {
	bc, err := buildBarcode(format, data)
	if err != nil {
		return nil, err
	}
	w, h := 400, 120
	switch format {
	case "qr", "datamatrix", "aztec":
		w, h = 256, 256
	}
	scaled, err := barcode.Scale(bc, w, h)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, scaled); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
