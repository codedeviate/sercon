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

	// Key dispatch (TTY mode). onKey runs on the tview application
	// goroutine; handlers must not block it, so the binding's handler
	// closures schedule onto the event loop via a non-blocking RunOnLoop.
	keyMu       sync.Mutex
	keyHandlers map[int]func(KeyEvent)
	nextKeyID   int
	keyWaiters  []chan KeyEvent
	abortFn     func()
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

	// color reports whether subprocess ANSI is rendered (true) or stripped
	// to plain text (false). Set from the leaf's color option at build time.
	color bool
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
	// Seed per-leaf options from the layout.
	root.WalkLeaves(func(leaf LayoutNode) {
		ps := c.panes[leaf.Name]
		if leaf.Title != "" {
			ps.titleInit = leaf.Title
		}
		ps.color = leaf.Color == nil || *leaf.Color
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

	if c.root.Mouse {
		app.EnableMouse(true)
	}

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
		var rendered string
		if h.ps.color {
			rendered = h.ps.ansi.Translate(string(b))
		} else {
			rendered = StripANSI(string(b))
		}
		tv := h.ps.textView
		h.c.app.QueueUpdateDraw(func() { _, _ = tv.Write([]byte(rendered)) })
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

// AsWriter returns an io.Writer that streams into this pane. Used by
// the api.exec.shell pane: option to wire cmd.Stdout / cmd.Stderr. Each
// call to AsWriter returns a fresh adapter with its own mutex; exec.Cmd
// spawns separate goroutines for stdout and stderr copies, so when the
// same writer is wired to both, concurrent Write calls must be
// serialised. TTY-mode writes go through QueueUpdateDraw (safe), but the
// fallback path writes directly into FallbackPane's strings.Builder
// and would race without this guard.
func (h paneHandle) AsWriter() io.Writer {
	return &paneIOWriter{h: h}
}

type paneIOWriter struct {
	h  paneHandle
	mu sync.Mutex
}

func (w *paneIOWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.h.write(p)
	return len(p), nil
}

// onKey handles the controller's keybindings. Ctrl-C aborts the Run via
// the abort callback (single press) and is never delivered to JS. Every
// other key drives built-in nav/scroll AND is dispatched to JS handlers /
// waiters (the "coexist" model). Tab/Shift-Tab additionally cycle focus.
func (c *Controller) onKey(ev *tcell.EventKey) *tcell.EventKey {
	switch ev.Key() {
	case tcell.KeyCtrlC:
		c.keyMu.Lock()
		abort := c.abortFn
		c.keyMu.Unlock()
		if abort != nil {
			abort()
			return nil // consume; teardown happens via the Run-cleanup path
		}
		// No abort wired (defensive / non-engine caller): preserve the old
		// behaviour so tview's default Ctrl-C still stops the app.
		return ev
	case tcell.KeyTab:
		c.focused = (c.focused + 1) % len(c.order)
		c.applyFocus()
		c.dispatchKey(toKeyEvent(ev))
		return nil
	case tcell.KeyBacktab:
		c.focused = (c.focused - 1 + len(c.order)) % len(c.order)
		c.applyFocus()
		c.dispatchKey(toKeyEvent(ev))
		return nil
	}
	// All other keys: deliver to JS and let tview handle nav/scroll.
	c.dispatchKey(toKeyEvent(ev))
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
	if c.root.Mouse {
		keys += "  [gray]mouse[-] on"
	}
	c.status.SetText(fmt.Sprintf(" [yellow]%s[-]   %s", c.order[c.focused], keys))
}

// KeyEvent is the descriptor delivered to tui.onKey handlers and resolved
// by tui.waitKey. Name is the tcell key name ("Enter", "Up", "Tab",
// "Ctrl-A", "F1", ...) or "Rune" for a printable character, in which case
// Rune holds that character. Ctrl/Alt/Shift reflect the event modifiers.
type KeyEvent struct {
	Name  string
	Rune  string
	Ctrl  bool
	Alt   bool
	Shift bool
}

// SetAbort installs the callback invoked when the user presses Ctrl-C in
// TTY mode. The binding wires this to Engine.AbortRun so a single press
// cancels the Run. Ctrl-C is never delivered to key handlers/waiters.
func (c *Controller) SetAbort(fn func()) {
	c.keyMu.Lock()
	c.abortFn = fn
	c.keyMu.Unlock()
}

// Interactive reports whether keyboard input is available (TTY mode).
func (c *Controller) Interactive() bool { return c.mode == modeTUI }

// AddKeyHandler registers fn to be called for every key (except Ctrl-C).
// Returns an id for RemoveKeyHandler. fn is called from the tview
// application goroutine and MUST NOT block it.
func (c *Controller) AddKeyHandler(fn func(KeyEvent)) int {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	if c.keyHandlers == nil {
		c.keyHandlers = map[int]func(KeyEvent){}
	}
	c.nextKeyID++
	id := c.nextKeyID
	c.keyHandlers[id] = fn
	return id
}

// RemoveKeyHandler unregisters a handler previously added via AddKeyHandler.
func (c *Controller) RemoveKeyHandler(id int) {
	c.keyMu.Lock()
	delete(c.keyHandlers, id)
	c.keyMu.Unlock()
}

// WaitKey blocks until the next key arrives (returning it, true) or the
// controller stops (returning a zero KeyEvent, false). FIFO across
// concurrent callers: one keypress resolves the oldest waiter. Intended to
// be called from a worker goroutine (e.g. PromisifyAsync), never the loop.
func (c *Controller) WaitKey() (KeyEvent, bool) {
	ch := make(chan KeyEvent, 1)
	c.keyMu.Lock()
	c.keyWaiters = append(c.keyWaiters, ch)
	c.keyMu.Unlock()
	select {
	case ev := <-ch:
		return ev, true
	case <-c.stopCh:
		return KeyEvent{}, false
	}
}

// dispatchKey fans a key out to the oldest pending waiter (if any) and to
// every registered handler. Called from the tview goroutine (onKey). It
// must not block: the waiter channel is buffered (non-blocking send) and
// handlers are documented to return promptly (the binding's closures only
// enqueue a non-blocking RunOnLoop).
func (c *Controller) dispatchKey(ev KeyEvent) {
	c.keyMu.Lock()
	if len(c.keyWaiters) > 0 {
		w := c.keyWaiters[0]
		c.keyWaiters = c.keyWaiters[1:]
		select {
		case w <- ev:
		default:
		}
	}
	handlers := make([]func(KeyEvent), 0, len(c.keyHandlers))
	for _, h := range c.keyHandlers {
		handlers = append(handlers, h)
	}
	c.keyMu.Unlock()
	for _, h := range handlers {
		// Isolate each handler: a panic must not propagate to the tview
		// application goroutine, whose event loop re-panics out of app.Run()
		// and would crash the whole process. One bad handler must not kill
		// the dispatcher.
		func(h func(KeyEvent)) {
			defer func() { _ = recover() }()
			h(ev)
		}(h)
	}
}

// toKeyEvent converts a tcell key event to a KeyEvent descriptor.
func toKeyEvent(ev *tcell.EventKey) KeyEvent {
	mod := ev.Modifiers()
	ke := KeyEvent{
		Ctrl:  mod&tcell.ModCtrl != 0,
		Alt:   mod&tcell.ModAlt != 0,
		Shift: mod&tcell.ModShift != 0,
	}
	if ev.Key() == tcell.KeyRune {
		ke.Name = "Rune"
		ke.Rune = string(ev.Rune())
		return ke
	}
	if name, ok := tcell.KeyNames[ev.Key()]; ok {
		ke.Name = name
	} else {
		ke.Name = fmt.Sprintf("Key(%d)", int(ev.Key()))
	}
	return ke
}

// wrapFlags maps a leaf's wrap mode to tview's SetWrap/SetWordWrap flags.
// "" and "char" char-wrap; "word" wraps at word boundaries; "off" disables.
func wrapFlags(mode string) (wrap, word bool) {
	switch mode {
	case "off":
		return false, false
	case "word":
		return true, true
	default: // "" or "char"
		return true, false
	}
}

// buildTextViews populates each leaf's TextView in panes. Called by
// StartScreen before buildFlex stitches them into the Flex tree.
func buildTextViews(node LayoutNode, panes map[string]*paneState, app *tview.Application) {
	if node.IsLeaf() {
		ps := panes[node.Name]
		wrap, word := wrapFlags(node.Wrap)
		colorOn := node.Color == nil || *node.Color
		tv := tview.NewTextView().
			SetDynamicColors(colorOn).
			SetScrollable(true).
			SetWrap(wrap).
			SetWordWrap(word)
		tv.SetBorder(true).SetTitle(" " + paneTitle(node) + " ").SetBorderColor(tcell.ColorGray)
		// Trigger a redraw whenever text is appended. Combined with
		// ScrollToEnd's trackEnd flag (below) the view follows the tail.
		tv.SetChangedFunc(func() { app.Draw() })
		// Autoscroll defaults on; an explicit { autoscroll: false } leaf
		// keeps the pane pinned at the top. Manual scroll-up clears tview's
		// trackEnd flag (pausing follow); scrolling back / End re-enables it.
		if node.AutoScroll == nil || *node.AutoScroll {
			tv.ScrollToEnd()
		}
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
