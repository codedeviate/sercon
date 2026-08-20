package main

import (
	"context"
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// The console shim must route log/info/debug to stdout and warn/error to
// stderr, with clean space-joined output (no Go-logger timestamp prefix).
func TestConsole_RoutesAndCleans(t *testing.T) {
	out, errb, restore := withCapturedStdio(t)
	defer restore()

	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "c.ts", `
		console.log("a", 1, true);
		console.info("i");
		console.debug("d");
		console.warn("w");
		console.error("e", 2);
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "a 1 true\ni\nd\n"; got != want {
		t.Fatalf("stdout:\n got: %q\nwant: %q", got, want)
	}
	if got, want := errb.String(), "w\ne 2\n"; got != want {
		t.Fatalf("stderr:\n got: %q\nwant: %q", got, want)
	}
}

// runConsoleScript runs src with the console shim captured, returning stdout.
func runConsoleScript(t *testing.T, src string) string {
	t.Helper()
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "c.ts", src); err != nil {
		t.Fatalf("run: %v", err)
	}
	return out.String()
}

// console.table renders an array of objects as an aligned, bordered table with
// a leading (index) column.
func TestConsoleTable_ArrayOfObjects(t *testing.T) {
	got := runConsoleScript(t, `console.table([{a:1,b:2},{a:3,b:4}]);`)
	want := strings.Join([]string{
		"┌─────────┬───┬───┐",
		"│ (index) │ a │ b │",
		"├─────────┼───┼───┤",
		"│    0    │ 1 │ 2 │",
		"│    1    │ 3 │ 4 │",
		"└─────────┴───┴───┘",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("table:\n got:\n%s\nwant:\n%s", got, want)
	}
}

// The columns argument restricts and orders the property columns.
func TestConsoleTable_ColumnsRestrict(t *testing.T) {
	got := runConsoleScript(t, `console.table([{name:"a",status:"ok",extra:9}], ["status","name"]);`)
	want := strings.Join([]string{
		"┌─────────┬────────┬──────┐",
		"│ (index) │ status │ name │",
		"├─────────┼────────┼──────┤",
		"│    0    │   ok   │  a   │",
		"└─────────┴────────┴──────┘",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("table:\n got:\n%s\nwant:\n%s", got, want)
	}
}

// A primitive array gets a Values column; missing keys render blank.
func TestConsoleTable_PrimitivesAndValues(t *testing.T) {
	got := runConsoleScript(t, `console.table(["x", {a:1}]);`)
	want := strings.Join([]string{
		"┌─────────┬───┬────────┐",
		"│ (index) │ a │ Values │",
		"├─────────┼───┼────────┤",
		"│    0    │   │   x    │",
		"│    1    │ 1 │        │",
		"└─────────┴───┴────────┘",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("table:\n got:\n%s\nwant:\n%s", got, want)
	}
}

// Non-tabular input falls back to a console.log-style line without throwing.
func TestConsoleTable_NonTabularFallback(t *testing.T) {
	if got, want := runConsoleScript(t, `console.table(42);`), "42\n"; got != want {
		t.Fatalf("fallback: got %q want %q", got, want)
	}
}
