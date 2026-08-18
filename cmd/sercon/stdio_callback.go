package main

import (
	"bytes"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
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
type lineCallback struct {
	mu       sync.Mutex
	buf      []byte   // bytes since the last newline
	queue    []string // complete lines awaiting delivery
	queueCap int
	draining bool // a drain is scheduled or running
	inCall   bool // the handler is executing right now (re-entrancy guard)
	stopped  bool

	fn      goja.Callable
	loop    *eventloop.EventLoop
	eng     *scriptengine.Engine
	release func() // the active HoldRun release, non-nil while queued
}

// callbackDest builds a destCallback entry from a JS function target.
func callbackDest(loop *eventloop.EventLoop, e *scriptengine.Engine, fn goja.Callable, tee bool) (destination, error) {
	return destination{
		kind: destCallback,
		cb:   &lineCallback{fn: fn, loop: loop, eng: e, queueCap: lineQueueCap},
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
		if c.eng != nil {
			c.release = c.eng.HoldRun("stdio line callback")
		}
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
			release := c.release
			c.release = nil
			c.mu.Unlock()
			if release != nil {
				release()
			}
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

// stop marks the callback dead and releases any HoldRun it was holding.
func (c *lineCallback) stop() {
	c.mu.Lock()
	c.stopped = true
	c.queue = nil
	release := c.release
	c.release = nil
	c.mu.Unlock()
	if release != nil {
		release()
	}
}
