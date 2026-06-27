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
