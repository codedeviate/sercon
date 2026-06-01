package main

import (
	"errors"
	"fmt"
	rand "math/rand/v2"
	"net"
	"runtime"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"golang.org/x/net/ipv4"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// rawNamespace wires net.raw.* — raw IPv4 packet send/receive. `open` returns a
// low-level handle (send + onPacket/onClose/onError + close); `tcp` is a
// one-shot request/response probe. Both need root / CAP_NET_RAW and are
// Linux/macOS only (capture-backed receive).
func rawNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"open": rawOpenFn(vm, loop, eng),
		"tcp":  rawTCPFn(vm, loop, eng),
	}
}

// rawUnsupportedMsg reports the platform-support error, or "" if supported.
func rawUnsupportedMsg() string {
	if runtime.GOOS == "windows" {
		return "net.raw: not supported on windows (raw capture/send unavailable)"
	}
	return ""
}

// rawReceiver bundles the capture source + compiled filter + link type so the
// handle and the probe share one receive setup path.
type rawReceiver struct {
	src  liveSource
	flt  *captureFilter
	link layers.LinkType
}

// openReceiver resolves the egress interface (or uses ifaceOverride), opens a
// live capture there, and compiles filterExpr (may be "").
func openReceiver(ifaceOverride, filterExpr string, dstForRoute net.IP) (rawReceiver, error) {
	iface := ifaceOverride
	if iface == "" {
		var err error
		iface, _, err = egressFor(dstForRoute)
		if err != nil {
			return rawReceiver{}, err
		}
	}
	var flt *captureFilter
	if filterExpr != "" {
		f, err := compileFilter(filterExpr)
		if err != nil {
			return rawReceiver{}, fmt.Errorf("net.raw: filter: %w", err)
		}
		flt = f
	}
	src, err := openLiveCapture(iface, true, 65536)
	if err != nil {
		return rawReceiver{}, err
	}
	return rawReceiver{src: src, flt: flt, link: src.LinkType()}, nil
}

// rawOpenFn implements net.raw.open({ iface?, filter?, readBuffer? }).
func rawOpenFn(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		iface, filterExpr := "", ""
		bufSize := 64
		if opts := call.Argument(0); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			if m, ok := opts.Export().(map[string]any); ok {
				iface = optString(m, "iface", "")
				filterExpr = optString(m, "filter", "")
				if n := optInt(m, "readBuffer", bufSize); n > 0 {
					bufSize = n
				}
			}
		}

		promise, resolve, reject := vm.NewPromise()
		if msg := rawUnsupportedMsg(); msg != "" {
			_ = reject(vm.NewGoError(errors.New(msg)))
			return vm.ToValue(promise)
		}

		openHold := eng.HoldRun("raw open")
		go func() {
			rc, err := openRawSend()
			var rcv rawReceiver
			if err == nil {
				// 8.8.8.8 is just a globally routable sentinel: with no iface
				// override and no single dst, it makes egressFor pick the
				// default-route interface to capture on.
				rcv, err = openReceiver(iface, filterExpr, net.IPv4(8, 8, 8, 8))
				if err != nil {
					_ = rc.Close()
				}
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				openHold()
				if err != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("net.raw.open: %w", err)))
					return
				}
				_ = resolve(buildRawObject(vm, loop, eng, rc, rcv, bufSize))
			})
		}()
		return vm.ToValue(promise)
	}
}

// buildRawObject builds the JS handle for an open raw engine. Runs on the loop.
func buildRawObject(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine, rc *ipv4.RawConn, rcv rawReceiver, bufSize int) *goja.Object {
	s := newPushSocket(vm, loop, bufSize)
	s.release = eng.HoldRun("raw")
	s.teardown = func() error {
		_ = rc.Close()
		return rcv.src.Close()
	}
	s.buildEvent = func(vm *goja.Runtime, in inbound) goja.Value {
		return scriptengine.OrderedToValue(vm, in.decoded)
	}

	go func() {
		defer close(s.recv)
		for {
			data, ci, rerr := rcv.src.ReadPacketData()
			if rerr != nil {
				if !isClosedConnErr(rerr) {
					s.sendInbound(inbound{err: rerr})
				}
				return
			}
			pkt := gopacket.NewPacket(data, rcv.link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
			if rcv.flt != nil && !rcv.flt.match(pkt) {
				continue
			}
			o := decodePacketFrom(pkt, data, rcv.link, ci)
			if !s.sendInbound(inbound{decoded: o}) {
				return
			}
		}
	}()

	obj := vm.NewObject()
	_ = obj.Set("link", rcv.link.String())
	_ = obj.Set("send", rawSendFn(vm, loop, rc, s))
	installSocketCallbacks(obj, s, "onPacket")
	return obj
}

// rawSendFn implements handle.send(specOrBytes): a Promise.
func rawSendFn(vm *goja.Runtime, loop *eventloop.EventLoop, rc *ipv4.RawConn, s *pushSocket) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if s.closed.Load() {
			panic(vm.NewTypeError("send: connection closed"))
		}
		arg := call.Argument(0)

		if raw, ok := arg.Export().([]byte); ok {
			full := append([]byte(nil), raw...)
			return rawSendBytes(vm, loop, rc, full)
		}

		m, ok := arg.Export().(map[string]any)
		if !ok {
			panic(vm.NewTypeError("send: argument must be a spec object or a Uint8Array"))
		}
		spec, errMsg := parseRawSpec(m)
		if errMsg != "" {
			panic(vm.NewTypeError(errMsg))
		}
		return rawSendSpec(vm, loop, rc, spec)
	}
}

// rawSendSpec builds a packet from a structured spec (resolving src via
// egressFor when omitted) and writes it off the loop, resolving { bytesSent }.
func rawSendSpec(vm *goja.Runtime, loop *eventloop.EventLoop, rc *ipv4.RawConn, spec rawSpec) goja.Value {
	promise, resolve, reject := vm.NewPromise()
	go func() {
		var err error
		if spec.src == nil {
			if _, src, e := egressFor(spec.dst); e == nil {
				spec.src = src
			} else {
				err = e
			}
		}
		var n int
		if err == nil {
			var full []byte
			full, err = buildPacket(spec)
			if err == nil {
				n = len(full)
				err = sendRaw(rc, full)
			}
		}
		loop.RunOnLoop(func(vm *goja.Runtime) {
			if err != nil {
				_ = reject(vm.NewGoError(fmt.Errorf("net.raw.send: %w", err)))
				return
			}
			ev := vm.NewObject()
			_ = ev.Set("bytesSent", n)
			_ = resolve(ev)
		})
	}()
	return vm.ToValue(promise)
}

// rawSendBytes writes a caller-supplied full IPv4 packet verbatim (the raw
// escape hatch), resolving { bytesSent }.
func rawSendBytes(vm *goja.Runtime, loop *eventloop.EventLoop, rc *ipv4.RawConn, full []byte) goja.Value {
	promise, resolve, reject := vm.NewPromise()
	go func() {
		err := sendRaw(rc, full)
		loop.RunOnLoop(func(vm *goja.Runtime) {
			if err != nil {
				_ = reject(vm.NewGoError(fmt.Errorf("net.raw.send: %w", err)))
				return
			}
			ev := vm.NewObject()
			_ = ev.Set("bytesSent", len(full))
			_ = resolve(ev)
		})
	}()
	return vm.ToValue(promise)
}

// rawTCPFn implements net.raw.tcp(host, port, opts?).
func rawTCPFn(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		host := call.Argument(0).String()
		port := int(call.Argument(1).ToInteger())

		iface, srcStr := "", ""
		flags := []string{"SYN"}
		ttl := 64
		var seq uint32
		timeout := 2 * time.Second
		var payload []byte
		srcPort := 0
		if opts := call.Argument(2); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			if m, ok := opts.Export().(map[string]any); ok {
				iface = optString(m, "iface", "")
				srcStr = optString(m, "src", "")
				ttl = optInt(m, "ttl", 64)
				seq = uint32(optInt(m, "seq", 0))
				srcPort = optInt(m, "srcPort", 0)
				timeout = optMillis(m, "timeout", timeout)
				if pv, ok := m["payload"]; ok {
					payload = exportPayload(pv)
				}
				if raw, ok := m["flags"].([]any); ok {
					flags = nil
					for _, v := range raw {
						if sfl, ok := v.(string); ok {
							flags = append(flags, sfl)
						}
					}
					// An explicit empty/garbage flags array re-defaults to SYN,
					// matching parseRawSpec — net.raw.tcp without flags is a SYN
					// probe, never a flagless segment.
					if len(flags) == 0 {
						flags = []string{"SYN"}
					}
				}
			}
		}

		promise, resolve, reject := vm.NewPromise()
		if msg := rawUnsupportedMsg(); msg != "" {
			_ = reject(vm.NewGoError(errors.New(msg)))
			return vm.ToValue(promise)
		}
		if host == "" {
			_ = reject(vm.NewGoError(fmt.Errorf("net.raw.tcp: host required")))
			return vm.ToValue(promise)
		}
		if port < 1 || port > 65535 {
			_ = reject(vm.NewGoError(fmt.Errorf("net.raw.tcp: port must be 1..65535, got %d", port)))
			return vm.ToValue(promise)
		}

		openHold := eng.HoldRun("raw.tcp " + host)
		go func() {
			decoded, err := rawTCPProbe(host, port, srcStr, iface, flags, ttl, seq, srcPort, payload, timeout)
			loop.RunOnLoop(func(vm *goja.Runtime) {
				openHold()
				if err != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("net.raw.tcp: %w", err)))
					return
				}
				if decoded == nil {
					_ = resolve(goja.Null())
					return
				}
				_ = resolve(scriptengine.OrderedToValue(vm, decoded))
			})
		}()
		return vm.ToValue(promise)
	}
}

// rawTCPProbe resolves host, opens send socket + filtered capture, sends one
// crafted segment, returns the first matching decoded reply (or nil on timeout).
func rawTCPProbe(host string, port int, srcStr, iface string, flags []string, ttl int, seq uint32, srcPort int, payload []byte, timeout time.Duration) (*scriptengine.Ordered, error) {
	dstAddr, err := net.ResolveIPAddr("ip4", host)
	if err != nil {
		return nil, fmt.Errorf("resolve %q: %w", host, err)
	}
	dst := dstAddr.IP.To4()
	if dst == nil {
		return nil, fmt.Errorf("%q did not resolve to IPv4", host)
	}

	if srcPort == 0 {
		srcPort = 32768 + rand.IntN(28000) // random high port (matches parseRawSpec)
	}
	spec := rawSpec{
		dst: dst, proto: "tcp", ttl: ttl, dstPort: port, srcPort: srcPort,
		flags: flags, seq: seq, window: 65535, payload: payload,
	}
	if srcStr != "" {
		spec.src = net.ParseIP(srcStr)
	}

	filterExpr := fmt.Sprintf("tcp and src host %s and src port %d and dst port %d", dst.String(), port, srcPort)
	rcv, err := openReceiver(iface, filterExpr, dst)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rcv.src.Close() }()

	rc, err := openRawSend()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rc.Close() }()

	if spec.src == nil {
		if _, src, e := egressFor(dst); e == nil {
			spec.src = src
		} else {
			return nil, e
		}
	}
	full, err := buildPacket(spec)
	if err != nil {
		return nil, err
	}

	hit := make(chan *scriptengine.Ordered, 1)
	go func() {
		for {
			data, ci, rerr := rcv.src.ReadPacketData()
			if rerr != nil {
				return
			}
			pkt := gopacket.NewPacket(data, rcv.link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
			if rcv.flt != nil && !rcv.flt.match(pkt) {
				continue
			}
			select {
			case hit <- decodePacketFrom(pkt, data, rcv.link, ci):
			default:
			}
			return
		}
	}()

	if err := sendRaw(rc, full); err != nil {
		return nil, err
	}
	select {
	case o := <-hit:
		return o, nil
	case <-time.After(timeout):
		return nil, nil
	}
}
