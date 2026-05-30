package main

import (
	"context"
	"testing"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// The regex bindings build their { match, groups, index } result with a
// fixed key order so JSON.stringify(result) is byte-stable across runs —
// callers that hash a canonical serialization (payment request signing,
// webhook signature verification) depend on it. A Go map would shuffle
// the keys (Go randomizes map iteration, which goja surfaces as JS
// property order). These tests assert the exact serialized string; a
// regression to a map would fail them.

func runStringify(t *testing.T, script string) string {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := eng.Register("__record", func(call goja.FunctionCall) goja.Value {
		got = call.Argument(0).String()
		return goja.Undefined()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "keyorder.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
	return got
}

func TestPregMatch_StableKeyOrder(t *testing.T) {
	// No groups: { match, groups: [], index }.
	got := runStringify(t, `__record(JSON.stringify(text.preg.match("/foo/", "barfoo")));`)
	if want := `{"match":"foo","groups":[],"index":3}`; got != want {
		t.Fatalf("preg.match key order:\n got: %s\nwant: %s", got, want)
	}
	// With groups: groups array sits between match and index.
	got = runStringify(t, `__record(JSON.stringify(text.preg.match("/(\\w+) (\\d+)/", "x alice 30")));`)
	if want := `{"match":"alice 30","groups":["alice","30"],"index":2}`; got != want {
		t.Fatalf("preg.match (grouped) key order:\n got: %s\nwant: %s", got, want)
	}
}

func TestPreg2Match_StableKeyOrder(t *testing.T) {
	got := runStringify(t, `__record(JSON.stringify(text.preg2.match("/foo/", "barfoo")));`)
	if want := `{"match":"foo","groups":[],"index":3}`; got != want {
		t.Fatalf("preg2.match key order:\n got: %s\nwant: %s", got, want)
	}
	got = runStringify(t, `__record(JSON.stringify(text.preg2.match("/(\\w+) (\\d+)/", "x alice 30")));`)
	if want := `{"match":"alice 30","groups":["alice","30"],"index":2}`; got != want {
		t.Fatalf("preg2.match (grouped) key order:\n got: %s\nwant: %s", got, want)
	}
}

// matchAll must keep the same per-element key order.
func TestPregMatchAll_StableKeyOrder(t *testing.T) {
	got := runStringify(t, `__record(JSON.stringify(text.preg.matchAll("/(\\d+)/", "a1 b2")));`)
	if want := `[{"match":"1","groups":["1"],"index":1},{"match":"2","groups":["2"],"index":4}]`; got != want {
		t.Fatalf("preg.matchAll key order:\n got: %s\nwant: %s", got, want)
	}
}
