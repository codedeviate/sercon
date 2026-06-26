// cmd/sercon/image_apng_format_test.go
//
// Regression coverage for the kettek/apng format-registration leak: kettek's
// init() registers an "apng" matcher that shares PNG's 8-byte magic, so
// image.Decode would return "apng" for every PNG. decodeImage normalizes this
// back to "png" so EXIF dispatch, autoOrient, exif.write, and the .format
// property all keep reporting "png". These tests lock that behavior.
package main

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// TestDecodeImage_PNGFormatNotApng asserts the root-cause normalization: a
// plain PNG sniffs as "png", never "apng".
func TestDecodeImage_PNGFormatNotApng(t *testing.T) {
	_, format, err := decodeImage(plainPNG(t))
	if err != nil {
		t.Fatalf("decodeImage(plainPNG): %v", err)
	}
	if format != "png" {
		t.Fatalf("decodeImage format = %q, want \"png\" (kettek/apng must not hijack the sniff)", format)
	}
}

// TestImageHandle_PNGFormatIsPng asserts the script-facing .format property of
// a decoded PNG handle is "png", not "apng".
func TestImageHandle_PNGFormatIsPng(t *testing.T) {
	vm := goja.New()
	ns := imageNamespace(vm, nil)
	obj := vm.NewObject()
	for k, v := range ns {
		_ = obj.Set(k, v)
	}
	_ = vm.Set("image", obj)
	vm.Set("pngBytes", vm.ToValue(plainPNG(t)))
	v, err := vm.RunString(`image.decode(pngBytes).format`)
	if err != nil {
		t.Fatalf("image.decode(png).format: %v", err)
	}
	if v.String() != "png" {
		t.Fatalf("image.decode(png).format = %q, want \"png\"", v.String())
	}
}

// TestExifGoja_PNGWriteReadRoundTrip is the scariest regression: exif.write on
// a PNG must NOT throw "writing EXIF to apng is unsupported", and a tag must
// round-trip. exif.write/read dispatch through sniffFormat → decodeImage, so
// the normalization is what keeps the "png" branch reachable.
func TestExifGoja_PNGWriteReadRoundTrip(t *testing.T) {
	vm := newExifVM(t)
	vm.Set("pngBytes", vm.ToValue(plainPNG(t)))
	v, err := vm.RunString(`
		const out = exif.write(pngBytes, { image: { Make: "Nikon" } });
		const back = exif.read(out.bytes);
		out.format + "|" + back.image.Make;
	`)
	if err != nil {
		t.Fatalf("exif.write/read on PNG threw (regression): %v", err)
	}
	got := v.String()
	if got != "png|Nikon" {
		t.Fatalf("PNG EXIF round-trip = %q, want \"png|Nikon\"", got)
	}
	if strings.Contains(got, "apng") {
		t.Fatalf("PNG EXIF reported apng format: %q", got)
	}
}

// TestExifOrientation_PNGPath exercises the autoOrient dispatch: a PNG with no
// EXIF resolves to orientation 1 (identity) via the "png" path, not the
// imagemeta fallback. This locks that autoOrient still routes PNG correctly.
func TestExifOrientation_PNGPath(t *testing.T) {
	got := exifOrientation(plainPNG(t), "png")
	if got != 1 {
		t.Fatalf("exifOrientation(png, \"png\") = %d, want 1 (no-EXIF identity via png path)", got)
	}
}
