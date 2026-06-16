package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// sseEvent is one Server-Sent Event before wire encoding. Empty string
// fields are omitted by formatSSEEvent; retry <= 0 is omitted.
type sseEvent struct {
	event string
	data  string
	id    string
	retry int // ms
}

// formatSSEEvent renders an event to text/event-stream wire bytes. Field
// order is retry, id, event, then one `data:` line per line of data, then
// the blank-line terminator. CRLF in data is normalized to LF first.
func formatSSEEvent(ev sseEvent) []byte {
	var b strings.Builder
	if ev.retry > 0 {
		fmt.Fprintf(&b, "retry: %d\n", ev.retry)
	}
	if ev.id != "" {
		fmt.Fprintf(&b, "id: %s\n", ev.id)
	}
	if ev.event != "" {
		fmt.Fprintf(&b, "event: %s\n", ev.event)
	}
	data := strings.ReplaceAll(ev.data, "\r\n", "\n")
	for _, line := range strings.Split(data, "\n") {
		fmt.Fprintf(&b, "data: %s\n", line)
	}
	b.WriteByte('\n')
	return []byte(b.String())
}

// sseFrame is one formatted frame plus a buffered ack channel. The pump
// writes data, flushes, and sends the write error (or nil) on ack so the
// JS send() Promise resolves only after the bytes hit the socket.
type sseFrame struct {
	data []byte
	ack  chan error // buffered(1): pump never blocks on a sender that gave up
}

// sseStream tracks one open SSE response.
//
// events is never closed: a send() runs from an off-loop goroutine, and
// sending on a closed channel panics (a select send-case on a closed channel
// fires rather than blocking). Instead, explicit close() closes quit, which
// the pump selects on to exit; senders select on done (closed by the pump on
// exit) to reject. So nothing ever sends on a closed channel.
type sseStream struct {
	events    chan sseFrame // formatted frames headed for the pump (never closed)
	done      chan struct{} // == responseState.streamDone; closed once the pump exits
	quit      chan struct{} // closed by close() to ask the pump to stop
	closeOnce sync.Once     // guards close(quit)
	release   func()        // HoldRun release (idempotent)
}

// buildSSEEvent converts the JS send() argument into an sseEvent. A string
// argument is data-only; an object must carry `data` (string passthrough,
// anything else JSON-encoded) plus optional event/id/retry.
func buildSSEEvent(vm *goja.Runtime, arg goja.Value) (sseEvent, error) {
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return sseEvent{}, fmt.Errorf("res.sse send: argument required")
	}
	if s, ok := arg.Export().(string); ok {
		return sseEvent{data: s}, nil
	}
	o := arg.ToObject(vm)
	ev := sseEvent{}
	if v := o.Get("event"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		ev.event = v.String()
	}
	if v := o.Get("id"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		ev.id = v.String()
	}
	if v := o.Get("retry"); v != nil && !goja.IsUndefined(v) {
		ev.retry = int(v.ToInteger())
	}
	dv := o.Get("data")
	if dv == nil || goja.IsUndefined(dv) {
		return sseEvent{}, fmt.Errorf("res.sse send: object form requires `data`")
	}
	if s, ok := dv.Export().(string); ok {
		ev.data = s
	} else {
		raw, err := json.Marshal(dv.Export())
		if err != nil {
			return sseEvent{}, fmt.Errorf("res.sse send: json: %w", err)
		}
		ev.data = string(raw)
	}
	return ev, nil
}

// sseImpl backs res.sse(opts?). It claims the response for streaming, writes
// the SSE headers and flushes, marks the response final (unblocking the
// dispatcher, which then parks on streamDone), and starts the pump. Returns a
// JS handle with send(data) / close() / closed.
func sseImpl(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, state *responseState, w http.ResponseWriter, r *http.Request, opts goja.Value) goja.Value {
	flusher, ok := w.(http.Flusher)
	if !ok {
		panic(vm.NewTypeError("res.sse: response writer does not support streaming"))
	}

	keepAlive := 0
	retry := 0
	if opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
		o := opts.ToObject(vm)
		if v := o.Get("keepAlive"); v != nil && !goja.IsUndefined(v) {
			keepAlive = int(v.ToInteger())
		}
		if v := o.Get("retry"); v != nil && !goja.IsUndefined(v) {
			retry = int(v.ToInteger())
		}
	}

	// Claim the response: stream mode + skip the buffered writeResponse.
	state.mu.Lock()
	if state.finalized {
		state.mu.Unlock()
		panic(vm.NewTypeError("res.sse: response already finalized"))
	}
	state.stream = true
	state.upgrade = true // writeResponse short-circuits on upgrade
	state.streamDone = make(chan struct{})
	streamDone := state.streamDone
	state.mu.Unlock()

	// Write SSE headers + flush before unblocking the dispatcher. We are on
	// the loop goroutine here; dispatchHandler is still parked inside
	// loopSchedule, so this is the only goroutine touching w.
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no") // defeat nginx proxy buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	st := &sseStream{
		events:  make(chan sseFrame, 64),
		done:    streamDone,
		quit:    make(chan struct{}),
		release: eng.HoldRun(fmt.Sprintf("sse %s", r.RemoteAddr)),
	}

	closedPromise, closedResolve, _ := vm.NewPromise()

	teardown := func() {
		st.closeOnce.Do(func() { close(st.quit) })
	}

	// Pump goroutine: sole owner of w after this point. Writes frames and
	// keepalive pings, flushing each. Exits on events-close (graceful), a
	// write error (client gone), or request-context cancel (disconnect). On
	// exit it releases the HoldRun sentinel, closes streamDone (unparking
	// dispatchHandler so net/http finishes the response), and resolves the
	// `closed` Promise on the loop.
	go func() {
		var tickC <-chan time.Time
		if keepAlive > 0 {
			ticker := time.NewTicker(time.Duration(keepAlive) * time.Millisecond)
			tickC = ticker.C
			defer ticker.Stop()
		}
		defer func() {
			st.release()
			close(st.done)
			loop.RunOnLoop(func(vm *goja.Runtime) { _ = closedResolve(goja.Undefined()) })
		}()
		if retry > 0 {
			fmt.Fprintf(w, "retry: %d\n\n", retry)
			flusher.Flush()
		}
		for {
			select {
			case f := <-st.events:
				_, err := w.Write(f.data)
				if err == nil {
					flusher.Flush()
				}
				f.ack <- err // buffered(1), never blocks
				if err != nil {
					return
				}
			case <-tickC:
				if _, err := w.Write([]byte(": ping\n\n")); err != nil {
					return
				}
				flusher.Flush()
			case <-st.quit:
				return // explicit close()
			case <-r.Context().Done():
				return // client disconnected
			}
		}
	}()

	obj := vm.NewObject()
	_ = obj.Set("remote", r.RemoteAddr)
	_ = obj.Set("closed", closedPromise)

	// send(data) — string → data-only event; object → {event,data,id,retry}.
	// Resolves once the frame is written+flushed; rejects if the stream is
	// already closed/disconnected.
	_ = obj.Set("send", func(call goja.FunctionCall) goja.Value {
		ev, err := buildSSEEvent(vm, call.Argument(0))
		if err != nil {
			panic(vm.NewGoError(err))
		}
		frame := sseFrame{data: formatSSEEvent(ev), ack: make(chan error, 1)}
		promise, resolve, reject := vm.NewPromise()
		go func() {
			rejectClosed := func() {
				loop.RunOnLoop(func(vm *goja.Runtime) { _ = reject(vm.NewTypeError("res.sse: stream closed")) })
			}
			select {
			case st.events <- frame:
				select {
				case werr := <-frame.ack:
					loop.RunOnLoop(func(vm *goja.Runtime) {
						if werr != nil {
							_ = reject(vm.NewGoError(werr))
						} else {
							_ = resolve(goja.Undefined())
						}
					})
				case <-st.done:
					rejectClosed()
				}
			case <-st.quit:
				rejectClosed()
			case <-st.done:
				rejectClosed()
			}
		}()
		return vm.ToValue(promise)
	})

	// close() — graceful end: stop the pump and resolve once it has exited.
	_ = obj.Set("close", func(call goja.FunctionCall) goja.Value {
		promise, resolve, _ := vm.NewPromise()
		teardown()
		go func() {
			<-st.done
			loop.RunOnLoop(func(vm *goja.Runtime) { _ = resolve(goja.Undefined()) })
		}()
		return vm.ToValue(promise)
	})

	return vm.ToValue(obj)
}
