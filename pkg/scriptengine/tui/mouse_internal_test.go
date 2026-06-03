package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

func startTwoPaneMouseTUI(t *testing.T) (*Controller, tcell.SimulationScreen) {
	t.Helper()
	root, err := ParseLayout(map[string]any{
		"mouse": true,
		"rows": []any{
			map[string]any{"name": "A"},
			map[string]any{"name": "B"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewController(root)
	if err != nil {
		t.Fatal(err)
	}
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(80, 24)
	if err := c.StartScreen(sim); err != nil {
		t.Fatal(err)
	}
	c.WaitReady(2 * time.Second)
	return c, sim
}

func innerMidpoint(c *Controller, name string) (x, y int) {
	done := make(chan struct{})
	c.app.QueueUpdateDraw(func() {
		px, py, pw, ph := c.panes[name].textView.GetInnerRect()
		x, y = px+pw/2, py+ph/2
		close(done)
	})
	<-done
	return
}

func TestMouse_WheelScrollsPaneUnderCursorNotFocused(t *testing.T) {
	c, sim := startTwoPaneMouseTUI(t)
	defer c.Stop()
	for i := 0; i < 100; i++ {
		c.Pane("B").Writeln(fmt.Sprintf("B-%02d", i))
	}
	c.Sync()
	if got := c.FocusedPane(); got != "A" {
		t.Fatalf("precondition: focus should start on A, got %q", got)
	}
	before := c.PaneScrollOffset("B")
	if before <= 0 {
		t.Fatalf("precondition: B should be scrolled (autoscroll); offset=%d", before)
	}
	bx, by := innerMidpoint(c, "B")
	sim.InjectMouse(bx, by, tcell.WheelUp, tcell.ModNone)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.PaneScrollOffset("B") < before {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if after := c.PaneScrollOffset("B"); after >= before {
		t.Fatalf("wheel over B did not scroll B: before=%d after=%d", before, after)
	}
	if got := c.FocusedPane(); got != "A" {
		t.Fatalf("wheel must not change focus: got %q, want A", got)
	}
}

func TestMouse_LeftClickFocusesPaneUnderCursor(t *testing.T) {
	c, sim := startTwoPaneMouseTUI(t)
	defer c.Stop()
	if got := c.FocusedPane(); got != "A" {
		t.Fatalf("precondition: focus should start on A, got %q", got)
	}
	bx, by := innerMidpoint(c, "B")
	sim.InjectMouse(bx, by, tcell.Button1, tcell.ModNone)
	sim.InjectMouse(bx, by, tcell.ButtonNone, tcell.ModNone)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if c.FocusedPane() == "B" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("left-click over B did not focus B; focus=%q", c.FocusedPane())
}

func TestMouse_WheelOverStatusBarIsNoop(t *testing.T) {
	c, sim := startTwoPaneMouseTUI(t)
	defer c.Stop()
	for i := 0; i < 100; i++ {
		c.Pane("A").Writeln(fmt.Sprintf("A-%02d", i))
		c.Pane("B").Writeln(fmt.Sprintf("B-%02d", i))
	}
	c.Sync()
	aBefore, bBefore := c.PaneScrollOffset("A"), c.PaneScrollOffset("B")
	sim.InjectMouse(10, 23, tcell.WheelUp, tcell.ModNone) // row 23 = status bar
	time.Sleep(100 * time.Millisecond)
	c.Sync()
	if c.PaneScrollOffset("A") != aBefore || c.PaneScrollOffset("B") != bBefore {
		t.Fatalf("wheel over status bar changed a pane offset: A %d->%d B %d->%d",
			aBefore, c.PaneScrollOffset("A"), bBefore, c.PaneScrollOffset("B"))
	}
}
