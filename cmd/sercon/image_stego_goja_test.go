// cmd/sercon/image_stego_goja_test.go
package main

import (
	"testing"

	"github.com/dop251/goja"
)

func stegoVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	ns := stegoNamespace(vm)
	obj := vm.NewObject()
	for k, v := range ns {
		_ = obj.Set(k, v)
	}
	img := vm.NewObject()
	_ = img.Set("stego", obj)
	_ = vm.Set("image", img)
	// Provide a carrier PNG (64x64 opaque) as a global Uint8Array.
	if err := vm.Set("carrier", stegoCarrierPNG(t, 64, 64)); err != nil {
		t.Fatal(err)
	}
	return vm
}

func TestStegoGoja_TextRoundTrip(t *testing.T) {
	vm := stegoVM(t)
	v, err := vm.RunString(`
		const out = image.stego.embed(carrier, "secret message");
		const back = image.stego.extract(out.bytes);
		(typeof back) + "|" + back;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "string|secret message" {
		t.Fatalf("got %q (want string|secret message)", got)
	}
}

func TestStegoGoja_BinaryRoundTrip(t *testing.T) {
	vm := stegoVM(t)
	// Exercise the isText=false branch of extract: a binary payload must come
	// back as bytes (NOT a JS string) and round-trip byte-for-byte.
	//
	// Note: goja surfaces a Go []byte returned via vm.ToValue as a plain JS
	// Array of byte values (not a Uint8Array instance, despite the
	// "[]byte → Uint8Array" shorthand in the binding comments). The bytes are
	// intact and iterable; the test asserts on that real contract — not a
	// string, and `new Uint8Array(back)` reconstructs the exact bytes.
	v, err := vm.RunString(`
		const bin = new Uint8Array([1,2,3,250,0,255]);
		const out = image.stego.embed(carrier, bin);
		const back = image.stego.extract(out.bytes);
		const view = new Uint8Array(back);
		(typeof back) + "|" + (typeof back === "string") + "|" + Array.from(view).join(",");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "object|false|1,2,3,250,0,255" {
		t.Fatalf("got %q (want object|false|1,2,3,250,0,255)", got)
	}
}

func TestStegoGoja_PasswordRoundTrip(t *testing.T) {
	vm := stegoVM(t)
	v, err := vm.RunString(`
		const out = image.stego.embed(carrier, "classified", { password: "pw" });
		image.stego.extract(out.bytes, { password: "pw" });
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "classified" {
		t.Fatalf("got %q", v.String())
	}
}

func TestStegoGoja_WrongPasswordThrows(t *testing.T) {
	vm := stegoVM(t)
	_, err := vm.RunString(`
		const out = image.stego.embed(carrier, "x", { password: "right" });
		image.stego.extract(out.bytes, { password: "wrong" });
	`)
	if err == nil {
		t.Fatal("wrong password should throw")
	}
}

func TestStegoGoja_Capacity(t *testing.T) {
	vm := stegoVM(t)
	v, err := vm.RunString(`image.stego.capacity(carrier).bytes`)
	if err != nil {
		t.Fatal(err)
	}
	if v.ToInteger() <= 0 {
		t.Fatalf("capacity = %d, want > 0", v.ToInteger())
	}
}

func TestStegoGoja_DetectAnalyze(t *testing.T) {
	vm := stegoVM(t) // helper already in this file; provides image.stego.* + `carrier`
	v, err := vm.RunString(`
		const out = image.stego.embed(carrier, "hidden");
		const d = image.stego.detect(out.bytes);
		const a = image.stego.analyze(out.bytes);
		d.sercon + "|" + d.suspicious + "|" + (a.channels.length) + "|" + a.verdict.suspicious;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "true|true|3|true" {
		t.Fatalf("got %q (want true|true|3|true)", got)
	}
}

func TestStegoGoja_Bitplane(t *testing.T) {
	vm := stegoVM(t)
	v, err := vm.RunString(`
		const out = image.stego.bitplane(carrier, { channel: "rgb", plane: 0 });
		out.bytes.length > 0;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatal("bitplane should return non-empty PNG bytes")
	}
	if _, err := vm.RunString(`image.stego.bitplane(carrier, { plane: 9 })`); err == nil {
		t.Fatal("plane 9 must throw")
	}
	if _, err := vm.RunString(`image.stego.bitplane(carrier, { channel: "x" })`); err == nil {
		t.Fatal("bad channel must throw")
	}
}

func TestStegoGoja_AnalyzeMultiBit(t *testing.T) {
	vm := stegoVM(t)
	v, err := vm.RunString(`
		const out = image.stego.embed(carrier, "hidden", { bits: 2 });
		const d = image.stego.detect(out.bytes);
		const a = image.stego.analyze(out.bytes);
		d.bits + "|" + (a.channels[0].chiSquareByBits.length) + "|" + (a.channels[0].entropyByPlane.length) + "|" + (typeof a.estimatedBits);
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "2|4|4|number" {
		t.Fatalf("got %q (want 2|4|4|number)", got)
	}
}

func TestStegoGoja_MultiBit(t *testing.T) {
	vm := stegoVM(t)
	v, err := vm.RunString(`
		const cap1 = image.stego.capacity(carrier, { bits: 1 });
		const cap4 = image.stego.capacity(carrier, { bits: 4 });
		const out = image.stego.embed(carrier, "deep secret", { bits: 4 });
		const back = image.stego.extract(out.bytes);
		(cap4.bytes === cap1.bytes * 4) + "|" + cap4.bits + "|" + back;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "true|4|deep secret" {
		t.Fatalf("got %q (want true|4|deep secret)", got)
	}
	for _, bad := range []string{"0", "5", "2.5"} {
		if _, err := vm.RunString(`image.stego.embed(carrier, "x", { bits: ` + bad + ` })`); err == nil {
			t.Fatalf("bits:%s must throw", bad)
		}
	}
}
