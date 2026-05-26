package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func runPreg2Script(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("preg2", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return preg2Namespace(vm)
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "p2.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// Lookahead — the headline PCRE feature RE2 can't do. "foo" only
// when followed by "bar".
func TestPreg2_Lookahead(t *testing.T) {
	got := runPreg2Script(t, `
		const m = preg2.match("/foo(?=bar)/", "foobar foobaz");
		const __result = m === null ? "null" : [m.match, m.index].join(",");
	`)
	if got != "foo,0" {
		t.Errorf("lookahead: %v", got)
	}
}

// Lookbehind — match digits preceded by a $.
func TestPreg2_Lookbehind(t *testing.T) {
	got := runPreg2Script(t, `
		const m = preg2.match("/(?<=\\$)\\d+/", "total: $42 owed");
		const __result = m.match;
	`)
	if got != "42" {
		t.Errorf("lookbehind: %v", got)
	}
}

// Backreference inside the pattern — a doubled word.
func TestPreg2_Backreference(t *testing.T) {
	got := runPreg2Script(t, `
		const m = preg2.match("/(\\w+) \\1/", "the the end");
		const __result = [m.match, m.groups[0]].join(",");
	`)
	if got != "the the,the" {
		t.Errorf("backreference: %v", got)
	}
}

// matchAll drains every hit, same shape as preg.
func TestPreg2_MatchAll(t *testing.T) {
	got := runPreg2Script(t, `
		const xs = preg2.matchAll("/\\d+/", "a1 b22 c333");
		const __result = xs.map(m => m.match).join(",");
	`)
	if got != "1,22,333" {
		t.Errorf("matchAll: %v", got)
	}
}

// replace uses .NET / regexp2 $1 substitution syntax.
func TestPreg2_Replace(t *testing.T) {
	got := runPreg2Script(t, `
		const __result = preg2.replace("/(\\w+)@(\\w+)/", "$2/$1", "alice@corp bob@dept");
	`)
	if got != "corp/alice dept/bob" {
		t.Errorf("replace: %v", got)
	}
}

// Flags i / m / s / x all map onto regexp2 options. `x` is the one
// RE2 (api.preg) couldn't support.
func TestPreg2_Flags(t *testing.T) {
	if got := runPreg2Script(t, `const __result = preg2.match("/HELLO/i", "hello") !== null;`); got != true {
		t.Errorf("i flag: %v", got)
	}
	if got := runPreg2Script(t, `const __result = preg2.matchAll("/^x/m", "x\nx\nx").length;`); got != int64(3) {
		t.Errorf("m flag: %v", got)
	}
	if got := runPreg2Script(t, `const __result = preg2.match("/a.b/s", "a\nb") !== null;`); got != true {
		t.Errorf("s flag: %v", got)
	}
	// x: whitespace in the pattern is ignored, so "/ \d+ /x" matches "123".
	if got := runPreg2Script(t, `const __result = preg2.match("/ \\d+ /x", "abc123") !== null;`); got != true {
		t.Errorf("x flag: %v", got)
	}
}

// null on no match (like preg).
func TestPreg2_NullOnNoMatch(t *testing.T) {
	got := runPreg2Script(t, `const __result = preg2.match("/zzz/", "abc");`)
	if got != nil {
		t.Errorf("expected nil, got %#v", got)
	}
}

// Unsupported flags and malformed patterns throw with the preg2: prefix.
func TestPreg2_Errors(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
	if err := eng.RegisterNamespaceFactory("preg2", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return preg2Namespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ src, hint string }{
		{`preg2.match("/abc/u", "abc");`, "Unicode"},
		{`preg2.match("/abc/z", "abc");`, "unknown flag"},
		{`preg2.match("abc", "abc");`, "must start with"},
		{`preg2.match("/abc", "abc");`, "closing"},
		{`preg2.match("/(/", "abc");`, "preg2:"},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			_, err := eng.Run(context.Background(), "x.ts", tc.src)
			if err == nil {
				t.Fatalf("expected throw")
			}
			if !strings.Contains(err.Error(), tc.hint) {
				t.Errorf("err wording: %v (want %q)", err, tc.hint)
			}
		})
	}
}

// Optional group that didn't participate surfaces as empty string —
// same stable-shape policy as preg.
func TestPreg2_OptionalGroupEmpty(t *testing.T) {
	got := runPreg2Script(t, `
		const m = preg2.match("/(a)(b)?(c)/", "ac");
		const __result = JSON.stringify(m.groups);
	`)
	if got != `["a","","c"]` {
		t.Errorf("optional group: %v", got)
	}
}
