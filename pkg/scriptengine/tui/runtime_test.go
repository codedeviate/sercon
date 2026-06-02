package tui_test

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

// Non-TTY mode: writes route through FallbackPane.
func TestController_FallbackMode(t *testing.T) {
	root, err := tui.ParseLayout(map[string]any{
		"rows": []any{
			map[string]any{"name": "log"},
			map[string]any{"name": "brew"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.StartFallback(&buf); err != nil {
		t.Fatal(err)
	}
	c.Pane("log").Writeln("orchestrator says hi")
	c.Pane("brew").Writeln("brew installing\nbrew done")
	c.Stop()
	got := buf.String()
	for _, want := range []string{
		"[log] orchestrator says hi\n",
		"[brew] brew installing\n",
		"[brew] brew done\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fallback output missing %q; got:\n%s", want, got)
		}
	}
}

// TTY mode: drive a tcell.SimulationScreen, write to a pane, assert the
// pane's TextView contains the text. Focus cycles via Tab.
func TestController_TUIMode_PaneWriteAndFocus(t *testing.T) {
	root, err := tui.ParseLayout(map[string]any{
		"rows": []any{
			map[string]any{"name": "log"},
			map[string]any{"name": "brew"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 20)
	if err := c.StartScreen(sim); err != nil {
		t.Fatal(err)
	}
	// Synchronously wait for the application loop to be ready.
	c.WaitReady(2 * time.Second)

	c.Pane("log").Writeln("ready")
	c.Pane("brew").Writeln("installing")

	// Give the application loop a beat to draw.
	c.Sync()

	if got := c.FocusedPane(); got != "log" {
		t.Errorf("initial focus: got %q, want log", got)
	}

	// Tab → focus next pane.
	// Sync() alone isn't enough: tview's Application loop drains the
	// screen's event channel (where InjectKey delivers) and the
	// QueueUpdateDraw channel independently, so the no-op Sync callback
	// can be processed before the key event. Poll FocusedPane until it
	// matches, with a generous timeout.
	sim.InjectKey(tcell.KeyTab, 0, tcell.ModNone)
	waitFocus(t, c, "brew", 2*time.Second)

	// Shift-Tab → focus previous.
	sim.InjectKey(tcell.KeyBacktab, 0, tcell.ModNone)
	waitFocus(t, c, "log", 2*time.Second)

	// Pane content is reachable via the controller (for tests).
	if got := c.PaneContent("log"); !strings.Contains(got, "ready") {
		t.Errorf("log pane missing 'ready'; got %q", got)
	}

	c.Stop()
}

// Pane handle for an unknown pane returns nil (binding translates to throw).
func TestController_UnknownPaneNil(t *testing.T) {
	root, err := tui.ParseLayout(map[string]any{"name": "only"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	if c.Pane("missing") != nil {
		t.Fatal("expected nil for unknown pane")
	}
}

// Two consecutive Stop() calls are safe (idempotent).
func TestController_StopIdempotent(t *testing.T) {
	root, _ := tui.ParseLayout(map[string]any{"name": "x"})
	c, _ := tui.NewController(root)
	var buf bytes.Buffer
	if err := c.StartFallback(&buf); err != nil {
		t.Fatal(err)
	}
	c.Stop()
	// Second call must not panic.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); c.Stop() }()
	wg.Wait()
}

// screenText flattens the simulation screen's cell buffer into a string.
func screenText(sim tcell.SimulationScreen) string {
	cells, w, h := sim.GetContents()
	var b strings.Builder
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r := cells[y*w+x].Runes
			if len(r) == 0 || r[0] == 0 {
				b.WriteRune(' ')
			} else {
				b.WriteRune(r[0])
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func TestController_AutoScrollFollowsTail(t *testing.T) {
	root, err := tui.ParseLayout(map[string]any{"name": "log"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(40, 10)
	if err := c.StartScreen(sim); err != nil {
		t.Fatal(err)
	}
	c.WaitReady(2 * time.Second)
	for i := 0; i < 40; i++ {
		c.Pane("log").Writeln(fmt.Sprintf("LINE-%02d", i))
	}
	c.Sync()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(screenText(sim), "LINE-39") {
			c.Stop()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.Stop()
	t.Fatalf("autoscroll: last line LINE-39 not visible; screen:\n%s", screenText(sim))
}

func TestController_AutoScrollOptOutStaysTop(t *testing.T) {
	no := false
	root := tui.LayoutNode{Name: "log", AutoScroll: &no}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(40, 10)
	if err := c.StartScreen(sim); err != nil {
		t.Fatal(err)
	}
	c.WaitReady(2 * time.Second)
	for i := 0; i < 40; i++ {
		c.Pane("log").Writeln(fmt.Sprintf("LINE-%02d", i))
	}
	c.Sync()
	time.Sleep(100 * time.Millisecond)
	c.Sync()
	got := screenText(sim)
	c.Stop()
	if !strings.Contains(got, "LINE-00") {
		t.Fatalf("opt-out: expected top line LINE-00 visible; screen:\n%s", got)
	}
	if strings.Contains(got, "LINE-39") {
		t.Fatalf("opt-out: last line should NOT be visible (top pinned); screen:\n%s", got)
	}
}

func TestController_MouseStatusIndicator(t *testing.T) {
	root, err := tui.ParseLayout(map[string]any{
		"mouse": true,
		"rows":  []any{map[string]any{"name": "log"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 10)
	if err := c.StartScreen(sim); err != nil {
		t.Fatal(err)
	}
	c.WaitReady(2 * time.Second)
	c.Sync()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(screenText(sim), "mouse") {
			c.Stop()
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	c.Stop()
	t.Fatalf("status bar missing 'mouse' indicator; screen:\n%s", screenText(sim))
}

func newTUIForKeys(t *testing.T) (*tui.Controller, tcell.SimulationScreen) {
	t.Helper()
	root, err := tui.ParseLayout(map[string]any{
		"rows": []any{
			map[string]any{"name": "log"},
			map[string]any{"name": "out"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 20)
	if err := c.StartScreen(sim); err != nil {
		t.Fatal(err)
	}
	c.WaitReady(2 * time.Second)
	return c, sim
}

func TestController_OnKeyReceivesRune(t *testing.T) {
	c, sim := newTUIForKeys(t)
	defer c.Stop()
	got := make(chan tui.KeyEvent, 1)
	c.AddKeyHandler(func(ev tui.KeyEvent) { got <- ev })
	sim.InjectKey(tcell.KeyRune, 'q', tcell.ModNone)
	select {
	case ev := <-got:
		if ev.Name != "Rune" || ev.Rune != "q" {
			t.Fatalf("got %+v, want {Name:Rune Rune:q}", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("onKey handler not invoked for rune 'q'")
	}
}

func TestController_RemoveKeyHandler(t *testing.T) {
	c, sim := newTUIForKeys(t)
	defer c.Stop()
	got := make(chan tui.KeyEvent, 4)
	id := c.AddKeyHandler(func(ev tui.KeyEvent) { got <- ev })
	c.RemoveKeyHandler(id)
	sim.InjectKey(tcell.KeyRune, 'x', tcell.ModNone)
	select {
	case ev := <-got:
		t.Fatalf("removed handler still fired: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestController_WaitKeyFIFO(t *testing.T) {
	c, sim := newTUIForKeys(t)
	defer c.Stop()
	res := make(chan tui.KeyEvent, 1)
	go func() {
		ev, ok := c.WaitKey()
		if ok {
			res <- ev
		}
	}()
	time.Sleep(50 * time.Millisecond)
	sim.InjectKey(tcell.KeyEnter, 0, tcell.ModNone)
	select {
	case ev := <-res:
		if ev.Name != "Enter" {
			t.Fatalf("got %+v, want Enter", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitKey did not resolve on Enter")
	}
}

// A waiter parked in WaitKey must unblock with (zero, false) when the
// controller stops — this is the crux of the teardown path (a pending
// tui.waitKey() rejecting cleanly when the Run ends / Ctrl-C aborts).
func TestController_WaitKeyUnblocksOnStop(t *testing.T) {
	c, _ := newTUIForKeys(t)
	type res struct {
		ev tui.KeyEvent
		ok bool
	}
	done := make(chan res, 1)
	go func() {
		ev, ok := c.WaitKey()
		done <- res{ev, ok}
	}()
	time.Sleep(50 * time.Millisecond) // let WaitKey enqueue
	c.Stop()
	select {
	case r := <-done:
		if r.ok {
			t.Fatalf("WaitKey should return ok=false on stop, got ok=true (%+v)", r.ev)
		}
		if r.ev != (tui.KeyEvent{}) {
			t.Fatalf("WaitKey should return a zero KeyEvent on stop, got %+v", r.ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitKey did not unblock after Stop")
	}
}

func TestController_CtrlCInvokesAbortAndConsumes(t *testing.T) {
	c, sim := newTUIForKeys(t)
	defer c.Stop()
	aborted := make(chan struct{}, 1)
	c.SetAbort(func() { aborted <- struct{}{} })
	sim.InjectKey(tcell.KeyCtrlC, 0, tcell.ModCtrl)
	select {
	case <-aborted:
	case <-time.After(2 * time.Second):
		t.Fatal("Ctrl-C did not invoke abort callback")
	}
}

func TestController_InteractiveTrueInTTY(t *testing.T) {
	c, _ := newTUIForKeys(t)
	defer c.Stop()
	if !c.Interactive() {
		t.Fatal("Interactive() should be true in TTY mode")
	}
}

func TestController_InteractiveFalseInFallback(t *testing.T) {
	root, _ := tui.ParseLayout(map[string]any{"name": "x"})
	c, _ := tui.NewController(root)
	var buf bytes.Buffer
	if err := c.StartFallback(&buf); err != nil {
		t.Fatal(err)
	}
	defer c.Stop()
	if c.Interactive() {
		t.Fatal("Interactive() should be false in fallback mode")
	}
}

func TestController_OnKeyHandlerPanicIsolated(t *testing.T) {
	c, sim := newTUIForKeys(t)
	defer c.Stop()
	got := make(chan tui.KeyEvent, 1)
	// First handler panics; second must still receive the key.
	c.AddKeyHandler(func(ev tui.KeyEvent) { panic("boom") })
	c.AddKeyHandler(func(ev tui.KeyEvent) { got <- ev })
	sim.InjectKey(tcell.KeyRune, 'z', tcell.ModNone)
	select {
	case ev := <-got:
		if ev.Rune != "z" {
			t.Fatalf("got %+v, want rune z", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second handler did not receive the key (panic not isolated, or dispatch died)")
	}
}

// waitFocus polls FocusedPane until it returns want or the timeout
// elapses. Used in TTY-mode tests because tview's Application loop
// drains the screen-event channel and the QueueUpdateDraw channel
// independently — Sync() alone doesn't guarantee a previously
// InjectKey-delivered event has been processed.
func waitFocus(t *testing.T, c *tui.Controller, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var got string
	for time.Now().Before(deadline) {
		got = c.FocusedPane()
		if got == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("focus didn't reach %q within %s; last seen %q", want, timeout, got)
}

// fgAtRune returns the foreground color of the first screen cell whose
// primary rune is r, and whether such a cell was found.
func fgAtRune(sim tcell.SimulationScreen, r rune) (tcell.Color, bool) {
	cells, w, h := sim.GetContents()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			rs := cells[y*w+x].Runes
			if len(rs) > 0 && rs[0] == r {
				fg, _, _ := cells[y*w+x].Style.Decompose()
				return fg, true
			}
		}
	}
	return tcell.ColorDefault, false
}

func startSingleLeaf(t *testing.T, leaf map[string]any, w, h int) (*tui.Controller, tcell.SimulationScreen) {
	t.Helper()
	root, err := tui.ParseLayout(leaf)
	if err != nil {
		t.Fatal(err)
	}
	c, err := tui.NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	if err := c.StartScreen(sim); err != nil {
		t.Fatal(err)
	}
	c.WaitReady(2 * time.Second)
	// SetSize must come after StartScreen/WaitReady: tview's SetScreen calls
	// screen.Init() internally which resets the simulation screen to 80×25.
	// Calling SetSize afterwards injects a resize event that tview processes
	// on its next draw cycle.
	sim.SetSize(w, h)
	c.Sync()
	return c, sim
}

func TestController_WrapOffClipsLongLine(t *testing.T) {
	c, sim := startSingleLeaf(t, map[string]any{"name": "log", "wrap": "off"}, 40, 10)
	defer c.Stop()
	c.Pane("log").Writeln(strings.Repeat("X", 60) + "ZEND")
	c.Sync()
	time.Sleep(100 * time.Millisecond)
	c.Sync()
	if strings.Contains(screenText(sim), "ZEND") {
		t.Fatalf("wrap:off should clip the long line; ZEND was visible:\n%s", screenText(sim))
	}
}

func TestController_WrapCharShowsWrappedTail(t *testing.T) {
	c, sim := startSingleLeaf(t, map[string]any{"name": "log", "wrap": "char"}, 40, 10)
	defer c.Stop()
	c.Pane("log").Writeln(strings.Repeat("X", 60) + "ZEND")
	c.Sync()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(screenText(sim), "ZEND") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("wrap:char should wrap the tail into view; ZEND not visible:\n%s", screenText(sim))
}

func TestController_ColorOnRendersColor(t *testing.T) {
	c, sim := startSingleLeaf(t, map[string]any{"name": "log"}, 40, 10)
	defer c.Stop()
	c.Pane("log").Writeln("\x1b[31mZ\x1b[0m")
	c.Sync()
	time.Sleep(100 * time.Millisecond)
	c.Sync()
	fg, ok := fgAtRune(sim, 'Z')
	if !ok {
		t.Fatalf("rune Z not found on screen:\n%s", screenText(sim))
	}
	if fg == tcell.ColorDefault {
		t.Fatalf("color on: Z should have a non-default foreground, got default")
	}
}

func TestController_ColorOffStripsToPlain(t *testing.T) {
	c, sim := startSingleLeaf(t, map[string]any{"name": "log", "color": false}, 40, 10)
	defer c.Stop()
	c.Pane("log").Writeln("\x1b[31mQ\x1b[0m")
	c.Sync()
	time.Sleep(100 * time.Millisecond)
	c.Sync()
	txt := screenText(sim)
	if !strings.Contains(txt, "Q") {
		t.Fatalf("color off: expected plain Q on screen:\n%s", txt)
	}
	if strings.Contains(txt, "[31m") {
		t.Fatalf("color off: raw tag leaked to screen:\n%s", txt)
	}
	fg, ok := fgAtRune(sim, 'Q')
	if !ok {
		t.Fatalf("rune Q not found:\n%s", txt)
	}
	// tview's TextView always renders text with an explicit style (white fg on
	// black bg in its default theme), not tcell.ColorDefault. The invariant for
	// color:false is that ANSI SGR 31 (red) is NOT applied — Q's fg should be
	// the plain-text default (white), not darkred.
	darkred := tcell.GetColor("darkred")
	if fg == darkred {
		t.Fatalf("color off: Q fg is darkred — ANSI SGR 31 was applied when it should have been stripped (got %v)", fg)
	}
}
