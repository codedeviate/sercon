package main

import (
	"context"
	"sort"
	"testing"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestTopLevelGlobals_Shape asserts the script-facing surface is exactly
// the nine reserved top-level globals — and crucially that `api` is
// NOT present. Guards against accidental re-introduction of the old
// wrapper or drift adding a 10th reserved name.
func TestTopLevelGlobals_Shape(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var keys []string
	var apiPresent bool
	if err := eng.Register("__recordKeys", func(call goja.FunctionCall) goja.Value {
		arr, ok := call.Argument(0).Export().([]any)
		if !ok {
			t.Errorf("__recordKeys: expected []any, got %T", call.Argument(0).Export())
			return goja.Undefined()
		}
		keys = make([]string, 0, len(arr))
		for _, v := range arr {
			s, _ := v.(string)
			keys = append(keys, s)
		}
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__recordApi", func(call goja.FunctionCall) goja.Value {
		apiPresent = !goja.IsUndefined(call.Argument(0))
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "shape.ts", `
const wanted = ["crypto", "db", "codec", "fs", "net", "runtime", "text", "services", "tui"];
const present = wanted.filter(n => typeof globalThis[n] !== "undefined");
__recordKeys(present);
__recordApi(typeof globalThis["api"] === "undefined" ? undefined : globalThis["api"]);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	sort.Strings(keys)
	want := []string{"codec", "crypto", "db", "fs", "net", "runtime", "services", "text", "tui"}
	if len(keys) != len(want) {
		t.Fatalf("reserved globals: got %v, want %v", keys, want)
	}
	for i, k := range keys {
		if k != want[i] {
			t.Fatalf("reserved globals: got %v, want %v", keys, want)
		}
	}
	if apiPresent {
		t.Fatal("`api` global must not exist after v0.9.0; it was found at globalThis")
	}
}
