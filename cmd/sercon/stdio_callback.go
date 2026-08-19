package main

import (
	"bytes"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// lineQueueCap bounds how many complete lines may await delivery to a JS
// handler. Past it, writers fall through to the destination beneath instead of
// blocking — a server handler's console.log must never park on a slow handler.
const lineQueueCap = 1024

// lineCallback is the destCallback payload: it splits the byte stream into
// lines and delivers each to a JS function.
//
// Delivery is queued, never direct. Writes arrive from arbitrary goroutines
// (server handlers, exec.stream, async ops) AND from on-loop script code, and
// per CLAUDE.md a raw goja.Callable from off-loop is illegal while
// LoopCallable.Call from on-loop deadlocks. Enqueueing plus a fire-and-forget
// loop.RunOnLoop drain sidesteps both with one mechanism: delivery is one tick
// late but strictly ordered.
//
// The guarantee this establishes: a line is delivered to the handler while
// the loop is turning, and otherwise written to the destination beneath —
// never dropped. There is deliberately no HoldRun keeping the run alive on
// the queue's behalf (an earlier version of this file parked one, but
// acquiring it from tryFeed — which runs off-loop as often as not — meant
// touching the shared goja.Runtime from a foreign goroutine). The accepted
// cost is that a line written moments before the run ends reaches the
// destination beneath rather than the handler: see stop() and the
// destCallback case in stream.closeDest.
type lineCallback struct {
	mu       sync.Mutex
	buf      []byte   // bytes since the last newline
	queue    []string // complete lines awaiting delivery
	queueCap int
	draining bool // a drain is scheduled or running
	inCall   bool // the handler is executing right now (re-entrancy guard)
	stopped  bool

	fn   goja.Callable
	loop *eventloop.EventLoop
}

// callbackDest builds a destCallback entry from a JS function target.
func callbackDest(loop *eventloop.EventLoop, fn goja.Callable, tee bool) (destination, error) {
	return destination{
		kind: destCallback,
		cb:   &lineCallback{fn: fn, loop: loop, queueCap: lineQueueCap},
		tee:  tee,
	}, nil
}

// tryFeed accepts p and reports whether it took ownership. false means the
// caller must fall through to the destination beneath:
//   - inCall: the handler's own write would otherwise recurse forever
//   - stopped: the entry has been popped
//   - the queue is full
//
// Called with the stream's mutex held, so this must not block or call into JS.
func (c *lineCallback) tryFeed(p []byte) bool {
	c.mu.Lock()
	if c.stopped || c.inCall {
		c.mu.Unlock()
		return false
	}

	pending := append(c.buf, p...)
	var lines []string
	for {
		i := bytes.IndexByte(pending, '\n')
		if i < 0 {
			break
		}
		lines = append(lines, string(pending[:i]))
		pending = pending[i+1:]
	}

	if len(c.queue)+len(lines) > c.queueCap {
		// Overflow: keep nothing from this write so the caller can fall
		// through with the original bytes intact and in order.
		c.mu.Unlock()
		return false
	}

	c.buf = append(c.buf[:0], pending...)
	c.queue = append(c.queue, lines...)
	needDrain := len(c.queue) > 0 && !c.draining
	if needDrain {
		c.draining = true
	}
	loop := c.loop
	c.mu.Unlock()

	if needDrain && loop != nil {
		loop.RunOnLoop(func(vm *goja.Runtime) { c.drain(vm) })
	}
	return true
}

// drain delivers every queued line. Runs ON the loop, so it invokes the
// callable directly — scheduling again from here would deadlock.
func (c *lineCallback) drain(vm *goja.Runtime) {
	for {
		c.mu.Lock()
		if c.stopped || len(c.queue) == 0 {
			c.draining = false
			c.mu.Unlock()
			return
		}
		line := c.queue[0]
		c.queue = c.queue[1:]
		c.inCall = true
		fn := c.fn
		c.mu.Unlock()

		// A throw from the handler must not abort the drain or leak inCall.
		func() {
			defer func() {
				c.mu.Lock()
				c.inCall = false
				c.mu.Unlock()
				_ = recover() // a handler that throws loses that line, nothing else
			}()
			_, _ = fn(goja.Undefined(), vm.ToValue(line))
		}()
	}
}

// takePartial removes and returns any buffered bytes that never got a newline.
// The stream writes them to the destination beneath when this entry is popped.
func (c *lineCallback) takePartial() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	rest := c.buf
	c.buf = nil
	return rest
}

// stop marks the callback dead and returns any complete lines that were
// queued but never delivered, oldest first. There is no live event loop
// guarantee here (see the package doc comment above), so the stream writes
// these to the destination beneath rather than losing them — the same
// deal takePartial makes for a trailing partial line, and closeDest applies
// both in queue order: whole lines first, then the partial.
//
// Called with the stream's mutex held (closeDest is only reached from
// pop()/reset(), both locked), so this must not block or call into JS.
func (c *lineCallback) stop() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
	leftover := c.queue
	c.queue = nil
	return leftover
}
