package main

import (
	"fmt"
	"net"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"golang.org/x/net/icmp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// icmpServerMembers builds the {listen} map exposed as server.icmp.
func icmpServerMembers(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"listen": func(call goja.FunctionCall) goja.Value {
			return icmpListen(vm, loop, eng, call)
		},
	}
}

// icmpListen implements server.icmp.listen(opts?, (msg, reply) => …). Raw ICMP
// has no ports, so the socket receives all host ICMP traffic and needs root /
// CAP_NET_RAW. Synchronous bind (throws on failure) like the other server.*
// listeners; a read loop invokes the handler ON the loop via LoopCallable with
// msg { bytes, text, address, type, code } and a reply(opts?) that sends an
// ICMP message back to the sender (or opts.to). Returns { address, close() }.
func icmpListen(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, call goja.FunctionCall) goja.Value {
	network := "ip4"
	if opts := call.Argument(0); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
		if m, ok := opts.Export().(map[string]any); ok {
			if optString(m, "network", "ip4") == "ip6" {
				network = "ip6"
			}
		}
	}

	fn, ok := goja.AssertFunction(call.Argument(1))
	if !ok {
		panic(vm.NewTypeError("server.icmp.listen: handler function required"))
	}
	handler := scriptengine.NewLoopCallable(loop, fn)

	listenNet, listenAddr, _, _ := icmpNetwork(network)
	conn, err := icmp.ListenPacket(listenNet, listenAddr)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("server.icmp.listen: %w (raw ICMP needs root / CAP_NET_RAW)", err)))
	}
	if serveReadyWriter != nil {
		fmt.Fprintf(serveReadyWriter, "READY listening on icmp/%s\n", conn.LocalAddr().String())
	}
	release := eng.HoldRun("server.icmp listen " + conn.LocalAddr().String())

	// Read loop (off-loop). Parse each packet, copy the body, capture the
	// sender, then invoke the handler ON the loop via LoopCallable. The message
	// object and reply function are built inside the on-loop buildArgs.
	go func() {
		buf := make([]byte, 64*1024)
		for {
			n, peer, rerr := conn.ReadFrom(buf)
			if n > 0 {
				typ, code, body, perr := parseICMP(network, buf[:n])
				if perr == nil {
					bodyCopy := append([]byte(nil), body...)
					peer := peer // capture for reply
					_, _ = handler.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
						msg := vm.NewObject()
						_ = msg.Set("bytes", vm.ToValue(bodyCopy))
						_ = msg.Set("text", string(bodyCopy))
						if peer != nil {
							_ = msg.Set("address", peer.String())
						}
						_ = msg.Set("type", typ)
						_ = msg.Set("code", code)
						reply := icmpReplyFn(vm, loop, conn, network, peer)
						return []goja.Value{msg, vm.ToValue(reply)}, nil
					})
				}
			}
			if rerr != nil {
				return // listener closed (or read error) → exit
			}
		}
	}()

	handle := vm.NewObject()
	_ = handle.Set("address", fmt.Sprintf("icmp/%s", conn.LocalAddr().String()))
	closeOnce := atomic.Bool{}
	// close() returns Promise<void> for parity with the rest of server.*.
	_ = handle.Set("close", func(goja.FunctionCall) goja.Value {
		promise, resolve, _ := vm.NewPromise()
		if !closeOnce.Swap(true) {
			_ = conn.Close()
			release()
		}
		_ = resolve(goja.Undefined())
		return vm.ToValue(promise)
	})
	return handle
}

// icmpReplyFn builds the reply(opts?) function bound to a received packet's
// sender. opts defaults `to` to the sender's address; otherwise it accepts the
// same options as net.icmp send (Echo or raw body), validated via
// parseICMPSendOpts and built via buildICMPMessage. Returns a Promise<void>.
func icmpReplyFn(vm *goja.Runtime, loop *eventloop.EventLoop, conn *icmp.PacketConn, network string, peer net.Addr) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		m := map[string]any{}
		if optsArg := call.Argument(0); optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
			if mm, ok := optsArg.Export().(map[string]any); ok {
				m = mm
			}
		}
		if _, ok := m["to"]; !ok && peer != nil {
			m["to"] = peer.String()
		}
		o, errMsg := parseICMPSendOpts(m)
		if errMsg != "" {
			panic(vm.NewTypeError(errMsg))
		}
		promise, resolve, reject := vm.NewPromise()
		go func() {
			dst, err := net.ResolveIPAddr(network, o.to)
			if err == nil {
				var b []byte
				b, err = buildICMPMessage(network, o)
				if err == nil {
					_, err = conn.WriteTo(b, dst)
				}
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if err != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("server.icmp reply: %w", err)))
				} else {
					_ = resolve(goja.Undefined())
				}
			})
		}()
		return vm.ToValue(promise)
	}
}
