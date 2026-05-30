package main

import (
	"sync"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// inbound is one received unit: a byte payload plus optional metadata
// (UDP/ICMP attach sender address etc.). err non-nil ends the stream.
type inbound struct {
	payload []byte
	meta    map[string]any
	err     error
}

// pushSocket is the shared machinery behind net.tcp/udp/icmp handles. A
// reader goroutine (protocol-specific, provided by the caller) feeds recv;
// a single dispatcher goroutine — started on first onData/onMessage
// registration — drains recv and invokes the JS data callback on the loop
// via LoopCallable. HoldRun keeps loop.Run alive while open.
type pushSocket struct {
	vm   *goja.Runtime
	loop *eventloop.EventLoop

	recv chan inbound  // buffered; reader → dispatcher
	done chan struct{} // closed by closeFromScript to unblock a parked reader

	mu       sync.Mutex
	onData   *scriptengine.LoopCallable
	onClose  *scriptengine.LoopCallable
	onError  *scriptengine.LoopCallable
	dispatch bool // dispatcher started?

	closeOnce sync.Once
	closed    atomic.Bool
	release   func()       // HoldRun release (idempotent)
	teardown  func() error // protocol-specific: cancel ctx, close conn
}

func newPushSocket(vm *goja.Runtime, loop *eventloop.EventLoop, bufSize int) *pushSocket {
	if bufSize <= 0 {
		bufSize = 64
	}
	return &pushSocket{vm: vm, loop: loop, recv: make(chan inbound, bufSize), done: make(chan struct{})}
}

// startDispatch is called once, under mu, when the first data callback is
// registered. It owns draining recv for the socket's lifetime.
func (s *pushSocket) startDispatch() {
	if s.dispatch {
		return
	}
	s.dispatch = true
	go func() {
		for in := range s.recv {
			if in.err != nil {
				s.fireError(in.err)
				break
			}
			s.fireData(in.payload, in.meta)
		}
		s.fireClose()
		s.releaseOnce()
	}()
}

func (s *pushSocket) fireData(payload []byte, meta map[string]any) {
	s.mu.Lock()
	cb := s.onData
	s.mu.Unlock()
	if cb == nil {
		return
	}
	// buildArgs runs on the loop: safe to build goja values here.
	_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
		ev := vm.NewObject()
		_ = ev.Set("bytes", vm.ToValue(payload))
		_ = ev.Set("text", string(payload))
		for k, v := range meta {
			_ = ev.Set(k, vm.ToValue(v))
		}
		return []goja.Value{ev}, nil
	})
}

func (s *pushSocket) fireError(err error) {
	s.mu.Lock()
	cb := s.onError
	s.mu.Unlock()
	if cb == nil {
		return
	}
	_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
		return []goja.Value{vm.ToValue(err.Error())}, nil
	})
}

func (s *pushSocket) fireClose() {
	s.mu.Lock()
	cb := s.onClose
	s.mu.Unlock()
	if cb == nil {
		return
	}
	_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
		return nil, nil
	})
}

// sendInbound hands one received unit to the dispatcher. A reader MUST use
// this rather than a bare `s.recv <- in`: if the buffer is full and no
// consumer is draining (e.g. no data callback registered), a bare send parks
// forever, and a later close() can't unblock it (the reader isn't in
// conn.Read, so closing the conn doesn't help). Returns false once the socket
// is closed, signalling the reader to exit and let `defer close(s.recv)` run.
func (s *pushSocket) sendInbound(in inbound) bool {
	select {
	case s.recv <- in:
		return true
	case <-s.done:
		return false
	}
}

func (s *pushSocket) releaseOnce() {
	s.closeOnce.Do(func() {
		if s.release != nil {
			s.release()
		}
	})
}

// closeFromScript: teardown the conn (stops the reader → recv closes →
// dispatcher drains, fires onClose, releases the HoldRun). Idempotent.
//
// If no data callback was ever registered the dispatcher never started, so
// nothing will drain recv or release the HoldRun — a fire-and-forget socket
// (e.g. `u.send(...); await u.close()` with no onMessage) would otherwise
// keep loop.Run alive until the script times out. In that case release here.
func (s *pushSocket) closeFromScript() error {
	if s.closed.Swap(true) {
		return nil
	}
	// Signal first, then tear down: a reader parked on a full-buffer send to
	// recv (no consumer draining) isn't sitting in conn.Read, so closing the
	// conn alone wouldn't unblock it. Closing done lets sendInbound bail.
	close(s.done)
	var err error
	if s.teardown != nil {
		err = s.teardown()
	}
	s.mu.Lock()
	started := s.dispatch
	s.mu.Unlock()
	if !started {
		s.releaseOnce()
	}
	return err
}

// installSocketCallbacks wires the callback-registration and close methods
// onto the JS handle object. dataMethod is "onData" (TCP) or "onMessage"
// (UDP/ICMP); registering it starts the dispatcher on first call.
func installSocketCallbacks(obj *goja.Object, s *pushSocket, dataMethod string) {
	register := func(field **scriptengine.LoopCallable, startDispatch bool) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			fn, ok := goja.AssertFunction(call.Argument(0))
			if !ok {
				panic(s.vm.NewTypeError("%s: callback function required", dataMethod))
			}
			s.mu.Lock()
			*field = scriptengine.NewLoopCallable(s.loop, fn)
			if startDispatch {
				s.startDispatch()
			}
			s.mu.Unlock()
			return goja.Undefined()
		}
	}
	_ = obj.Set(dataMethod, register(&s.onData, true))
	_ = obj.Set("onClose", register(&s.onClose, false))
	_ = obj.Set("onError", register(&s.onError, false))
	_ = obj.Set("close", func(goja.FunctionCall) goja.Value {
		if err := s.closeFromScript(); err != nil {
			panic(s.vm.NewGoError(err))
		}
		return goja.Undefined()
	})
}
