// cmd/sercon/text_stego_test.go
package main

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestTextStego_RoundTrip(t *testing.T) {
	cover := "The quick brown fox.\nSecond line."
	out, err := textStegoEmbed(cover, []byte("attack at dawn"), true, "")
	if err != nil {
		t.Fatal(err)
	}
	// The visible text (zero-width chars stripped) is unchanged.
	stripped := strings.NewReplacer("\u200b", "", "\u200c", "").Replace(out)
	if stripped != cover {
		t.Fatalf("visible text changed: %q", stripped)
	}
	data, isText, err := textStegoExtract(out, "")
	if err != nil {
		t.Fatal(err)
	}
	if !isText || string(data) != "attack at dawn" {
		t.Fatalf("got %q isText=%v", data, isText)
	}
}

func TestTextStego_BinaryAndPassword(t *testing.T) {
	out, err := textStegoEmbed("cover", []byte{9, 8, 7, 0, 255}, false, "pw")
	if err != nil {
		t.Fatal(err)
	}
	data, isText, err := textStegoExtract(out, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if isText || string(data) != string([]byte{9, 8, 7, 0, 255}) {
		t.Fatalf("binary round-trip failed: %v", data)
	}
	if _, _, err := textStegoExtract(out, "wrong"); err == nil {
		t.Fatal("wrong password should error")
	}
}

func TestTextStego_NoPayloadErrors(t *testing.T) {
	if _, _, err := textStegoExtract("just plain text, no hidden run", ""); err == nil {
		t.Fatal("plain text should error (no sercon payload)")
	}
}

func TestTextStegoGoja_RoundTrip(t *testing.T) {
	vm := goja.New()
	obj := vm.NewObject()
	for k, v := range textStegoNamespace(vm) {
		_ = obj.Set(k, v)
	}
	tns := vm.NewObject()
	_ = tns.Set("stego", obj)
	_ = vm.Set("text", tns)
	v, err := vm.RunString(`
		const out = text.stego.embed("cover text", "secret", { password: "pw" });
		text.stego.extract(out, { password: "pw" });
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "secret" {
		t.Fatalf("got %q", v.String())
	}
}
