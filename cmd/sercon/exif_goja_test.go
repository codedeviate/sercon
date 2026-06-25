// cmd/sercon/exif_goja_test.go
package main

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func newExifVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	ns := exifNamespace(vm)
	img := vm.NewObject()
	for k, v := range ns {
		_ = img.Set(k, v)
	}
	_ = vm.Set("exif", img)
	return vm
}

func TestExifGoja_WriteReadRoundTrip(t *testing.T) {
	vm := newExifVM(t)
	// build a plain JPEG in Go, expose as Uint8Array
	vm.Set("srcBytes", vm.ToValue(plainJPEG(t)))
	v, err := vm.RunString(`
		const out = exif.replace(srcBytes, { image: { Make: "Canon" } });
		const back = exif.read(out.bytes);
		back.image.Make;
	`)
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	if v.String() != "Canon" {
		t.Fatalf("round-trip via goja got %q", v.String())
	}
}

func TestExifGoja_WriteUnsupportedFormatThrows(t *testing.T) {
	vm := newExifVM(t)
	vm.Set("pngBytes", vm.ToValue(plainPNG(t)))
	// PNG write IS supported; use a webp-ish fake to assert the throw path.
	_, err := vm.RunString(`exif.write(new Uint8Array([0x00,0x01,0x02,0x03]), { image: {} })`)
	if err == nil || !strings.Contains(err.Error(), "exif") {
		t.Fatalf("expected a thrown error for unrecognized format, got %v", err)
	}
}
