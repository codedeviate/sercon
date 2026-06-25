// cmd/sercon/exif_goja_test.go
package main

import (
	"encoding/json"
	"math"
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

// TestExifGoja_NumericGpsRationalRoundTrip exercises the three correctness
// paths that pure-Go float64 engine tests can't reach: goja exports JS integers
// as int64 (Orientation: 6), a single rational must read back flat ([1,250] not
// [[1,250]]), and GPS decimal degrees must survive the write→read cycle.
func TestExifGoja_NumericGpsRationalRoundTrip(t *testing.T) {
	vm := newExifVM(t)
	vm.Set("srcBytes", vm.ToValue(plainJPEG(t)))
	v, err := vm.RunString(`
		const out = exif.replace(srcBytes, {
			image: { Make: "x", Orientation: 6 },
			exif:  { ExposureTime: [1, 250] },
			gps:   { GPSLatitude: 57.7089, GPSLongitude: 11.9746 },
		});
		const back = exif.read(out.bytes);
		const exp = back.exif.ExposureTime;
		JSON.stringify({
			orientation: back.image.Orientation,
			expIsFlat: Array.isArray(exp) && exp.length === 2 &&
			           typeof exp[0] === "number" && typeof exp[1] === "number",
			exp0: exp[0], exp1: exp[1],
			lat: back.gps.GPSLatitude,
			lng: back.gps.GPSLongitude,
		});
	`)
	if err != nil {
		t.Fatalf("script: %v", err)
	}
	got := v.String()
	var r struct {
		Orientation int     `json:"orientation"`
		ExpIsFlat   bool    `json:"expIsFlat"`
		Exp0        float64 `json:"exp0"`
		Exp1        float64 `json:"exp1"`
		Lat         float64 `json:"lat"`
		Lng         float64 `json:"lng"`
	}
	if err := json.Unmarshal([]byte(got), &r); err != nil {
		t.Fatalf("unmarshal result %q: %v", got, err)
	}
	if r.Orientation != 6 {
		t.Fatalf("Orientation round-trip (int64 coercion) failed: got %d, want 6 — raw=%s", r.Orientation, got)
	}
	if !r.ExpIsFlat || r.Exp0 != 1 || r.Exp1 != 250 {
		t.Fatalf("ExposureTime should read back flat [1,250], got flat=%v [%v,%v] — raw=%s", r.ExpIsFlat, r.Exp0, r.Exp1, got)
	}
	if math.Abs(r.Lat-57.7089) > 1e-3 {
		t.Fatalf("GPSLatitude round-trip failed: got %v, want ~57.7089 — raw=%s", r.Lat, got)
	}
	if math.Abs(r.Lng-11.9746) > 1e-3 {
		t.Fatalf("GPSLongitude round-trip failed: got %v, want ~11.9746 — raw=%s", r.Lng, got)
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
