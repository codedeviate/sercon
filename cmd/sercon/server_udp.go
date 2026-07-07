package main

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// udpConnHookForTest, when non-nil, is invoked with the bound *net.UDPConn
// right after a successful bind. It exists solely so tests can grab the raw
// conn and close it out from under the read loop to simulate a genuine
// (non-close()) read error, without exposing the conn on the JS handle. Nil
// in production; never touched outside tests.
var udpConnHookForTest func(*net.UDPConn)

// udpServerMembers builds the {listen} map exposed as server.udp.
func udpServerMembers(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"listen": func(call goja.FunctionCall) goja.Value {
			return udpListen(vm, loop, eng, call)
		},
	}
}

// udpListen implements `server.udp.listen({ port, host? }, (msg, reply) => …)`.
// Mirrors tcpListen's lifecycle: synchronous bind (so bind errors throw
// immediately), a read loop in a background goroutine, one eng.HoldRun keeping
// the loop alive while bound, and a `{ address, close() }` handle. Each
// datagram invokes the handler on the loop via LoopCallable with a message
// object `{ bytes, text, address, port }` and a `reply(data)` function bound to
// that datagram's sender. There is no per-connection handle (UDP is
// connectionless); reply snapshots the payload on the loop and writes it back
// off-loop via WriteToUDP, mirroring socket_udp.go's udpSendToFn.
func udpListen(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, call goja.FunctionCall) goja.Value {
	opts := call.Argument(0)
	if opts == nil || goja.IsUndefined(opts) || goja.IsNull(opts) {
		panic(vm.NewTypeError("server.udp.listen: options object required"))
	}

	host := ""
	port := 0
	if m, ok := opts.Export().(map[string]any); ok {
		host = optString(m, "host", "")
		port = optInt(m, "port", 0)
	}
	// `sercon serve --port-override N` replaces every server.*.listen port.
	if servePortOverride != 0 {
		port = servePortOverride
	}

	fn, ok := goja.AssertFunction(call.Argument(1))
	if !ok {
		panic(vm.NewTypeError("server.udp.listen: handler function required"))
	}
	handler := scriptengine.NewLoopCallable(loop, fn)

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	laddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("server.udp.listen %s: %w", addr, err)))
	}
	conn, err := net.ListenUDP("udp", laddr)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("server.udp.listen %s: %w", addr, err)))
	}
	if udpConnHookForTest != nil {
		udpConnHookForTest(conn)
	}
	if serveReadyWriter != nil {
		fmt.Fprintf(serveReadyWriter, "READY listening on udp/%s\n", conn.LocalAddr().String())
	}
	release := eng.HoldRun("server.udp listen " + conn.LocalAddr().String())

	closeOnce := atomic.Bool{}
	// doClose closes the socket (unblocking the read loop) and releases the
	// HoldRun. Shared by the JS close(), the shutdown hook, and the read
	// loop's own error-exit path (below) — closeOnce makes it safe for all
	// three to race to be first. Both conn.Close() and release() (ClearTimeout
	// via the loop aux-job queue) are safe to call from a non-loop goroutine.
	doClose := func() {
		if closeOnce.Swap(true) {
			return
		}
		_ = conn.Close()
		release()
	}

	// Read loop (off-loop). For each datagram, copy the bytes and capture the
	// sender, then invoke the handler ON the loop via LoopCallable. The message
	// object and reply function are built inside the on-loop buildArgs.
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, sender, err := conn.ReadFromUDP(buf)
			if n > 0 {
				cp := append([]byte(nil), buf[:n]...)
				sender := sender // capture for reply
				_, _ = handler.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
					msg := vm.NewObject()
					_ = msg.Set("bytes", vm.ToValue(cp))
					_ = msg.Set("text", string(cp))
					if sender != nil {
						_ = msg.Set("address", sender.IP.String())
						_ = msg.Set("port", sender.Port)
					}
					reply := func(call goja.FunctionCall) goja.Value {
						payload := snapshotPayload(call.Argument(0)) // on loop
						promise, resolve, reject := vm.NewPromise()
						go func() {
							_, werr := conn.WriteToUDP(payload, sender)
							loop.RunOnLoop(func(vm *goja.Runtime) {
								if werr != nil {
									_ = reject(vm.NewGoError(fmt.Errorf("server.udp reply: %w", werr)))
								} else {
									_ = resolve(goja.Undefined())
								}
							})
						}()
						return vm.ToValue(promise)
					}
					return []goja.Value{msg, vm.ToValue(reply)}, nil
				})
			}
			if err != nil {
				// ReadFromUDP() fails both when the socket was closed via
				// doClose() (srv.close() / shutdown hook — doClose is then a
				// no-op here) AND on a genuine, unrelated read error. Either
				// way the loop is done, so always drive doClose(): idempotent
				// on the former, and on the latter it's what actually
				// releases the HoldRun sentinel and closes the socket —
				// without this call that path leaked both, keeping the event
				// loop (and Run) alive forever.
				doClose()
				return
			}
		}
	}()

	handle := vm.NewObject()
	_ = handle.Set("address", fmt.Sprintf("udp/%s", conn.LocalAddr().String()))

	// Register a graceful-shutdown hook so `sercon serve` can close this
	// listener on SIGTERM/SIGINT. An explicit close() removes it first.
	removeHook := eng.AddShutdownHook(func(context.Context) error {
		doClose()
		return nil
	})

	// close() returns Promise<void> for parity with the rest of server.*.
	_ = handle.Set("close", func(goja.FunctionCall) goja.Value {
		promise, resolve, _ := vm.NewPromise()
		removeHook() // don't let GracefulShutdown close it a second time
		doClose()
		_ = resolve(goja.Undefined())
		return vm.ToValue(promise)
	})
	return handle
}
