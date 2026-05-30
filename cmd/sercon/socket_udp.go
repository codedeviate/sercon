package main

import (
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// udpNamespace wires net.udp.* — UDP client sockets (connected + bound)
// with a push/callback read model (onMessage / onClose / onError + send +
// close).
func udpNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"open": udpOpenFn(vm, loop, eng),
	}
}

// udpOpenFn implements net.udp.open(opts). It returns a Promise that resolves
// to a handle object. Two modes, selected by opts:
//
//   - Connected { host, port, readBuffer? }: net.DialUDP to the resolved
//     peer; handle.send(data) writes to that peer; reader uses conn.Read.
//   - Bound { bind: ":9999", readBuffer? }: net.ListenUDP on the local
//     address; reader uses ReadFromUDP and attaches { address, port } meta;
//     handle.sendTo(data, host, port) writes to an arbitrary peer.
//
// The socket setup happens off-loop in a goroutine and the handle is built
// back ON the loop via RunOnLoop — the same hand-rolled pattern used by
// tcpConnectFn / server_ws.go (PromisifyAsync isn't ergonomic for returning a
// live method-bearing object). readBuffer is the inbound channel capacity
// (default 64).
func udpOpenFn(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		// Parse opts on the loop (goja values aren't safe off-loop).
		var (
			bind    string
			host    string
			port    string
			bufSize = 64
		)
		if opts := call.Argument(0); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			if m, ok := opts.Export().(map[string]any); ok {
				bind = optString(m, "bind", "")
				host = optString(m, "host", "")
				port = udpPortString(m, "port")
				if n := optInt(m, "readBuffer", bufSize); n > 0 {
					bufSize = n
				}
			}
		}

		promise, resolve, reject := vm.NewPromise()

		mode := ""
		switch {
		case bind != "":
			mode = "bound"
		case host != "" && port != "":
			mode = "connected"
		default:
			// Reject asynchronously so the returned value is always a Promise.
			_ = reject(vm.NewGoError(errors.New("net.udp.open: provide either { bind } or { host, port }")))
			return vm.ToValue(promise)
		}

		// Hold the loop alive across the off-loop setup: vm.NewPromise() + a
		// goroutine does not bump the loop's jobCount, so without this the loop
		// could exit before setup resolves. Released on the loop once settled.
		var holdReason string
		if mode == "bound" {
			holdReason = "udp open bind " + bind
		} else {
			holdReason = "udp open " + net.JoinHostPort(host, port)
		}
		openHold := eng.HoldRun(holdReason)

		go func() {
			if mode == "connected" {
				raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
				if err == nil {
					var conn *net.UDPConn
					conn, err = net.DialUDP("udp", nil, raddr)
					if err == nil {
						loop.RunOnLoop(func(vm *goja.Runtime) {
							openHold()
							obj := buildUDPObject(vm, loop, eng, conn, bufSize, false)
							_ = resolve(obj)
						})
						return
					}
				}
				loop.RunOnLoop(func(vm *goja.Runtime) {
					openHold()
					_ = reject(vm.NewGoError(fmt.Errorf("net.udp.open: %w", err)))
				})
				return
			}

			// bound
			laddr, err := net.ResolveUDPAddr("udp", bind)
			if err == nil {
				var conn *net.UDPConn
				conn, err = net.ListenUDP("udp", laddr)
				if err == nil {
					loop.RunOnLoop(func(vm *goja.Runtime) {
						openHold()
						obj := buildUDPObject(vm, loop, eng, conn, bufSize, true)
						_ = resolve(obj)
					})
					return
				}
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				openHold()
				_ = reject(vm.NewGoError(fmt.Errorf("net.udp.open: %w", err)))
			})
		}()

		return vm.ToValue(promise)
	}
}

// buildUDPObject constructs the JS handle for a UDP socket. It MUST run on the
// loop (it builds goja values). It starts the reader goroutine, registers the
// HoldRun sentinel + teardown, wires send/sendTo, and installs the shared
// onMessage/onClose/onError/close callbacks. bound selects the reader strategy
// (ReadFromUDP + meta) and which write methods are exposed.
func buildUDPObject(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, conn *net.UDPConn, bufSize int, bound bool) *goja.Object {
	localAddr := conn.LocalAddr().String()
	s := newPushSocket(vm, loop, bufSize)
	s.release = eng.HoldRun("udp " + localAddr)
	s.teardown = func() error { return conn.Close() }

	// Reader goroutine: one datagram per Read == one inbound event (UDP
	// preserves datagram boundaries). Copy each datagram before handing it to
	// the channel. On error close recv; a clean close (teardown closed the
	// conn) ends silently, otherwise forward the error as an event.
	go func() {
		defer close(s.recv)
		buf := make([]byte, 64*1024)
		for {
			if bound {
				n, addr, err := conn.ReadFromUDP(buf)
				if n > 0 {
					cp := append([]byte(nil), buf[:n]...)
					meta := map[string]any{}
					if addr != nil {
						meta["address"] = addr.IP.String()
						meta["port"] = addr.Port
					}
					s.recv <- inbound{payload: cp, meta: meta}
				}
				if err != nil {
					if !isClosedConnErr(err) {
						s.recv <- inbound{err: err}
					}
					return
				}
			} else {
				n, err := conn.Read(buf)
				if n > 0 {
					cp := append([]byte(nil), buf[:n]...)
					s.recv <- inbound{payload: cp}
				}
				if err != nil {
					if !isClosedConnErr(err) {
						s.recv <- inbound{err: err}
					}
					return
				}
			}
		}
	}()

	obj := vm.NewObject()
	_ = obj.Set("local", localAddr)
	if bound {
		_ = obj.Set("sendTo", udpSendToFn(vm, loop, conn, s))
		// send without a peer is meaningless on a bound socket.
		_ = obj.Set("send", func(goja.FunctionCall) goja.Value {
			panic(vm.NewTypeError("send: bound UDP socket has no peer; use sendTo(data, host, port)"))
		})
	} else {
		_ = obj.Set("send", udpSendFn(vm, loop, conn, s))
	}
	installSocketCallbacks(obj, s, "onMessage")
	return obj
}

// udpSendFn implements handle.send(data) on a connected UDP socket: a Promise
// that writes the payload to the connected peer via conn.Write. The payload
// (string→UTF-8 bytes, Uint8Array→bytes) is snapshotted ON the loop, then
// written OFF the loop, resolving/rejecting back on the loop (mirrors
// tcpWriteFn).
func udpSendFn(vm *goja.Runtime, loop *eventloop.EventLoop, conn *net.UDPConn, s *pushSocket) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if s.closed.Load() {
			panic(vm.NewTypeError("send: connection closed"))
		}
		payload := snapshotPayload(call.Argument(0))
		promise, resolve, reject := vm.NewPromise()
		go func() {
			_, werr := conn.Write(payload)
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if werr != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("net.udp.send: %w", werr)))
				} else {
					_ = resolve(goja.Undefined())
				}
			})
		}()
		return vm.ToValue(promise)
	}
}

// udpSendToFn implements handle.sendTo(data, host, port) on a bound UDP
// socket: a Promise that writes the payload to an arbitrary peer via
// WriteToUDP. The payload and destination are snapshotted ON the loop, the
// address resolved and write performed OFF the loop, resolving/rejecting back
// on the loop.
func udpSendToFn(vm *goja.Runtime, loop *eventloop.EventLoop, conn *net.UDPConn, s *pushSocket) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if s.closed.Load() {
			panic(vm.NewTypeError("sendTo: connection closed"))
		}
		payload := snapshotPayload(call.Argument(0))
		host := call.Argument(1).String()
		port := call.Argument(2).String()
		promise, resolve, reject := vm.NewPromise()
		go func() {
			raddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, port))
			if err == nil {
				_, err = conn.WriteToUDP(payload, raddr)
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if err != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("net.udp.sendTo: %w", err)))
				} else {
					_ = resolve(goja.Undefined())
				}
			})
		}()
		return vm.ToValue(promise)
	}
}

// snapshotPayload converts a goja value to bytes ON the loop: a Uint8Array
// exports as []byte, anything else falls back to its UTF-8 string form.
func snapshotPayload(arg goja.Value) []byte {
	if bs, ok := arg.Export().([]byte); ok {
		return bs
	}
	return []byte(arg.String())
}

// udpPortString reads a "port" opt that may arrive as a number or string and
// renders it as the string ResolveUDPAddr expects.
func udpPortString(opts map[string]any, key string) string {
	v, ok := opts[key]
	if !ok {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case int64:
		return fmt.Sprintf("%d", t)
	case int:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%d", int64(t))
	}
	return ""
}

// isClosedConnErr reports whether err is the benign result of closing the
// conn out from under a blocked Read (teardown). Such closes shouldn't surface
// as an onError event — the stream just ends.
func isClosedConnErr(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, net.ErrClosed) || strings.Contains(err.Error(), "use of closed network connection")
}
