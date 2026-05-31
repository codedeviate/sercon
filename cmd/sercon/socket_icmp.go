package main

import (
	"fmt"
	"net"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// ICMP protocol numbers for icmp.ParseMessage.
const (
	protoICMPv4 = 1
	protoICMPv6 = 58
)

// icmpNamespace wires net.icmp.* — raw ICMP send/receive with a
// push/callback read model (onMessage / onClose / onError + send +
// close). Requires raw-socket privileges (root / CAP_NET_RAW).
func icmpNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"open": icmpOpenFn(vm, loop, eng),
	}
}

// icmpNetwork normalizes an opts "network" string to "ip4" or "ip6"
// (default "ip4") and returns the matching icmp.ListenPacket network/address
// pair, the icmp.ParseMessage protocol number, and the default echo-request
// message type.
func icmpNetwork(network string) (listenNet, listenAddr string, proto int, echoType icmp.Type) {
	if network == "ip6" {
		return "ip6:ipv6-icmp", "::", protoICMPv6, ipv6.ICMPTypeEchoRequest
	}
	return "ip4:icmp", "0.0.0.0", protoICMPv4, ipv4.ICMPTypeEcho
}

// marshalEcho builds an ICMP echo (request) message for the given network
// ("ip4" or "ip6") and marshals it to wire bytes. This is the privilege-free
// core exercised by the round-trip unit test.
func marshalEcho(network string, id, seq int, payload []byte) ([]byte, error) {
	var typ icmp.Type
	if network == "ip6" {
		typ = ipv6.ICMPTypeEchoRequest
	} else {
		typ = ipv4.ICMPTypeEcho
	}
	msg := icmp.Message{
		Type: typ,
		Code: 0,
		Body: &icmp.Echo{ID: id, Seq: seq, Data: payload},
	}
	return msg.Marshal(nil)
}

// marshalRaw builds an ICMP message with a caller-supplied raw body for the
// given network ("ip4" or "ip6"), type number, and code, and marshals it to
// wire bytes. The body is emitted verbatim (icmp.RawBody) — used for
// hand-built non-Echo messages (e.g. destination-unreachable). Privilege-free;
// mirrors marshalEcho and is exercised by the round-trip unit test.
func marshalRaw(network string, typeNum, code int, body []byte) ([]byte, error) {
	var typ icmp.Type
	if network == "ip6" {
		typ = ipv6.ICMPType(typeNum)
	} else {
		typ = ipv4.ICMPType(typeNum)
	}
	msg := icmp.Message{
		Type: typ,
		Code: code,
		Body: &icmp.RawBody{Data: body},
	}
	return msg.Marshal(nil)
}

// parseICMP parses a marshalled ICMP message for the given network ("ip4" or
// "ip6"), returning the message type as an int, the code, and the marshalled
// body. Privilege-free.
func parseICMP(network string, b []byte) (typ int, code int, body []byte, err error) {
	proto := protoICMPv4
	if network == "ip6" {
		proto = protoICMPv6
	}
	msg, err := icmp.ParseMessage(proto, b)
	if err != nil {
		return 0, 0, nil, err
	}
	body, err = msg.Body.Marshal(proto)
	if err != nil {
		return 0, 0, nil, err
	}
	var typeNum int
	switch t := msg.Type.(type) {
	case ipv4.ICMPType:
		typeNum = int(t)
	case ipv6.ICMPType:
		typeNum = int(t)
	}
	return typeNum, msg.Code, body, nil
}

// icmpOpenFn implements net.icmp.open(opts). It returns a Promise that
// resolves to a handle object backed by a raw ICMP socket. opts:
//
//	{ network?: "ip4" | "ip6" (default "ip4"), readBuffer? }
//
// The socket is opened off-loop in a goroutine; on success the handle is built
// back ON the loop via RunOnLoop (the same hand-rolled pattern as
// udpOpenFn / server_ws.go). If icmp.ListenPacket fails — typically because
// raw sockets need root / CAP_NET_RAW — the Promise rejects with an error
// naming that requirement. readBuffer is the inbound channel capacity
// (default 64).
func icmpOpenFn(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		// Parse opts on the loop (goja values aren't safe off-loop).
		network := "ip4"
		bufSize := 64
		if opts := call.Argument(0); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			if m, ok := opts.Export().(map[string]any); ok {
				if n := optString(m, "network", "ip4"); n == "ip6" {
					network = "ip6"
				}
				if n := optInt(m, "readBuffer", bufSize); n > 0 {
					bufSize = n
				}
			}
		}

		listenNet, listenAddr, _, _ := icmpNetwork(network)

		promise, resolve, reject := vm.NewPromise()

		// Keep the loop alive across the off-loop open: vm.NewPromise() + a
		// goroutine does not bump the loop's jobCount.
		openHold := eng.HoldRun("icmp open " + network)

		go func() {
			conn, err := icmp.ListenPacket(listenNet, listenAddr)
			if err != nil {
				loop.RunOnLoop(func(vm *goja.Runtime) {
					openHold()
					_ = reject(vm.NewGoError(fmt.Errorf(
						"net.icmp.open: %w (raw ICMP needs root / CAP_NET_RAW)", err)))
				})
				return
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				openHold()
				obj := buildICMPObject(vm, loop, eng, conn, network, bufSize)
				_ = resolve(obj)
			})
		}()

		return vm.ToValue(promise)
	}
}

// buildICMPObject constructs the JS handle for a raw ICMP socket. It MUST run
// on the loop (it builds goja values). It starts the reader goroutine,
// registers the HoldRun sentinel + teardown, wires send, and installs the
// shared onMessage/onClose/onError/close callbacks.
func buildICMPObject(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, conn *icmp.PacketConn, network string, bufSize int) *goja.Object {
	_, _, proto, _ := icmpNetwork(network)
	s := newPushSocket(vm, loop, bufSize)
	s.release = eng.HoldRun("icmp " + network)
	s.teardown = func() error { return conn.Close() }

	// Reader goroutine: one packet per ReadFrom == one inbound event. Parse the
	// message and attach { address, type, code } meta. On a benign close
	// (teardown closed the conn) end silently; otherwise forward the error.
	go func() {
		defer close(s.recv)
		buf := make([]byte, 64*1024)
		for {
			n, peer, err := conn.ReadFrom(buf)
			if n > 0 {
				typ, code, body, perr := parseICMP(network, buf[:n])
				if perr == nil {
					meta := map[string]any{"type": typ, "code": code}
					if peer != nil {
						meta["address"] = peer.String()
					}
					if !s.sendInbound(inbound{payload: append([]byte(nil), body...), meta: meta}) {
						return
					}
				}
			}
			if err != nil {
				if !isClosedConnErr(err) {
					s.sendInbound(inbound{err: err})
				}
				return
			}
		}
	}()

	obj := vm.NewObject()
	_ = obj.Set("network", network)
	_ = obj.Set("local", conn.LocalAddr().String())
	_ = obj.Set("send", icmpSendFn(vm, loop, conn, s, network, proto))
	installSocketCallbacks(obj, s, "onMessage")
	return obj
}

// icmpSendOpts is the parsed, validated state of a handle.send(opts) call.
type icmpSendOpts struct {
	to      string
	code    int
	hasType bool
	typeNum int
	id, seq int
	payload []byte
	hasBody bool
	body    []byte
}

// parseICMPSendOpts parses and validates a send opts map. On a validation
// failure it returns a non-empty message suitable for vm.NewTypeError; on
// success it returns the parsed opts and "". It is privilege-free and
// independent of the socket so the validation rules can be unit-tested
// directly (mirroring marshalEcho / parseICMP). Detection of id/seq/payload is
// by key PRESENCE, not value, so { body, id: 0 } is still rejected. A nil map
// (no opts passed) yields the "to required" error.
func parseICMPSendOpts(m map[string]any) (icmpSendOpts, string) {
	var o icmpSendOpts
	o.to = optString(m, "to", "")
	o.code = optInt(m, "code", 0)

	var hasID, hasSeq, hasPayload bool
	if _, ok := m["id"]; ok {
		hasID = true
		o.id = optInt(m, "id", 0)
	}
	if _, ok := m["seq"]; ok {
		hasSeq = true
		o.seq = optInt(m, "seq", 0)
	}
	if _, ok := m["type"]; ok {
		o.hasType = true
		o.typeNum = optInt(m, "type", 0)
	}
	if pv, ok := m["payload"]; ok {
		hasPayload = true
		o.payload = exportPayload(pv)
	}
	if bv, ok := m["body"]; ok {
		o.hasBody = true
		o.body = exportPayload(bv)
	}

	if o.to == "" {
		return o, "send: opts.to (destination address) required"
	}
	if o.hasBody {
		if !o.hasType {
			return o, "send: opts.type is required when sending a raw body"
		}
		if hasID || hasSeq || hasPayload {
			return o, "send: body is mutually exclusive with id/seq/payload"
		}
	}
	return o, ""
}

// icmpSendFn implements handle.send(opts): a Promise that builds and writes an
// ICMP message. opts has two modes (see parseICMPSendOpts):
//
//	Echo mode: { to, type?, code?, id?, seq?, payload? } — type defaults to the
//	  network's echo request.
//	Raw mode:  { to, type, code?, body } — body is marshalled verbatim
//	  (marshalRaw / icmp.RawBody); type is required and mutually exclusive with
//	  id/seq/payload.
//
// The opts are parsed + validated ON the loop (goja values aren't safe
// off-loop); the destination is resolved and the packet built + written OFF
// the loop, resolving/rejecting back on the loop.
func icmpSendFn(vm *goja.Runtime, loop *eventloop.EventLoop, conn *icmp.PacketConn, s *pushSocket, network string, proto int) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if s.closed.Load() {
			panic(vm.NewTypeError("send: connection closed"))
		}

		// Parse + validate opts on the loop (goja values aren't safe off-loop).
		var m map[string]any
		if optsArg := call.Argument(0); optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
			if mm, ok := optsArg.Export().(map[string]any); ok {
				m = mm
			}
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
				if o.hasBody {
					b, err = marshalRaw(network, o.typeNum, o.code, o.body)
				} else {
					var typ icmp.Type
					if o.hasType {
						if network == "ip6" {
							typ = ipv6.ICMPType(o.typeNum)
						} else {
							typ = ipv4.ICMPType(o.typeNum)
						}
					} else if network == "ip6" {
						typ = ipv6.ICMPTypeEchoRequest
					} else {
						typ = ipv4.ICMPTypeEcho
					}
					msg := icmp.Message{
						Type: typ,
						Code: o.code,
						Body: &icmp.Echo{ID: o.id, Seq: o.seq, Data: o.payload},
					}
					b, err = msg.Marshal(nil)
				}
				if err == nil {
					_, err = conn.WriteTo(b, dst)
				}
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if err != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("net.icmp.send: %w", err)))
				} else {
					_ = resolve(goja.Undefined())
				}
			})
		}()
		return vm.ToValue(promise)
	}
}

// exportPayload converts an already-exported goja value (a map entry, not a
// goja.Value) to bytes: a []byte (Uint8Array) passes through, a string is
// converted to its bytes; any other type (e.g. a number) yields nil.
func exportPayload(v any) []byte {
	switch t := v.(type) {
	case []byte:
		return t
	case string:
		return []byte(t)
	}
	return nil
}
