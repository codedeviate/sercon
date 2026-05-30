package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// tcpNamespace wires net.tcp.* — TCP client sockets with a push/callback
// read model (onData / onClose / onError + write + close).
func tcpNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"connect": tcpConnectFn(vm, loop, eng),
	}
}

// tcpConnectFn implements net.tcp.connect(host, port, opts?). It returns a
// Promise that resolves to a handle object. Because the resolved value is a
// goja object built from the per-Run vm, the dial happens off-loop in a
// goroutine and the handle is built back ON the loop via RunOnLoop — the
// same hand-rolled pattern used by server_ws.go (PromisifyAsync isn't
// ergonomic for returning a live method-bearing object). opts:
// { timeout?, readBuffer? } — timeout in ms (default 10s), readBuffer is
// the inbound channel capacity (default 64).
func tcpConnectFn(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		port := call.Argument(1).String()

		// Parse opts on the loop (goja values aren't safe off-loop).
		timeout := 10 * time.Second
		bufSize := 64
		if opts := call.Argument(2); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			m, ok := opts.Export().(map[string]any)
			if ok {
				timeout = optMillis(m, "timeout", timeout)
				if n := optInt(m, "readBuffer", bufSize); n > 0 {
					bufSize = n
				}
			}
		}

		promise, resolve, reject := vm.NewPromise()
		addr := net.JoinHostPort(host, port)
		// Hold the loop alive across the off-loop dial: vm.NewPromise() +
		// a goroutine does not bump the loop's jobCount, so without this
		// the loop could exit before the dial resolves. Released on the
		// loop once the promise settles.
		dialHold := eng.HoldRun("tcp connect " + addr)
		go func() {
			d := net.Dialer{Timeout: timeout}
			conn, err := d.DialContext(context.Background(), "tcp", addr)
			loop.RunOnLoop(func(vm *goja.Runtime) {
				dialHold()
				if err != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("net.tcp.connect: %w", err)))
					return
				}
				obj, _ := buildTCPObject(vm, loop, eng, conn, bufSize)
				_ = resolve(obj)
			})
		}()
		return vm.ToValue(promise)
	}
}

// buildTCPObject constructs the JS handle for a connected TCP socket. It
// MUST run on the loop (it builds goja values). It starts the reader
// goroutine, registers the HoldRun sentinel + teardown, and wires the
// write method plus the shared onData/onClose/onError/close callbacks. It
// returns both the JS handle and the underlying *pushSocket so a caller
// (e.g. server.tcp.listen) can drive closeFromScript / onRelease for an
// accepted connection; the client path ignores the second value.
func buildTCPObject(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, conn net.Conn, bufSize int) (*goja.Object, *pushSocket) {
	remoteAddr := conn.RemoteAddr().String()
	s := newPushSocket(vm, loop, bufSize)
	s.release = eng.HoldRun("tcp " + remoteAddr)
	s.teardown = func() error { return conn.Close() }

	// Reader goroutine: read into a reused buffer; copy each chunk before
	// handing it to the channel (the buffer is reused — copying is
	// mandatory). On error close recv; forward non-EOF errors first.
	go func() {
		defer close(s.recv)
		buf := make([]byte, 32*1024)
		for {
			n, err := conn.Read(buf)
			if n > 0 {
				cp := append([]byte(nil), buf[:n]...)
				if !s.sendInbound(inbound{payload: cp}) {
					return
				}
			}
			if err != nil {
				if err != io.EOF {
					s.sendInbound(inbound{err: err})
				}
				return
			}
		}
	}()

	obj := vm.NewObject()
	_ = obj.Set("remote", remoteAddr)
	_ = obj.Set("local", conn.LocalAddr().String())
	_ = obj.Set("write", tcpWriteFn(vm, loop, conn, s))
	installSocketCallbacks(obj, s, "onData")
	return obj, s
}

// tcpWriteFn implements handle.write(data): a Promise that writes the
// payload to the connection. The payload (string→UTF-8 bytes, Uint8Array→
// bytes) is snapshotted ON the loop, then written OFF the loop in a
// goroutine, resolving/rejecting back on the loop (mirrors ws.send).
func tcpWriteFn(vm *goja.Runtime, loop *eventloop.EventLoop, conn net.Conn, s *pushSocket) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if s.closed.Load() {
			panic(vm.NewTypeError("write: connection closed"))
		}
		arg := call.Argument(0)
		promise, resolve, reject := vm.NewPromise()

		// Snapshot the payload on the loop.
		var payload []byte
		if bs, ok := arg.Export().([]byte); ok {
			payload = bs
		} else {
			payload = []byte(arg.String())
		}

		go func() {
			_, werr := conn.Write(payload)
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if werr != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("net.tcp.write: %w", werr)))
				} else {
					_ = resolve(goja.Undefined())
				}
			})
		}()
		return vm.ToValue(promise)
	}
}
