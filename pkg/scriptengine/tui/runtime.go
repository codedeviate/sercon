package tui

import (
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// stopTimeout bounds how long Stop() waits for the tview application
// goroutine to exit after app.Stop(). Terminal teardown is sub-
// millisecond in practice; 200ms is generous and avoids piling up
// per-test latency in large test suites.
const stopTimeout = 200 * time.Millisecond

// Controller realises a LayoutNode tree as a tview Flex layout of
// TextView leaves and runs the tview Application on its own goroutine.
// Pane handles returned by Pane(name) are safe to call from any
// goroutine — they enqueue writes via tview.Application.QueueUpdateDraw.
//
// A Controller is single-use: instantiate with NewController, call
// StartScreen (TTY mode) or StartFallback (non-TTY mode), use Pane to
// route output, and call Stop exactly once when the Run finishes.
// Re-creating a TUI for a second Run means a fresh Controller.
type Controller struct {
	root  LayoutNode
	panes map[string]*paneState // keyed by leaf name
	order []string              // leaf names in declaration order (focus cycle)

	mode    controllerMode
	app     *tview.Application // TTY mode only
	screen  tcell.Screen       // TTY mode only
	flex    *tview.Flex        // TTY mode root
	status  *tview.TextView    // TTY mode bottom status bar
	focused int                // index into order (TTY mode)

	fallbackOut io.Writer // non-TTY mode

	readyCh  chan struct{}
	stopCh   chan struct{}
	stopOnce sync.Once

	syncMu sync.Mutex

	// stopped is set by Stop() before app.Stop() so any post-stop writes
	// to QueueUpdateDraw are silently dropped instead of hanging on the
	// dead event loop's done-channel wait.
	stopped atomic.Bool
}

type controllerMode int

const (
	modeUninitialized controllerMode = iota
	modeTUI
	modeFallback
)

// paneState holds the per-pane runtime widgets / writers.
type paneState struct {
	name      string
	titleInit string

	// TUI mode:
	textView *tview.TextView

	// Fallback mode:
	fallback *FallbackPane

	// Shared:
	ansi *ANSITranslator
}

// NewController builds a controller from a parsed layout. It does not
// start any UI; call StartScreen or StartFallback to bring it up.
func NewController(root LayoutNode) (*Controller, error) {
	names := root.AllNames()
	if len(names) == 0 {
		return nil, errors.New("tui: layout has no panes")
	}
	c := &Controller{
		root:    root,
		panes:   make(map[string]*paneState, len(names)),
		order:   names,
		readyCh: make(chan struct{}),
		stopCh:  make(chan struct{}),
	}
	for _, n := range names {
		c.panes[n] = &paneState{name: n, ansi: NewANSITranslator()}
	}
	// Seed titles from the layout.
	root.WalkLeaves(func(leaf LayoutNode) {
		if leaf.Title != "" {
			c.panes[leaf.Name].titleInit = leaf.Title
		}
	})
	return c, nil
}

// StartFallback wires every pane to a FallbackPane writing to out. No
// tview application is started.
func (c *Controller) StartFallback(out io.Writer) error {
	if c.mode != modeUninitialized {
		return errors.New("tui: controller already started")
	}
	c.mode = modeFallback
	c.fallbackOut = out
	for _, ps := range c.panes {
		ps.fallback = NewFallbackPane(out, ps.name)
	}
	close(c.readyCh)
	return nil
}

// StartScreen brings up the tview Application on the given screen
// (tcell.NewScreen() in production; tcell.NewSimulationScreen() in tests).
// Returns when the Application goroutine has been launched.
func (c *Controller) StartScreen(screen tcell.Screen) error {
	if c.mode != modeUninitialized {
		return errors.New("tui: controller already started")
	}
	c.mode = modeTUI
	c.screen = screen

	app := tview.NewApplication().SetScreen(screen)
	c.app = app

	// Build TextView leaves first so the flex tree can reference them.
	buildTextViews(c.root, c.panes, app)

	c.flex = buildFlex(c.root, c.panes)

	c.status = tview.NewTextView().
		SetDynamicColors(true).
		SetTextAlign(tview.AlignLeft)
	c.refreshStatus()

	rootFlex := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(c.flex, 0, 1, true).
		AddItem(c.status, 1, 0, false)

	app.SetRoot(rootFlex, true)
	app.SetFocus(c.panes[c.order[0]].textView)
	app.SetInputCapture(c.onKey)

	go func() {
		defer close(c.stopCh)
		close(c.readyCh) // mark ready as soon as the goroutine starts
		_ = app.Run()
	}()
	return nil
}

// WaitReady blocks until the controller is initialised (either
// StartFallback returned or StartScreen's app goroutine launched).
// Mostly useful for tests; production callers can ignore.
func (c *Controller) WaitReady(timeout time.Duration) {
	select {
	case <-c.readyCh:
	case <-time.After(timeout):
	}
}

// Sync schedules a no-op QueueUpdateDraw and waits for it to land, so
// tests can assert state after a pending event has been processed.
//
// MUST NOT be called from within a QueueUpdateDraw callback or any
// function executing on the tview event-loop goroutine — doing so
// deadlocks (the loop blocks on its own callback's done channel).
// Intended for test code on a different goroutine.
func (c *Controller) Sync() {
	if c.mode != modeTUI || c.app == nil {
		return
	}
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	done := make(chan struct{})
	c.app.QueueUpdateDraw(func() { close(done) })
	<-done
}

// Stop tears down the TUI (restoring the alt screen) or flushes the
// fallback writer. Idempotent.
func (c *Controller) Stop() {
	c.stopOnce.Do(func() {
		switch c.mode {
		case modeTUI:
			if c.app != nil {
				c.stopped.Store(true)
				c.app.Stop()
			}
			// Wait for the application goroutine to actually exit so
			// the alt screen is fully restored before we return.
			select {
			case <-c.stopCh:
			case <-time.After(stopTimeout):
			}
		case modeFallback:
			for _, ps := range c.panes {
				if ps.fallback != nil {
					ps.fallback.Flush()
				}
			}
		}
	})
}

// FocusedPane returns the name of the currently focused pane (TUI mode
// only; "" in fallback mode). Reads the focus index on the event-loop
// goroutine via QueueUpdateDraw so the read is properly synchronised.
// Must not be invoked from within an event-loop callback (see Sync).
func (c *Controller) FocusedPane() string {
	if c.mode != modeTUI {
		return ""
	}
	c.syncMu.Lock()
	defer c.syncMu.Unlock()
	if c.stopped.Load() {
		return ""
	}
	var name string
	done := make(chan struct{})
	c.app.QueueUpdateDraw(func() {
		name = c.order[c.focused]
		close(done)
	})
	<-done
	return name
}

// PaneContent returns the current text content of a pane. In TUI mode it
// reads from the TextView; in fallback mode it returns "" (content was
// already streamed to the fallback writer).
func (c *Controller) PaneContent(name string) string {
	ps, ok := c.panes[name]
	if !ok {
		return ""
	}
	if c.mode == modeTUI && ps.textView != nil {
		return ps.textView.GetText(true) // stripped of color tags
	}
	return ""
}

// PaneNames returns the leaf names in declaration order.
func (c *Controller) PaneNames() []string { return c.order }

// Pane returns a handle for the named pane, or nil if no pane with that
// name exists. The binding layer translates a nil result to a JS throw.
func (c *Controller) Pane(name string) Pane {
	ps, ok := c.panes[name]
	if !ok {
		return nil
	}
	return paneHandle{c: c, ps: ps}
}

// Pane is the public interface the binding layer hands to JS. Each
// method enqueues to the TUI goroutine (TUI mode) or writes through the
// fallback writer (fallback mode) and returns synchronously.
type Pane interface {
	Write(s string)
	Writeln(s string)
	Clear()
	Title(s string)
	// AsWriter returns an io.Writer that streams into this pane. Used by
	// the api.exec.shell pane: option to wire cmd.Stdout / cmd.Stderr.
	AsWriter() io.Writer
}

type paneHandle struct {
	c  *Controller
	ps *paneState
}

func (h paneHandle) Write(s string)   { h.write([]byte(s)) }
func (h paneHandle) Writeln(s string) { h.write([]byte(s + "\n")) }

func (h paneHandle) write(b []byte) {
	switch h.c.mode {
	case modeTUI:
		if h.c.stopped.Load() {
			return
		}
		tagged := h.ps.ansi.Translate(string(b))
		tv := h.ps.textView
		h.c.app.QueueUpdateDraw(func() { _, _ = tv.Write([]byte(tagged)) })
	case modeFallback:
		_, _ = h.ps.fallback.Write(b)
	}
}

func (h paneHandle) Clear() {
	switch h.c.mode {
	case modeTUI:
		if h.c.stopped.Load() {
			return
		}
		tv := h.ps.textView
		h.c.app.QueueUpdateDraw(func() { tv.Clear() })
	case modeFallback:
		// No-op in fallback mode: lines are already on the writer.
	}
}

func (h paneHandle) Title(s string) {
	switch h.c.mode {
	case modeTUI:
		if h.c.stopped.Load() {
			return
		}
		tv := h.ps.textView
		h.c.app.QueueUpdateDraw(func() { tv.SetTitle(" " + s + " ") })
	case modeFallback:
		// No-op: prefixed lines already identify the pane.
	}
}

func (h paneHandle) AsWriter() io.Writer { return paneIOWriter{h: h} }

type paneIOWriter struct{ h paneHandle }

func (w paneIOWriter) Write(p []byte) (int, error) {
	w.h.write(p)
	return len(p), nil
}

// onKey handles the controller's keybindings.
func (c *Controller) onKey(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyTab:
		c.focused = (c.focused + 1) % len(c.order)
		c.applyFocus()
		return nil
	case tcell.KeyBacktab:
		c.focused = (c.focused - 1 + len(c.order)) % len(c.order)
		c.applyFocus()
		return nil
	case tcell.KeyCtrlC:
		// Let tcell's default handler do its thing (raise SIGINT-style
		// quit). tview converts Ctrl-C into app.Stop, which is what we
		// want — the engine's watcher goroutine sees ctx.Done().
		return ev
	}
	return ev
}

func (c *Controller) applyFocus() {
	name := c.order[c.focused]
	tv := c.panes[name].textView
	c.app.SetFocus(tv)
	for _, ps := range c.panes {
		ps.textView.SetBorderColor(tcell.ColorGray)
	}
	tv.SetBorderColor(tcell.ColorYellow)
	c.refreshStatus()
}

func (c *Controller) refreshStatus() {
	if c.status == nil {
		return
	}
	keys := "[gray]Tab[-] focus  [gray]PgUp/PgDn[-] scroll  [gray]Home/End[-] jump  [gray]Ctrl-C[-] quit"
	c.status.SetText(fmt.Sprintf(" [yellow]%s[-]   %s", c.order[c.focused], keys))
}

// buildTextViews populates each leaf's TextView in panes. Called by
// StartScreen before buildFlex stitches them into the Flex tree.
func buildTextViews(node LayoutNode, panes map[string]*paneState, app *tview.Application) {
	if node.IsLeaf() {
		ps := panes[node.Name]
		tv := tview.NewTextView().
			SetDynamicColors(true).
			SetScrollable(true).
			SetWrap(true).
			SetWordWrap(false)
		tv.SetBorder(true).SetTitle(" " + paneTitle(node) + " ").SetBorderColor(tcell.ColorGray)
		// Trigger a redraw whenever text is appended so the view auto-scrolls.
		tv.SetChangedFunc(func() { app.Draw() })
		ps.textView = tv
		return
	}
	children := node.Rows
	if node.IsCols() {
		children = node.Cols
	}
	for _, ch := range children {
		buildTextViews(ch, panes, app)
	}
}

// paneTitle returns the title to display in the pane's border.
func paneTitle(node LayoutNode) string {
	if node.Title != "" {
		return node.Title
	}
	return node.Name
}

// buildFlex converts a LayoutNode tree into a tview.Flex tree, attaching
// the already-created TextView at each leaf.
func buildFlex(node LayoutNode, panes map[string]*paneState) *tview.Flex {
	if node.IsLeaf() {
		// A leaf wrapped in a 1-child Flex so the parent can give it
		// proportional sizing.
		return tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(panes[node.Name].textView, 0, 1, false)
	}
	dir := tview.FlexRow
	children := node.Rows
	if node.IsCols() {
		dir = tview.FlexColumn
		children = node.Cols
	}
	f := tview.NewFlex().SetDirection(dir)
	for _, child := range children {
		w := child.Weight
		if w <= 0 {
			w = 1
		}
		if child.IsLeaf() {
			f.AddItem(panes[child.Name].textView, 0, w, false)
		} else {
			f.AddItem(buildFlex(child, panes), 0, w, false)
		}
	}
	return f
}
