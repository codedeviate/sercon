package main

import (
	"testing"

	"github.com/dop251/goja"
)

// newVM is a bare goja runtime for bridge unit tests (no engine needed).
func newVM() *goja.Runtime { return goja.New() }

func mustEval(t *testing.T, vm *goja.Runtime, src string) goja.Value {
	t.Helper()
	v, err := vm.RunString(src)
	if err != nil {
		t.Fatalf("setup script: %v", err)
	}
	return v
}

func TestJSToIR_Scalars(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	cases := []struct {
		src  string
		kind dumpKind
	}{
		{"null", dumpNull},
		{"undefined", dumpNull},
		{"true", dumpBool},
		{"42", dumpInt},
		{"3.0", dumpInt}, // integral float collapses to int
		{"3.14", dumpFloat},
		{`"hi"`, dumpString},
	}
	for _, c := range cases {
		n, err := jsToIR(vm, mustEval(t, vm, c.src), opts)
		if err != nil {
			t.Fatalf("%s: %v", c.src, err)
		}
		if n.kind != c.kind {
			t.Errorf("%s: kind = %d, want %d", c.src, n.kind, c.kind)
		}
	}
}

func TestJSToIR_ArrayVsObjectVsClass(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})

	arr, _ := jsToIR(vm, mustEval(t, vm, `[1,2,3]`), opts)
	if arr.kind != dumpArray || len(arr.items) != 3 {
		t.Fatalf("array: kind=%d len=%d", arr.kind, len(arr.items))
	}

	obj, _ := jsToIR(vm, mustEval(t, vm, `({b:2, a:1})`), opts)
	if obj.kind != dumpMap || len(obj.pairs) != 2 || obj.pairs[0].key != "b" {
		t.Fatalf("object: kind=%d pairs=%v", obj.kind, obj.pairs) // insertion order: b then a
	}

	cls, _ := jsToIR(vm, mustEval(t, vm, `({__class:"Point", x:1, y:2})`), opts)
	if cls.kind != dumpClass || cls.class != "Point" || len(cls.pairs) != 2 {
		t.Fatalf("classed: kind=%d class=%q pairs=%d", cls.kind, cls.class, len(cls.pairs))
	}
	if cls.pairs[0].key != "x" { // __class is consumed, not emitted as a pair
		t.Fatalf("classed first key = %q, want x", cls.pairs[0].key)
	}
}

func TestJSToIR_SharedRefReused(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	// shared (acyclic) child appears twice
	n, err := jsToIR(vm, mustEval(t, vm, `(function(){var c={k:1}; return [c,c];})()`), opts)
	if err != nil {
		t.Fatal(err)
	}
	if n.items[0] != n.items[1] {
		t.Fatal("shared child should be the same *irNode pointer")
	}
}

func TestJSToIR_CycleThrows(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	_, err := jsToIR(vm, mustEval(t, vm, `(function(){var a={}; a.self=a; return a;})()`), opts)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestJSToIR_UnsupportedThrows(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	for _, src := range []string{`(function(){})`, `Symbol("x")`, `10n`} {
		if _, err := jsToIR(vm, mustEval(t, vm, src), opts); err == nil {
			t.Errorf("%s: expected unsupported-type error", src)
		}
	}
}

func TestJSToIR_RejectsExoticObjects(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	for _, src := range []string{
		`new Date(0)`,
		`/abc/g`,
		`new Map([["a",1]])`,
		`new Set([1,2])`,
	} {
		if _, err := jsToIR(vm, mustEval(t, vm, src), opts); err == nil {
			t.Errorf("%s: expected unsupported-type error", src)
		}
	}
}

func TestJSToIR_EmptyContainers(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})

	arr, err := jsToIR(vm, mustEval(t, vm, `[]`), opts)
	if err != nil {
		t.Fatalf("empty array: %v", err)
	}
	if arr.kind != dumpArray || len(arr.items) != 0 {
		t.Fatalf("empty array: kind=%d len=%d", arr.kind, len(arr.items))
	}

	obj, err := jsToIR(vm, mustEval(t, vm, `({})`), opts)
	if err != nil {
		t.Fatalf("empty object: %v", err)
	}
	if obj.kind != dumpMap || len(obj.pairs) != 0 {
		t.Fatalf("empty object: kind=%d pairs=%d", obj.kind, len(obj.pairs))
	}
}

func TestJSToIR_AcceptsClassInstance(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	n, err := jsToIR(vm, mustEval(t, vm, `(function(){class Foo{}; var f=new Foo(); f.a=1; return f;})()`), opts)
	if err != nil {
		t.Fatalf("class instance: %v", err)
	}
	if n.kind != dumpMap || len(n.pairs) != 1 || n.pairs[0].key != "a" {
		t.Fatalf("class instance: kind=%d pairs=%v", n.kind, n.pairs)
	}
}

func TestIRToJS_RoundTripStableOrder(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	in := mustEval(t, vm, `({z:1, a:2, nested:{y:[1,2], x:3}})`)
	n, err := jsToIR(vm, in, opts)
	if err != nil {
		t.Fatal(err)
	}
	out := irToJS(vm, n, opts)
	vm.Set("__out", out)
	got := mustEval(t, vm, `JSON.stringify(__out)`).String()
	if want := `{"z":1,"a":2,"nested":{"y":[1,2],"x":3}}`; got != want {
		t.Fatalf("round-trip order:\n got: %s\nwant: %s", got, want)
	}
}

func TestIRToJS_SharedRefRebuilt(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	shared := &irNode{kind: dumpMap, pairs: []irPair{{"k", nodeInt(1)}}}
	root := &irNode{kind: dumpArray, items: []*irNode{shared, shared}}
	out := irToJS(vm, root, opts)
	vm.Set("__out", out)
	if got := mustEval(t, vm, `__out[0] === __out[1]`).ToBoolean(); !got {
		t.Fatal("shared IR node should rebuild as the same JS object")
	}
}

func TestIRToJS_ClassSentinel(t *testing.T) {
	vm := newVM()
	opts := withDumpDefaults(dumpOpts{})
	n := &irNode{kind: dumpClass, class: "Point", pairs: []irPair{{"x", nodeInt(1)}}}
	out := irToJS(vm, n, opts)
	vm.Set("__out", out)
	if got := mustEval(t, vm, `JSON.stringify(__out)`).String(); got != `{"__class":"Point","x":1}` {
		t.Fatalf("classed round-trip: %s", got)
	}
}
