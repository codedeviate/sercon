// cmd/sercon/codec_yaml_test.go
package main

import (
	"testing"

	"github.com/dop251/goja"
)

func yamlVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	obj := vm.NewObject()
	for k, v := range yamlNamespace(vm) {
		_ = obj.Set(k, v)
	}
	_ = vm.Set("yaml", obj)
	return vm
}

func TestYamlParse(t *testing.T) {
	vm := yamlVM(t)
	v, err := vm.RunString(`
		const o = yaml.parse("a: 1\nb: [x, y]\nnested:\n  on: true\n");
		o.a + "|" + o.b.length + "|" + o.b[1] + "|" + o.nested.on;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "1|2|y|true" {
		t.Fatalf("got %q (want 1|2|y|true)", got)
	}
}

func TestYamlStringifyRoundTrip(t *testing.T) {
	vm := yamlVM(t)
	v, err := vm.RunString(`
		const text = yaml.stringify({ name: "x", nums: [1, 2, 3], nested: { on: true } });
		const back = yaml.parse(text);
		back.name + "|" + back.nums[2] + "|" + back.nested.on;
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "x|3|true" {
		t.Fatalf("got %q (want x|3|true)", got)
	}
}

// A top-level sequence must parse to a JS array, not just mappings.
func TestYamlParseTopLevelSequence(t *testing.T) {
	vm := yamlVM(t)
	v, err := vm.RunString(`
		const arr = yaml.parse("- a\n- b\n- c\n");
		Array.isArray(arr) + "|" + arr.length + "|" + arr[2];
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "true|3|c" {
		t.Fatalf("got %q (want true|3|c)", got)
	}
}

// A non-string mapping key must still be reachable as an object property.
func TestYamlParseNonStringKey(t *testing.T) {
	vm := yamlVM(t)
	v, err := vm.RunString(`
		const o = yaml.parse("1: one\ntrue: yes\n");
		o["1"] + "|" + o["true"];
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != "one|yes" {
		t.Fatalf("got %q (want one|yes)", got)
	}
}

func TestYamlParseMalformedThrows(t *testing.T) {
	vm := yamlVM(t)
	if _, err := vm.RunString(`yaml.parse("a: b: c: [unbalanced");`); err == nil {
		t.Fatal("malformed yaml should throw")
	}
}
