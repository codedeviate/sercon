package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/gdamore/tcell/v2"

	"github.com/codedeviate/sercon/pkg/scriptengine"
	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

// tuiOutputForTest, when non-nil, replaces os.Stdout for the tui
// fallback writer. Only set by tui_test.go's withTestStdout helper.
// In production this stays nil and the fallback writes to os.Stdout.
var (
	tuiOutputForTest   io.Writer
	tuiOutputForTestMu sync.Mutex
)

// withTestStdout runs fn with os.Stdout-replacement enabled for the tui
// fallback path. Tests use this to capture pane writes without juggling
// real file descriptors.
func withTestStdout(w io.Writer, fn func()) {
	tuiOutputForTestMu.Lock()
	tuiOutputForTest = w
	tuiOutputForTestMu.Unlock()
	defer func() {
		tuiOutputForTestMu.Lock()
		tuiOutputForTest = nil
		tuiOutputForTestMu.Unlock()
	}()
	fn()
}

// tuiNamespace builds the tui factory function. It captures the
// engine so the controller's Stop can be registered as an AddRunCleanup.
// The factory itself is invoked once per Run; the closure variables
// (controller, ctrlMu) are therefore per-Run state.
func tuiNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	var (
		ctrl   *tui.Controller
		ctrlMu sync.Mutex
	)
	throw := func(msg string) {
		panic(vm.NewGoError(errors.New(msg)))
	}
	layout := func(call goja.FunctionCall) goja.Value {
		if eng.WatchMode() {
			throw("tui is not supported under --watch")
		}
		ctrlMu.Lock()
		defer ctrlMu.Unlock()
		if ctrl != nil {
			throw("layout already declared for this Run")
		}
		arg := call.Argument(0)
		if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
			throw("tui.layout: tree argument is required")
		}
		root, err := tui.ParseLayout(arg.Export())
		if err != nil {
			throw("tui.layout: " + err.Error())
		}
		c, err := tui.NewController(root)
		if err != nil {
			throw("tui.layout: " + err.Error())
		}
		out := pickFallbackOutput()
		if isTTY(os.Stdout) && out == os.Stdout {
			if err := startControllerScreen(c); err != nil {
				throw("tui.layout: " + err.Error())
			}
		} else {
			if err := c.StartFallback(out); err != nil {
				throw("tui.layout: " + err.Error())
			}
		}
		ctrl = c
		setActiveController(c)
		// Single-press Ctrl-C: route the TUI's Ctrl-C through the engine's
		// cancel path so the first press ends the Run (and the cleanup below
		// restores the screen). Without this the script keeps running after
		// the screen is torn down, requiring a second Ctrl-C.
		c.SetAbort(func() { eng.AbortRun() })
		eng.AddRunCleanup(func() {
			c.Stop()
			setActiveController(nil)
		})
		return goja.Undefined()
	}
	pane := func(call goja.FunctionCall) goja.Value {
		ctrlMu.Lock()
		c := ctrl
		ctrlMu.Unlock()
		if c == nil {
			throw("tui.pane: call tui.layout(...) first")
		}
		name := call.Argument(0).String()
		h := c.Pane(name)
		if h == nil {
			throw(fmt.Sprintf("tui.pane: unknown pane %q (declared: %v)", name, c.PaneNames()))
		}
		obj := vm.NewObject()
		_ = obj.Set("write", func(call goja.FunctionCall) goja.Value {
			h.Write(call.Argument(0).String())
			return goja.Undefined()
		})
		_ = obj.Set("writeln", func(call goja.FunctionCall) goja.Value {
			h.Writeln(call.Argument(0).String())
			return goja.Undefined()
		})
		_ = obj.Set("clear", func(call goja.FunctionCall) goja.Value {
			h.Clear()
			return goja.Undefined()
		})
		_ = obj.Set("title", func(call goja.FunctionCall) goja.Value {
			h.Title(call.Argument(0).String())
			return goja.Undefined()
		})
		// Expose the Go handle for services.exec.shell's pane: option to
		// pick up without going back through the name → handle lookup.
		// Stored under a non-enumerable, internal property name.
		_ = obj.DefineDataProperty("__sercon_pane__", vm.ToValue(h),
			goja.FLAG_FALSE, goja.FLAG_FALSE, goja.FLAG_FALSE)
		return obj
	}
	return map[string]any{
		"layout": layout,
		"pane":   pane,
	}
}

// tuiNewScreen creates the tcell screen for the TTY path. It is a package
// var so tests can substitute an Init-counting screen to guard the
// single-Init contract (see startControllerScreen).
var tuiNewScreen = tcell.NewScreen

// startControllerScreen creates a tcell screen and hands it to the
// Controller for the TTY path.
//
// It deliberately does NOT call screen.Init(): Controller.StartScreen
// builds a tview.Application via SetScreen, and SetScreen initialises the
// screen exactly once (its doc even notes "Init() need not be called on
// the screen"). Calling Init() here as well double-initialises the tcell
// tty — tcell's tty.Start runs term.MakeRaw, which returns the termios as
// it was *before* the call, so the second Init saves the already-raw
// state as the restore target. The single Fini() on teardown then
// restores the terminal to raw mode instead of cooked, leaving it with no
// echo and Ctrl-C delivered as a keystroke. That was the v0.11.0 bug where
// a second `tui` run hung and only killing the terminal recovered it.
func startControllerScreen(c *tui.Controller) error {
	screen, err := tuiNewScreen()
	if err != nil {
		return err
	}
	if err := c.StartScreen(screen); err != nil {
		screen.Fini()
		return err
	}
	return nil
}

// activeController holds the TUI controller for the current Run.
// Populated by the tui.layout binding; read by services.exec.shell's
// pane: option when a string name is given. Cleared by the Engine's
// AddRunCleanup hook at Run end.
var (
	activeController   *tui.Controller
	activeControllerMu sync.RWMutex
)

func activeTUIController() *tui.Controller {
	activeControllerMu.RLock()
	defer activeControllerMu.RUnlock()
	return activeController
}

func setActiveController(c *tui.Controller) {
	activeControllerMu.Lock()
	activeController = c
	activeControllerMu.Unlock()
}

// pickFallbackOutput returns the writer the non-TTY fallback should use.
// In tests, withTestStdout swaps this for an in-memory buffer; in
// production it's os.Stdout.
func pickFallbackOutput() io.Writer {
	tuiOutputForTestMu.Lock()
	defer tuiOutputForTestMu.Unlock()
	if tuiOutputForTest != nil {
		return tuiOutputForTest
	}
	return os.Stdout
}

// isTTY reports whether w is a character device. Mirrors help.go's
// shouldColor logic so the two stay consistent.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
