// cmd/sercon/image_anim_goja_test.go
package main

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func animVM(t *testing.T) *goja.Runtime {
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

func TestDecodeEncodeFrames_Goja(t *testing.T) {
	vm := animVM(t)
	vm.Set("gifBytes", vm.ToValue(makeGIF(t)))
	v, err := vm.RunString(`
		const dec = image.decodeFrames(gifBytes);
		const out = image.encodeFrames(dec, { format: "apng" });   // GIF → APNG
		const re  = image.decodeFrames(out.bytes);
		dec.format + "/" + dec.frames.length + "/" + dec.frames[0].delayMs + "|" +
		out.format + "|" + re.frames.length;
	`)
	if err != nil {
		t.Fatalf("frames round-trip: %v", err)
	}
	got := v.String()
	if !strings.HasPrefix(got, "gif/2/100|apng|2") {
		t.Fatalf("got %q", got)
	}
}

func TestEncodeFrames_BadFormatThrows(t *testing.T) {
	vm := animVM(t)
	vm.Set("gifBytes", vm.ToValue(makeGIF(t)))
	if _, err := vm.RunString(`image.encodeFrames(image.decodeFrames(gifBytes), { format: "webp" })`); err == nil {
		t.Fatal("unsupported encode format should throw")
	}
}

func TestEncodeFrames_BadFrameImageThrows(t *testing.T) {
	vm := animVM(t)
	if _, err := vm.RunString(`image.encodeFrames({ frames: [{ image: {}, delayMs: 10 }] }, { format: "gif" })`); err == nil {
		t.Fatal("a frame without a real Image handle should throw")
	}
}
