package scriptengine

import (
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// holdRunEntry tracks one outstanding HoldRun sentinel for diagnostics
// and idempotent release.
type holdRunEntry struct {
	reason   string
	timer    *eventloop.Timer
	released atomic.Bool
}

// HoldRun keeps loop.Run from exiting until the returned release function
// runs. Useful for long-lived bindings (servers, listeners) that have no
// outstanding goja Promise to keep the loop's jobCount nonzero. Internally
// parks a 24h goja-eventloop SetTimeout sentinel; release() clears it.
//
// Multiple concurrent HoldRun calls are supported; each parks its own
// sentinel and decrements independently. release() is idempotent —
// calling it twice is safe.
//
// reason is a short label used in diagnostics (engine traces) so a
// scriptengine debugger can see which binding is keeping the loop alive.
//
// HoldRun must be called from a binding factory or a binding callback —
// it requires the per-Run EventLoop to be set, which happens at the start
// of Run. Calling HoldRun before Run has begun, or after Run has returned,
// returns a no-op release function.
func (e *Engine) HoldRun(reason string) (release func()) {
	e.holdRunMu.Lock()
	loop := e.holdRunLoop
	e.holdRunMu.Unlock()
	if loop == nil {
		// No active Run; return a no-op release. Bindings can still
		// safely use HoldRun in static contexts (the no-op fires on
		// release, nothing leaks).
		return func() {}
	}

	entry := &holdRunEntry{reason: reason}
	// Park a 24h sentinel. goja_nodejs/eventloop counts SetTimeout as
	// a live task, so the loop will not exit while this sits.
	entry.timer = loop.SetTimeout(func(*goja.Runtime) { /* never fires unless 24h elapses */ }, 24*time.Hour)

	e.holdRunMu.Lock()
	if e.holdRunSentinels == nil {
		e.holdRunSentinels = map[*holdRunEntry]struct{}{}
	}
	e.holdRunSentinels[entry] = struct{}{}
	e.holdRunMu.Unlock()

	e.trace("hold-run begin: %s", reason)

	return func() {
		if entry.released.Swap(true) {
			return // idempotent
		}
		// Snapshot loop reference because the engine may have moved on
		// to a new Run by the time release fires.
		e.holdRunMu.Lock()
		l := e.holdRunLoop
		delete(e.holdRunSentinels, entry)
		e.holdRunMu.Unlock()
		if l != nil {
			l.ClearTimeout(entry.timer)
		}
		e.trace("hold-run release: %s", reason)
	}
}

// holdRunBegin is called by Engine.Run at loop setup. Records the loop
// pointer for HoldRun calls to enqueue against. Resets any leftover
// sentinels from a prior Run (safety net; the cleanup drain should have
// done this already).
func (e *Engine) holdRunBegin(loop *eventloop.EventLoop) {
	e.holdRunMu.Lock()
	e.holdRunLoop = loop
	for entry := range e.holdRunSentinels {
		entry.released.Store(true)
	}
	e.holdRunSentinels = nil
	e.holdRunMu.Unlock()
}

// holdRunEnd is called by Engine.Run after the loop returns. Releases any
// leftover sentinels so the next Run starts clean.
func (e *Engine) holdRunEnd() {
	e.holdRunMu.Lock()
	loop := e.holdRunLoop
	entries := e.holdRunSentinels
	e.holdRunLoop = nil
	e.holdRunSentinels = nil
	e.holdRunMu.Unlock()
	if loop == nil {
		return
	}
	for entry := range entries {
		if !entry.released.Swap(true) {
			loop.ClearTimeout(entry.timer)
			e.trace("hold-run drain (leftover): %s", entry.reason)
		}
	}
}
