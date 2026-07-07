package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// tcpListenerHookForTest, when non-nil, is invoked with the bound
// net.Listener right after a successful bind. It exists solely so tests can
// grab the raw listener and close it out from under the accept loop to
// simulate a genuine (non-close()) accept error, without exposing the
// listener on the JS handle. Nil in production; never touched outside tests.
var tcpListenerHookForTest func(net.Listener)

// tcpServerMembers builds the {listen} map exposed as server.tcp.
func tcpServerMembers(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"listen": func(call goja.FunctionCall) goja.Value {
			return tcpListen(vm, loop, eng, call)
		},
	}
}

// tcpListen implements `server.tcp.listen({ port, host?, readBuffer? }, conn => …)`.
// Mirrors httpListen's lifecycle: synchronous bind (so bind errors throw
// immediately), an accept loop in a background goroutine, one eng.HoldRun
// keeping the loop alive while bound, and a `{ address, close() }` handle.
// Each accepted connection is wrapped with the same buildTCPObject handle the
// net.tcp.connect client uses, and the per-connection handler is invoked on
// the loop via LoopCallable (which is the correct marshalling primitive for
// the off-loop accept goroutine).
func tcpListen(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, call goja.FunctionCall) goja.Value {
	opts := call.Argument(0)
	if opts == nil || goja.IsUndefined(opts) || goja.IsNull(opts) {
		panic(vm.NewTypeError("server.tcp.listen: options object required"))
	}

	host := ""
	bufSize := 64
	port := 0
	if m, ok := opts.Export().(map[string]any); ok {
		host = optString(m, "host", "")
		if n := optInt(m, "readBuffer", bufSize); n > 0 {
			bufSize = n
		}
		port = optInt(m, "port", 0)
	}
	// `sercon serve --port-override N` replaces every server.*.listen port.
	if servePortOverride != 0 {
		port = servePortOverride
	}

	fn, ok := goja.AssertFunction(call.Argument(1))
	if !ok {
		panic(vm.NewTypeError("server.tcp.listen: handler function required"))
	}
	handler := scriptengine.NewLoopCallable(loop, fn)

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("server.tcp.listen %s: %w", addr, err)))
	}
	if tcpListenerHookForTest != nil {
		tcpListenerHookForTest(ln)
	}
	if serveReadyWriter != nil {
		fmt.Fprintf(serveReadyWriter, "READY listening on tcp/%s\n", ln.Addr().String())
	}
	release := eng.HoldRun("server.tcp listen " + ln.Addr().String())

	// Track accepted connections so the server's close() can tear them down.
	// Without this, an accepted-but-unclosed connection keeps its own HoldRun
	// outstanding after the server closes, hanging the loop to timeout
	// (asymmetric with server.http, whose Shutdown drains in-flight conns).
	var (
		connMu  sync.Mutex
		conns   = map[*pushSocket]struct{}{}
		closing bool
	)

	closeOnce := atomic.Bool{}
	// doClose tears down the listener + all accepted connections and releases
	// the HoldRun. Shared by the JS close(), the shutdown hook, and the accept
	// loop's own error-exit path (below) — every one of those can race to be
	// first, so closeOnce makes it safe to call from all three. Every step is
	// off-loop-safe: ln.Close() unblocks the accept loop, closeFromScript
	// works via atomics/channels/net (no vm access — the onClose JS callback
	// it triggers is marshalled onto the loop separately), and release()
	// (ClearTimeout) is enqueued as a loop aux-job.
	doClose := func() {
		if closeOnce.Swap(true) {
			return
		}
		_ = ln.Close()
		// Snapshot and close active connections through their handle's
		// closeFromScript, which releases each per-connection HoldRun
		// (incl. the no-dispatcher path). onRelease deregisters them.
		connMu.Lock()
		closing = true
		snapshot := make([]*pushSocket, 0, len(conns))
		for s := range conns {
			snapshot = append(snapshot, s)
		}
		connMu.Unlock()
		for _, s := range snapshot {
			_ = s.closeFromScript()
		}
		release()
	}

	// Accept loop (off-loop). For each connection, build the handle + invoke
	// the handler ON the loop via LoopCallable (buildTCPObject needs the vm
	// and must run on the loop).
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Accept() fails both when the listener was closed via
				// doClose() (srv.close() / shutdown hook — where doClose is a
				// no-op here, already run) AND on a genuine, unrelated accept
				// error (e.g. the listener's fd was closed or errored out from
				// under us some other way). Either way the loop is done, so
				// always drive doClose(): idempotent on the former, and on the
				// latter it's what actually releases the HoldRun sentinel and
				// closes the listener — without this call that path leaked
				// both, keeping the event loop (and Run) alive forever.
				doClose()
				return
			}
			_, _ = handler.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
				obj, s := buildTCPObject(vm, loop, eng, conn, bufSize)
				// Deregister on release (natural EOF/peer close, the handle's
				// close(), or the server's close()) so the set can't grow
				// unbounded over a long-lived server.
				s.onRelease = func() {
					connMu.Lock()
					delete(conns, s)
					connMu.Unlock()
				}
				connMu.Lock()
				if closing {
					// Server closed during the accept→build window: don't keep
					// the connection; close it so its hold is released too.
					connMu.Unlock()
					go func() { _ = s.closeFromScript() }()
				} else {
					conns[s] = struct{}{}
					connMu.Unlock()
				}
				return []goja.Value{obj}, nil
			})
		}
	}()

	handle := vm.NewObject()
	_ = handle.Set("address", fmt.Sprintf("tcp/%s", ln.Addr().String()))

	// Register a graceful-shutdown hook so `sercon serve` can close this
	// listener on SIGTERM/SIGINT. An explicit close() removes it first.
	removeHook := eng.AddShutdownHook(func(context.Context) error {
		doClose()
		return nil
	})

	// close() returns Promise<void> for parity with the rest of server.* (the
	// drain is synchronous, so it resolves immediately).
	_ = handle.Set("close", func(goja.FunctionCall) goja.Value {
		promise, resolve, _ := vm.NewPromise()
		removeHook() // don't let GracefulShutdown close it a second time
		doClose()
		_ = resolve(goja.Undefined())
		return vm.ToValue(promise)
	})
	return handle
}
