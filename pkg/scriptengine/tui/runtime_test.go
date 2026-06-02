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
