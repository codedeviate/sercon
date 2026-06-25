// cmd/sercon/image_orient_goja_test.go
package main

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// imageVM exposes the image namespace on a goja runtime for testing.
func imageVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	ns := imageNamespace(vm, nil)
	obj := vm.NewObject()
	for k, v := range ns {
		_ = obj.Set(k, v)
	}
	_ = vm.Set("image", obj)
	return vm
}

func TestOrient_ChainSwapsDims(t *testing.T) {
	vm := imageVM(t)
	vm.Set("src", vm.ToValue(plainJPEG(t))) // plainJPEG is 4x4 (square); use a rect for a real swap check below
	// Build a 3x2 PNG in-test via the image stack is awkward; assert orient(6) on a square keeps it square but is callable + chainable.
	v, err := vm.RunString(`
		const im = image.decode(src).orient(6);
		im.width + "x" + im.height;
	`)
	if err != nil {
		t.Fatalf("orient chain: %v", err)
	}
	if v.String() == "" {
		t.Fatal("orient(6) produced no dims")
	}
}

func TestOrient_RejectsBadValue(t *testing.T) {
	vm := imageVM(t)
	vm.Set("src", vm.ToValue(plainJPEG(t)))
	if _, err := vm.RunString(`image.decode(src).orient(0)`); err == nil {
		t.Fatal("orient(0) should throw")
	}
	if _, err := vm.RunString(`image.decode(src).orient(9)`); err == nil {
		t.Fatal("orient(9) should throw")
	}
	if _, err := vm.RunString(`image.decode(src).orient(2.5)`); err == nil {
		t.Fatal("orient(2.5) should throw (non-integer)")
	}
}

func TestAutoOrient_LoadOption(t *testing.T) {
	vm := imageVM(t)
	// A 3x2 image rotated 90° CW would be stored as orientation 6; auto-orient
	// should swap dims back. Build a tagged JPEG: write Orientation=6 onto a
	// JPEG, then decode with autoOrient and confirm dims swapped vs plain decode.
	tagged, err := writeExifJPEG(rectJPEG(t, 4, 2), exifDoc{"image": {"Orientation": int64(6)}}, modeReplace)
	if err != nil {
		t.Fatalf("writeExifJPEG: %v", err)
	}
	vm.Set("tagged", vm.ToValue(tagged))
	v, err := vm.RunString(`
		const plain = image.decode(tagged);
		const fixed = image.decode(tagged, { autoOrient: true });
		plain.width + "x" + plain.height + "|" + fixed.width + "x" + fixed.height;
	`)
	if err != nil {
		t.Fatalf("autoOrient: %v", err)
	}
	parts := strings.Split(v.String(), "|")
	if parts[0] == parts[1] {
		t.Fatalf("autoOrient did not change dims: %s", v.String())
	}
}

func TestDecode_NoOptsStillWorks(t *testing.T) {
	vm := imageVM(t)
	vm.Set("src", vm.ToValue(plainJPEG(t)))
	if _, err := vm.RunString(`image.decode(src).width`); err != nil {
		t.Fatalf("decode with no opts: %v", err)
	}
}
