package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// In non-TTY mode the binding routes pane writes to the fallback writer.
// We exercise the full layer: api.ui.tui.layout → api.ui.tui.pane → pane.writeln.
// The factory uses os.Stdout's TTY-ness to decide; we capture the
// fallback output via withTestStdout which the binding consults instead
// of os.Stdout when set.
func TestAPITUI_FallbackEndToEnd(t *testing.T) {
	var captured bytes.Buffer
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
	})
	withTestStdout(&captured, func() {
		if err := registerExampleAPI(eng); err != nil {
			t.Fatal(err)
		}
		_, err := eng.Run(context.Background(), "run.ts", `
api.ui.tui.layout({rows: [
  { name: "log" },
  { name: "brew" },
]});
const log = api.ui.tui.pane("log");
const brew = api.ui.tui.pane("brew");
log.writeln("hello");
brew.writeln("upgrading");
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	got := captured.String()
	for _, want := range []string{
		"[log] hello\n",
		"[brew] upgrading\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %q in captured stdout; got:\n%s", want, got)
		}
	}
}

// Layout validation propagates from the package into a JS throw.
func TestAPITUI_LayoutValidationThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerExampleAPI(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
try {
  api.ui.tui.layout({rows: [{name: "x"}, {name: "x"}]});
  throw new Error("expected validation error");
} catch (e) {
  if (!String(e).includes("duplicate pane name")) {
    throw new Error("got: " + e);
  }
}
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
}

// Pane handle for an unknown pane throws.
func TestAPITUI_UnknownPaneThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerExampleAPI(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
api.ui.tui.layout({name: "only"});
try {
  api.ui.tui.pane("missing");
  throw new Error("expected throw");
} catch (e) {
  if (!String(e).includes("unknown pane")) throw new Error("got: " + e);
}
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
}

// Calling layout twice in the same Run throws.
func TestAPITUI_LayoutOnceThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerExampleAPI(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
api.ui.tui.layout({name: "a"});
try {
  api.ui.tui.layout({name: "b"});
  throw new Error("expected throw");
} catch (e) {
  if (!String(e).includes("layout already declared")) throw new Error("got: " + e);
}
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
}

// WatchMode rejects api.ui.tui.layout.
func TestAPITUI_WatchModeRejectsLayout(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		WatchMode:      true,
	})
	if err := registerExampleAPI(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
try {
  api.ui.tui.layout({name: "a"});
  throw new Error("expected throw");
} catch (e) {
  if (!String(e).includes("not supported under --watch")) throw new Error("got: " + e);
}
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
}
