package main

import (
	"context"
	"sort"
	"testing"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// The top-level api.* surface must be exactly the 9 category buckets.
// Guards against future drift introducing a 10th top-level key (which
// would be a tacit re-flattening of the carefully-bucketed surface).
func TestAPI_TopLevelShape(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerExampleAPI(eng); err != nil {
		t.Fatal(err)
	}
	// A small recorder binding captures the keys of `api` into Go.
	var keys []string
	if err := eng.Register("__recordAPIKeys", func(call goja.FunctionCall) goja.Value {
		arr, ok := call.Argument(0).Export().([]any)
		if !ok {
			t.Errorf("__recordAPIKeys: expected []any, got %T", call.Argument(0).Export())
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
	_, err := eng.Run(context.Background(), "shape.ts", `
const ks = Object.keys(api);
__recordAPIKeys(ks);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	sort.Strings(keys)
	want := []string{"crypto", "db", "format", "fs", "net", "runtime", "text", "tools", "ui"}
	if len(keys) != len(want) {
		t.Fatalf("top-level api keys: got %v, want %v", keys, want)
	}
	for i, k := range keys {
		if k != want[i] {
			t.Fatalf("top-level api keys: got %v, want %v", keys, want)
		}
	}
}
