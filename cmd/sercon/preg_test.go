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

// runPregScript wires the `preg` namespace into a fresh engine + a
// `__capture` side-channel function that the script calls with its
// computed value. scriptengine's Run always resolves to undefined
// (top-level expression capture is on the backlog), so we route the
// answer through a registered function instead. Drives the binding
// through its real goja.Runtime path (not a direct Go call) so the
// JS-side coercions are exercised the same way scripts see them.
func runPregScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot: t.TempDir(),
		Timeout:    2 * time.Second,
	})
	if err := eng.RegisterNamespaceFactory("preg", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return pregNamespace(vm)
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
		t.Fatalf("register __capture: %v", err)
	}

	src := body + "\n__capture(__result);"
	if _, err := eng.Run(context.Background(), "preg.ts", src); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// preg.match returns null when the pattern fails to find anything,
// rather than e.g. an empty object — that keeps `if (m) {...}`
// idiomatic at the call site.
func TestPregMatch_NullOnNoMatch(t *testing.T) {
	got := runPregScript(t, `const __result = preg.match("/xyz/", "hello world");`)
	if got != nil {
		t.Fatalf("expected nil (null), got %#v", got)
	}
}

// First match's full text + groups + index round-trip cleanly. We
// read fields one at a time rather than JSON-stringifying because
// JS-side stringify uses insertion order, but `match` was built from
// a Go map[string]any whose key iteration order isn't stable.
func TestPregMatch_FirstMatchAndGroups(t *testing.T) {
	got := runPregScript(t, `
		const m = preg.match("/(\\w+) (\\d+)/i", "ID alpha 42 and beta 9");
		const __result = [m.match, m.groups.join("|"), m.index].join(",");
	`)
	want := "alpha 42,alpha|42,3"
	if got != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// matchAll drains every match — unlike match which returns just the
// first. Verifies the slice is the right length and the groups are
// addressable.
func TestPregMatchAll_EveryHit(t *testing.T) {
	got := runPregScript(t, `
		const xs = preg.matchAll("/(\\w+)=(\\d+)/", "a=1 b=2 c=3");
		const __result = JSON.stringify(xs.map(x => x.match));
	`)
	want := `["a=1","b=2","c=3"]`
	if got != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// replace substitutes via `$1` / `${1}` backreferences (Go's syntax,
// documented in the prose).
func TestPregReplace_Backreferences(t *testing.T) {
	got := runPregScript(t, `
		const out = preg.replace("/(\\w+)@(\\w+)/", "$2/$1", "alice@example bob@here");
		const __result = out;
	`)
	want := "example/alice here/bob"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// Case-insensitive (`i`), multiline (`m`), and dotall (`s`) flags all
// land on the Go-side `(?ims)` inline prefix.
func TestPregFlags_Supported(t *testing.T) {
	// `i` — case-insensitive
	if got := runPregScript(t, `const __result = preg.match("/hello/i", "HELLO") !== null;`); got != true {
		t.Errorf("`i` flag did not match HELLO; got %#v", got)
	}
	// `m` — `^` anchors at line starts
	if got := runPregScript(t, `const __result = preg.matchAll("/^x/m", "x\nx\nx").length;`); got != int64(3) {
		t.Errorf("`m` flag count: %#v (want 3)", got)
	}
	// `s` — `.` matches newline
	if got := runPregScript(t, `const __result = preg.match("/a.b/s", "a\nb") !== null;`); got != true {
		t.Errorf("`s` flag did not match across newline; got %#v", got)
	}
}

// PHP flags that don't translate to RE2 must throw with a clear,
// named-flag error rather than silently dropping. `u`, `U`, `x`
// each have dedicated wording so users know what to do.
func TestPregFlags_UnsupportedFlagsThrow(t *testing.T) {
	cases := []struct {
		pattern string
		hint    string
	}{
		{"/abc/u", "Unicode"},
		{"/abc/U", "ungreedy"},
		{"/abc/x", "extended"},
		{"/abc/z", "unknown flag"},
	}
	for _, tc := range cases {
		t.Run(tc.pattern, func(t *testing.T) {
			eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
			if err := eng.RegisterNamespaceFactory("preg", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
				return pregNamespace(vm)
			}); err != nil {
				t.Fatal(err)
			}
			_, err := eng.Run(context.Background(), "x.ts", `preg.match("`+tc.pattern+`", "abc");`)
			if err == nil {
				t.Fatalf("expected throw for %q", tc.pattern)
			}
			if !strings.Contains(err.Error(), tc.hint) {
				t.Errorf("error wording for %q: %v (want hint %q)", tc.pattern, err, tc.hint)
			}
		})
	}
}

// Malformed patterns — missing leading slash, missing closing slash,
// invalid regex body — must all throw with sercon's own error prefix
// rather than the bare RE2 message.
func TestPregMatch_MalformedPattern(t *testing.T) {
	cases := []string{
		"abc",       // no leading slash
		"/abc",      // no closing slash
		"/(unclos/", // valid delimiters but the body is RE2-invalid
	}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
			if err := eng.RegisterNamespaceFactory("preg", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
				return pregNamespace(vm)
			}); err != nil {
				t.Fatal(err)
			}
			_, err := eng.Run(context.Background(), "x.ts", `preg.match("`+p+`", "abc");`)
			if err == nil {
				t.Fatalf("expected throw for %q", p)
			}
			if !strings.Contains(err.Error(), "preg:") {
				t.Errorf("error should be prefixed `preg:`; got %v", err)
			}
		})
	}
}

// Optional groups that didn't match show up as empty strings rather
// than `undefined`. Stable-shape over JS-RegExp parity here.
func TestPregMatch_OptionalGroupEmpty(t *testing.T) {
	got := runPregScript(t, `
		const m = preg.match("/(a)(b)?(c)/", "ac");
		const __result = JSON.stringify(m.groups);
	`)
	want := `["a","","c"]`
	if got != want {
		t.Errorf("got %v, want %s", got, want)
	}
}
