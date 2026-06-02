package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/codedeviate/sercon/pkg/scriptengine"
	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

// In non-TTY mode the binding routes pane writes to the fallback writer.
// We exercise the full layer: tui.layout → tui.pane → pane.writeln.
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
		if err := registerSurface(eng); err != nil {
			t.Fatal(err)
		}
		_, err := eng.Run(context.Background(), "run.ts", `
tui.layout({rows: [
  { name: "log" },
  { name: "brew" },
]});
const log = tui.pane("log");
const brew = tui.pane("brew");
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
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
try {
  tui.layout({rows: [{name: "x"}, {name: "x"}]});
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
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
tui.layout({name: "only"});
try {
  tui.pane("missing");
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
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
tui.layout({name: "a"});
try {
  tui.layout({name: "b"});
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

// WatchMode rejects tui.layout.
func TestAPITUI_WatchModeRejectsLayout(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		WatchMode:      true,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
try {
  tui.layout({name: "a"});
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

// initCountingScreen wraps a SimulationScreen and counts Init calls.
type initCountingScreen struct {
	tcell.SimulationScreen
	inits int32
}

func (s *initCountingScreen) Init() error {
	atomic.AddInt32(&s.inits, 1)
	return s.SimulationScreen.Init()
}

// TestStartControllerScreen_InitsScreenExactlyOnce guards against the
// v0.11.0 double-init bug: startControllerScreen must NOT call screen.Init()
// itself — Controller.StartScreen → tview SetScreen inits it once. Two
// Inits make tcell save the already-raw termios as the restore state, so
// Fini() leaves the terminal in raw mode and the next TUI run hangs.
func TestStartControllerScreen_InitsScreenExactlyOnce(t *testing.T) {
	cs := &initCountingScreen{SimulationScreen: tcell.NewSimulationScreen("")}
	orig := tuiNewScreen
	tuiNewScreen = func() (tcell.Screen, error) { return cs, nil }
	defer func() { tuiNewScreen = orig }()

	root, err := tui.ParseLayout(map[string]any{"rows": []any{
		map[string]any{"name": "a"},
		map[string]any{"name": "b"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := startControllerScreen(c); err != nil {
		t.Fatalf("startControllerScreen: %v", err)
	}
	c.Stop()

	if n := atomic.LoadInt32(&cs.inits); n != 1 {
		t.Fatalf("screen Init called %d times, want exactly 1 (double-init leaves the terminal in raw mode after teardown)", n)
	}
}

func TestTUIBinding_OnKeyRegistersAndUnsubscribes(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	var err error
	withTestStdout(&captured, func() {
		_, err = eng.Run(context.Background(), "onkey.ts", `
tui.layout({ rows: [ { name: "log" } ] });
const off = tui.onKey((k) => { /* never fires in fallback */ });
if (typeof off !== "function") throw new Error("onKey must return an unsubscribe function");
off(); // must not throw
`)
	})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
}

func TestTUIBinding_OnKeyBeforeLayoutThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	var err error
	withTestStdout(&captured, func() {
		_, err = eng.Run(context.Background(), "onkey-early.ts", `tui.onKey(() => {});`)
	})
	if err == nil {
		t.Fatal("expected onKey-before-layout to throw")
	}
}

func TestTUIBinding_WaitKeyRejectsInFallback(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	var err error
	withTestStdout(&captured, func() {
		_, err = eng.Run(context.Background(), "waitkey.ts", `
tui.layout({ rows: [ { name: "log" } ] });
let threw = false;
try { await tui.waitKey(); } catch (e) { threw = true; }
if (!threw) throw new Error("waitKey should reject in non-TTY mode");
`)
	})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
}

func TestTUIBinding_WaitKeyBeforeLayoutThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	var err error
	withTestStdout(&captured, func() {
		_, err = eng.Run(context.Background(), "waitkey-early.ts", `await tui.waitKey();`)
	})
	if err == nil {
		t.Fatal("expected waitKey-before-layout to reject/throw")
	}
}

// TestTUIBinding_AbortRunEndsScript verifies that calling eng.AbortRun()
// from a goroutine while a TUI script is parked on a long timer ends the
// Run promptly and returns context.Canceled.
func TestTUIBinding_AbortRunEndsScript(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	var err error
	start := time.Now()
	withTestStdout(&captured, func() {
		go func() {
			time.Sleep(100 * time.Millisecond)
			eng.AbortRun()
		}()
		_, err = eng.Run(context.Background(), "abort.ts", `
tui.layout({ rows: [ { name: "log" } ] });
tui.pane("log").writeln("up");
await new Promise(r => setTimeout(r, 3600_000));
`)
	})
	if time.Since(start) > 2*time.Second {
		t.Fatalf("AbortRun did not end the TUI script promptly")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}
