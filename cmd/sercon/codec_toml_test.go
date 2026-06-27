// cmd/sercon/codec_toml_test.go
package main

import (
	"testing"

	"github.com/dop251/goja"
)

func tomlVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	obj := vm.NewObject()
	for k, v := range tomlNamespace(vm) {
		_ = obj.Set(k, v)
	}
	_ = vm.Set("toml", obj)
	return vm
}

func TestTomlParse(t *testing.T) {
	vm := tomlVM(t)
	v, err := vm.RunString(`
		const o = toml.parse('title = "demo"\n[server]\nport = 8080\nhosts = ["a","b"]\n');
		o.title + "|" + o.server.port + "|" + o.server.hosts.length + "|" + o.server.hosts[1];
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "demo|8080|2|b" {
		t.Fatalf("got %q (want demo|8080|2|b)", got)
	}
}

func TestTomlStringifyRoundTrip(t *testing.T) {
	vm := tomlVM(t)
	v, err := vm.RunString(`
		const text = toml.stringify({ name: "x", nums: [1,2,3], nested: { on: true } });
		const back = toml.parse(text);
		back.name + "|" + back.nums[2] + "|" + back.nested.on;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "x|3|true" {
		t.Fatalf("got %q (want x|3|true)", got)
	}
}

func TestTomlParseMalformedThrows(t *testing.T) {
	vm := tomlVM(t)
	if _, err := vm.RunString(`toml.parse("this is = = not toml");`); err == nil {
		t.Fatal("malformed toml should throw")
	}
}
