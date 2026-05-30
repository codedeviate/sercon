package main

import (
	"context"
	"testing"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func TestFormatValue(t *testing.T) {
	vm := goja.New()
	run := func(src string) goja.Value {
		v, err := vm.RunString(src)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	cases := []struct{ src, want string }{
		{`"hello"`, "hello"},     // strings print raw, not JSON-quoted
		{`42`, "42"},             // number
		{`true`, "true"},         // bool
		{`null`, "null"},         // null
		{`({a:1, b:[2,3]})`, `{"a":1,"b":[2,3]}`}, // object → JSON
		{`[1,"x",true]`, `[1,"x",true]`},          // array → JSON
	}
	for _, c := range cases {
		if got := formatValue(vm, run(c.src)); got != c.want {
			t.Errorf("formatValue(%s) = %q, want %q", c.src, got, c.want)
		}
	}
	// A circular object can't be JSON-stringified; formatValue must fall back
	// (to String()) rather than panic or return empty.
	circ := run(`(function(){var a={}; a.self=a; return a;})()`)
	if got := formatValue(vm, circ); got == "" {
		t.Error("circular value should format to a non-empty fallback")
	}
}

func TestAssertEqual_DeepEquality(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	// Distinct objects/arrays with identical contents must compare equal.
	if _, err := eng.Run(context.Background(), "ok.ts", `
		runtime.assert.equal({a:1, b:[2,{c:3}]}, {a:1, b:[2,{c:3}]});
		runtime.assert.equal([1,2,3], [1,2,3]);
		runtime.assert.equal("x", "x");
		runtime.assert.equal(5, 5);
	`); err != nil {
		t.Fatalf("deep-equal asserts should pass: %v", err)
	}
	// Differing contents still throw.
	if _, err := eng.Run(context.Background(), "bad.ts", `runtime.assert.equal({a:1}, {a:2});`); err == nil {
		t.Fatal("assert.equal of differing objects should throw")
	}
}
