package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// The console shim must route log/info/debug to stdout and warn/error to
// stderr, with clean space-joined output (no Go-logger timestamp prefix).
func TestConsole_RoutesAndCleans(t *testing.T) {
	var out, errb bytes.Buffer
	oldOut, oldErr := consoleOut, consoleErr
	consoleOut, consoleErr = &out, &errb
	defer func() { consoleOut, consoleErr = oldOut, oldErr }()

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
