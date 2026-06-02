//go:build !windows

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"

	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

// In pty mode with a pane, the child's tty-gated output must reach the pane.
// Stand up a real Controller on a simulation screen, register it active (what
// resolvePane looks up for a string pane name), then run execShell with
// { pane: "log", pty: true }.
func TestExecShell_PTY_PaneReceivesOutput(t *testing.T) {
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
	sim.SetSize(80, 10)
	if err := c.StartScreen(sim); err != nil {
		t.Fatal(err)
	}
	c.WaitReady(2 * time.Second)
	setActiveController(c)
	defer func() { setActiveController(nil); c.Stop() }()

	_, err = runShell(t, shellCall{
		cmd:  "test -t 1 && printf 'PANETTYOK\\n' || printf 'PANENO\\n'",
		opts: map[string]any{"pane": "log", "pty": true},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Pane writes go through QueueUpdateDraw; poll PaneContent for the marker.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(c.PaneContent("log"), "PANETTYOK") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("pane did not receive pty output; content=%q", c.PaneContent("log"))
}
