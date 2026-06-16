package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// captureNamespace wires net.capture.* — packet capture file I/O. openFile
// reads a .pcap/.pcapng file and dispatches each decoded packet to a JS
// handler; toFile writes raw frames into a .pcap file. (interfaces() and the
// live open() backends land in later tasks.) The factory captures eng so the
// long-lived openFile read keeps the loop alive via HoldRun.
func captureNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) map[string]any {
	return map[string]any{
		"interfaces": captureInterfacesFn(vm),
		"routes":     captureRoutesFn(vm),
		"open":       captureOpenFn(vm, loop, eng),
		"openFile":   captureOpenFileFn(vm, loop, eng),
		"toFile":     captureToFileFn(vm, loop, eng),
	}
}

// liveSource is a platform-specific live packet reader. The per-OS backends
// (capture_linux.go / capture_darwin.go / capture_other.go) define
// openLiveCapture which returns a value satisfying this interface.
type liveSource interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	LinkType() layers.LinkType
	Close() error
}

// openLiveCapture opens a live capture handle on iface in promiscuous mode (if
// promisc) with the given snaplen. It is implemented per-OS via build tags:
// capture_linux.go (AF_PACKET), capture_darwin.go (BPF), and capture_other.go
// (the !linux && !darwin stub that returns an "unsupported" error). Permission
// failures are wrapped with a platform-specific privilege hint by the backend.

// captureOpenFn implements net.capture.open({iface, promisc?, snaplen?},
// onPacket). It returns a Promise that resolves to a live-capture handle
// { iface, link, close() }. The privileged open() happens OFF the loop in a
// goroutine (it may block / needs root); on success the handle is built back
// ON the loop and a reader goroutine starts dispatching decoded packets to
// onPacket via a LoopCallable. Mirrors socket_tcp.go's hand-rolled promise +
// dial-time HoldRun handoff: a dial-time hold keeps the loop alive across the
// open, handed off to the capture's own hold on success.
//
// opts: { iface (required), promisc? (default true), snaplen? (default
// 262144) }.
func captureOpenFn(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		// Parse opts on the loop (goja values aren't safe off-loop).
		iface := ""
		promisc := true
		snaplen := 262144
		filterExpr := ""
		if opts := call.Argument(0); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			if m, ok := opts.Export().(map[string]any); ok {
				iface = optString(m, "iface", "")
				promisc = optBool(m, "promisc", promisc)
				snaplen = optInt(m, "snaplen", snaplen)
				filterExpr = optString(m, "filter", "")
			}
		}

		fn, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(vm.NewTypeError("net.capture.open: second argument must be a function"))
		}
		handler := scriptengine.NewLoopCallable(loop, fn)

		promise, resolve, reject := vm.NewPromise()
		if iface == "" {
			// Reject asynchronously so the returned value is always a Promise.
			_ = reject(vm.NewGoError(errors.New("net.capture.open: opts.iface is required")))
			return vm.ToValue(promise)
		}

		// Compile the filter up front, on the loop, so a malformed expression
		// rejects before any capture is opened (mirrors the missing-iface path).
		var flt *captureFilter
		if filterExpr != "" {
			f, err := compileFilter(filterExpr)
			if err != nil {
				_ = reject(vm.NewGoError(fmt.Errorf("net.capture.open: filter: %w", err)))
				return vm.ToValue(promise)
			}
			flt = f
		}

		// Hold the loop alive across the off-loop open: vm.NewPromise() + a
		// goroutine does not bump the loop's jobCount, so without this the loop
		// could exit before the open resolves. Handed off to the capture's own
		// hold on success; released on the loop on failure.
		dialHold := eng.HoldRun("capture open " + iface)

		go func() {
			src, err := openLiveCapture(iface, promisc, snaplen)
			if err != nil {
				loop.RunOnLoop(func(vm *goja.Runtime) {
					dialHold()
					_ = reject(vm.NewGoError(err))
				})
				return
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				// Take the capture's own hold, then release the dial hold.
				captureHold := eng.HoldRun("capture " + iface)
				dialHold()

				link := src.LinkType()
				var holdReleased atomic.Bool
				releaseHold := func() {
					if !holdReleased.Swap(true) {
						captureHold()
					}
				}

				// Reader goroutine: read + decode off-loop, dispatch on-loop.
				// A closed source makes ReadPacketData return an error, which
				// ends the loop; release the capture hold exactly once on exit.
				go func() {
					defer releaseHold()
					for {
						data, ci, rerr := src.ReadPacketData()
						if rerr != nil {
							return
						}
						pkt := gopacket.NewPacket(data, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
						if flt != nil && !flt.match(pkt) {
							continue
						}
						o := decodePacketFrom(pkt, data, link, ci)
						if _, cerr := handler.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
							return []goja.Value{scriptengine.OrderedToValue(vm, o)}, nil
						}); cerr != nil {
							return
						}
					}
				}()

				var closed atomic.Bool
				obj := vm.NewObject()
				_ = obj.Set("iface", iface)
				_ = obj.Set("link", link.String())
				_ = obj.Set("close", func(goja.FunctionCall) goja.Value {
					promise, resolve, reject := vm.NewPromise()
					if closed.Swap(true) {
						_ = resolve(goja.Undefined())
						return vm.ToValue(promise)
					}
					// Close unblocks ReadPacketData so the reader exits and
					// releases its hold; also release here in case the reader
					// already exited on an error.
					cerr := src.Close()
					releaseHold()
					if cerr != nil {
						_ = reject(vm.NewGoError(fmt.Errorf("net.capture.open.close: %w", cerr)))
					} else {
						_ = resolve(goja.Undefined())
					}
					return vm.ToValue(promise)
				})
				_ = resolve(obj)
			})
		}()

		return vm.ToValue(promise)
	}
}

// captureInterfacesFn implements net.capture.interfaces(): a synchronous list
// of the host's network interfaces, each { name, addresses, up, loopback }.
// Pure-Go (net.Interfaces); built on the loop so the ordered objects convert
// directly. Throws on enumeration failure.
func captureInterfacesFn(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(goja.FunctionCall) goja.Value {
		ifaces, err := net.Interfaces()
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("net.capture.interfaces: %w", err)))
		}
		out := make([]*scriptengine.Ordered, 0, len(ifaces))
		for _, iface := range ifaces {
			addrs := []any{}
			if as, err := iface.Addrs(); err == nil {
				for _, a := range as {
					addrs = append(addrs, a.String())
				}
			}
			out = append(out, scriptengine.NewOrdered().
				Set("name", iface.Name).
				Set("addresses", addrs).
				Set("up", iface.Flags&net.FlagUp != 0).
				Set("loopback", iface.Flags&net.FlagLoopback != 0))
		}
		return scriptengine.OrderedToValue(vm, out)
	}
}

// captureToFileFn implements net.capture.toFile(path, opts?). It opens/creates
// the file, writes the pcap global header, and returns a handle:
//
//   - write(bytes, opts?): snapshot the frame ON the loop and append it to the
//     file synchronously (local write — no goroutine needed). opts.ts (ms)
//     overrides the timestamp. Returns undefined.
//   - close(): flush + close the file once; returns Promise<void>.
//
// opts: { snaplen? (default 262144), linkType? (number, default Ethernet) }.
func captureToFileFn(vm *goja.Runtime, loop *eventloop.EventLoop, _ *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		path := call.Argument(0).String()

		snaplen := 262144
		linkType := layers.LinkTypeEthernet
		if opts := call.Argument(1); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			if m, ok := opts.Export().(map[string]any); ok {
				snaplen = optInt(m, "snaplen", snaplen)
				linkType = layers.LinkType(optInt(m, "linkType", int(linkType)))
			}
		}

		f, err := os.Create(path)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("net.capture.toFile: %w", err)))
		}
		w := pcapgo.NewWriter(f)
		if err := w.WriteFileHeader(uint32(snaplen), linkType); err != nil {
			_ = f.Close()
			panic(vm.NewGoError(fmt.Errorf("net.capture.toFile: %w", err)))
		}

		var closed atomic.Bool
		obj := vm.NewObject()
		_ = obj.Set("write", func(call goja.FunctionCall) goja.Value {
			if closed.Load() {
				panic(vm.NewTypeError("write: capture file closed"))
			}
			payload := snapshotPayload(call.Argument(0))
			ts := time.Now()
			if opts := call.Argument(1); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
				if m, ok := opts.Export().(map[string]any); ok {
					if v, ok := m["ts"]; ok {
						if ms, ok := toInt64(v); ok {
							ts = time.UnixMilli(ms)
						}
					}
				}
			}
			n := len(payload)
			ci := gopacket.CaptureInfo{Timestamp: ts, CaptureLength: n, Length: n}
			if err := w.WritePacket(ci, payload); err != nil {
				panic(vm.NewGoError(fmt.Errorf("net.capture.toFile.write: %w", err)))
			}
			return goja.Undefined()
		})
		_ = obj.Set("close", func(goja.FunctionCall) goja.Value {
			promise, resolve, reject := vm.NewPromise()
			if closed.Swap(true) {
				_ = resolve(goja.Undefined())
				return vm.ToValue(promise)
			}
			if err := f.Close(); err != nil {
				_ = reject(vm.NewGoError(fmt.Errorf("net.capture.toFile.close: %w", err)))
			} else {
				_ = resolve(goja.Undefined())
			}
			return vm.ToValue(promise)
		})
		return obj
	}
}

// captureOpenFileFn implements net.capture.openFile(path, onPacket). It returns
// a Promise that resolves (to undefined) at EOF after dispatching every packet
// to onPacket. The file is opened and parsed OFF the loop in a goroutine; each
// decoded packet is dispatched via a LoopCallable (decode runs off-loop, the
// goja conversion runs on the loop). A HoldRun keeps the loop alive across the
// read, released on completion or error. Mirrors socket_tcp.go's hand-rolled
// promise + dial-time hold handoff.
func captureOpenFileFn(vm *goja.Runtime, loop *eventloop.EventLoop, eng *scriptengine.Engine) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		path := call.Argument(0).String()

		fn, ok := goja.AssertFunction(call.Argument(1))
		if !ok {
			panic(vm.NewTypeError("net.capture.openFile: second argument must be a function"))
		}
		handler := scriptengine.NewLoopCallable(loop, fn)

		// Optional trailing opts arg — openFile(path, onPacket, { filter? }).
		// The 2-arg form (no opts) stays backward-compatible: flt is nil and
		// every packet passes.
		filterExpr := ""
		if opts := call.Argument(2); opts != nil && !goja.IsUndefined(opts) && !goja.IsNull(opts) {
			if m, ok := opts.Export().(map[string]any); ok {
				filterExpr = optString(m, "filter", "")
			}
		}

		promise, resolve, reject := vm.NewPromise()

		// Compile the filter up front, on the loop, so a malformed expression
		// rejects before the file is read.
		var flt *captureFilter
		if filterExpr != "" {
			f, err := compileFilter(filterExpr)
			if err != nil {
				_ = reject(vm.NewGoError(fmt.Errorf("net.capture.openFile: filter: %w", err)))
				return vm.ToValue(promise)
			}
			flt = f
		}

		// Hold the loop alive across the off-loop read: vm.NewPromise() + a
		// goroutine does not bump the loop's jobCount, so without this the loop
		// could exit before the read completes. Released on the loop on settle.
		hold := eng.HoldRun("capture file " + path)

		go func() {
			r, closeFn, err := openCaptureReader(path)
			if err != nil {
				loop.RunOnLoop(func(vm *goja.Runtime) {
					hold()
					_ = reject(vm.NewGoError(fmt.Errorf("net.capture.openFile: %w", err)))
				})
				return
			}
			defer closeFn()

			link := r.LinkType()
			for {
				data, ci, rerr := r.ReadPacketData()
				if rerr == io.EOF {
					break
				}
				if rerr != nil {
					loop.RunOnLoop(func(vm *goja.Runtime) {
						hold()
						_ = reject(vm.NewGoError(fmt.Errorf("net.capture.openFile: %w", rerr)))
					})
					return
				}
				pkt := gopacket.NewPacket(data, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
				if flt != nil && !flt.match(pkt) {
					continue
				}
				o := decodePacketFrom(pkt, data, link, ci)
				if _, cerr := handler.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
					return []goja.Value{scriptengine.OrderedToValue(vm, o)}, nil
				}); cerr != nil {
					loop.RunOnLoop(func(vm *goja.Runtime) {
						hold()
						_ = reject(vm.NewGoError(fmt.Errorf("net.capture.openFile: %w", cerr)))
					})
					return
				}
			}

			loop.RunOnLoop(func(vm *goja.Runtime) {
				hold()
				_ = resolve(goja.Undefined())
			})
		}()

		return vm.ToValue(promise)
	}
}

// captureReader is the common read surface shared by pcap and pcapng readers.
type captureReader interface {
	ReadPacketData() ([]byte, gopacket.CaptureInfo, error)
	LinkType() layers.LinkType
}

// openCaptureReader opens path, sniffs the first 4 magic bytes to choose a
// .pcap vs .pcapng reader, and returns the reader plus a close function for the
// underlying file. The peeked bytes are spliced back via io.MultiReader so the
// reader sees the whole file.
//
// pcap magics: 0xA1B2C3D4 / 0xD4C3B2A1 (us) and 0xA1B23C4D / 0x4D3CB2A1 (ns),
// in either byte order. pcapng magic: 0x0A0D0D0A (block-type of the SHB).
func openCaptureReader(path string) (captureReader, func(), error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, func() {}, err
	}
	closeFn := func() { _ = f.Close() }

	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		closeFn()
		return nil, func() {}, err
	}
	full := io.MultiReader(bytes.NewReader(magic[:]), f)

	if isPcapNgMagic(magic) {
		r, err := pcapgo.NewNgReader(full, pcapgo.DefaultNgReaderOptions)
		if err != nil {
			closeFn()
			return nil, func() {}, err
		}
		return r, closeFn, nil
	}

	r, err := pcapgo.NewReader(full)
	if err != nil {
		closeFn()
		return nil, func() {}, err
	}
	return r, closeFn, nil
}

// isPcapNgMagic reports whether the first 4 bytes are the pcapng SHB block-type
// (0x0A0D0D0A), in either byte order.
func isPcapNgMagic(magic [4]byte) bool {
	be := binary.BigEndian.Uint32(magic[:])
	le := binary.LittleEndian.Uint32(magic[:])
	return be == 0x0A0D0D0A || le == 0x0A0D0D0A
}

// toInt64 coerces a JS-exported number (int64/int/float64) to int64.
func toInt64(v any) (int64, bool) {
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case float64:
		return int64(t), true
	}
	return 0, false
}

// decodePacket decodes a raw frame into an insertion-ordered packet object.
//
// It runs OFF the event loop, so it builds only plain Go data into the
// Ordered (no goja): primitives, []byte (rendered as Uint8Array later by
// OrderedToValue), and nested *Ordered. Layer keys (eth/ip/tcp/udp/icmp/
// payload) are present only when that layer decodes; the always-present keys
// (ts/length/captureLength/link/bytes) make a truncated or unrecognised frame
// still yield a usable object. Every extraction is guarded by a nil/type
// check so malformed input never panics.
func decodePacket(data []byte, link layers.LinkType, ci gopacket.CaptureInfo) *scriptengine.Ordered {
	pkt := gopacket.NewPacket(data, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})
	return decodePacketFrom(pkt, data, link, ci)
}

// decodePacketFrom decodes an already-parsed packet into an insertion-ordered
// object. It is the body of decodePacket split out so capture readers that
// also evaluate a filter can parse the frame once (gopacket.NewPacket) and
// share the resulting packet between match() and the decode. data is still used
// for the raw "bytes" key.
func decodePacketFrom(pkt gopacket.Packet, data []byte, link layers.LinkType, ci gopacket.CaptureInfo) *scriptengine.Ordered {
	o := scriptengine.NewOrdered()
	o.Set("ts", ci.Timestamp.UnixMilli()) // 0 if zero-value; callers may set Timestamp
	o.Set("length", ci.Length)
	o.Set("captureLength", ci.CaptureLength)
	o.Set("link", link.String())

	if l, _ := pkt.Layer(layers.LayerTypeEthernet).(*layers.Ethernet); l != nil {
		o.Set("eth", scriptengine.NewOrdered().
			Set("src", l.SrcMAC.String()).
			Set("dst", l.DstMAC.String()).
			Set("type", l.EthernetType.String()))
	}

	if l, _ := pkt.Layer(layers.LayerTypeDot1Q).(*layers.Dot1Q); l != nil {
		o.Set("vlan", scriptengine.NewOrdered().
			Set("id", int(l.VLANIdentifier)).
			Set("priority", int(l.Priority)).
			Set("drop", l.DropEligible).
			Set("type", l.Type.String()))
	}

	if l, _ := pkt.Layer(layers.LayerTypeARP).(*layers.ARP); l != nil {
		op := fmt.Sprintf("%d", l.Operation)
		switch l.Operation {
		case layers.ARPRequest:
			op = "request"
		case layers.ARPReply:
			op = "reply"
		}
		o.Set("arp", scriptengine.NewOrdered().
			Set("operation", op).
			Set("senderMac", net.HardwareAddr(l.SourceHwAddress).String()).
			Set("senderIp", net.IP(l.SourceProtAddress).String()).
			Set("targetMac", net.HardwareAddr(l.DstHwAddress).String()).
			Set("targetIp", net.IP(l.DstProtAddress).String()))
	}

	if l, _ := pkt.Layer(layers.LayerTypeIPv4).(*layers.IPv4); l != nil {
		o.Set("ip", scriptengine.NewOrdered().
			Set("version", 4).
			Set("src", l.SrcIP.String()).
			Set("dst", l.DstIP.String()).
			Set("protocol", l.Protocol.String()).
			Set("ttl", int(l.TTL)))
	} else if l, _ := pkt.Layer(layers.LayerTypeIPv6).(*layers.IPv6); l != nil {
		o.Set("ip", scriptengine.NewOrdered().
			Set("version", 6).
			Set("src", l.SrcIP.String()).
			Set("dst", l.DstIP.String()).
			Set("protocol", l.NextHeader.String()).
			Set("ttl", int(l.HopLimit)))
	}

	if l, _ := pkt.Layer(layers.LayerTypeTCP).(*layers.TCP); l != nil {
		tcp := scriptengine.NewOrdered().
			Set("srcPort", int(l.SrcPort)).
			Set("dstPort", int(l.DstPort)).
			Set("seq", l.Seq).
			Set("ack", l.Ack).
			Set("window", int(l.Window)).
			Set("checksum", int(l.Checksum)).
			Set("flags", scriptengine.NewOrdered().
				Set("syn", l.SYN).
				Set("ack", l.ACK).
				Set("fin", l.FIN).
				Set("rst", l.RST).
				Set("psh", l.PSH).
				Set("urg", l.URG))
		opts := scriptengine.NewOrdered()
		for _, opt := range l.Options {
			switch opt.OptionType {
			case layers.TCPOptionKindMSS:
				if len(opt.OptionData) == 2 {
					opts.Set("mss", int(binary.BigEndian.Uint16(opt.OptionData)))
				}
			case layers.TCPOptionKindWindowScale:
				if len(opt.OptionData) == 1 {
					opts.Set("windowScale", int(opt.OptionData[0]))
				}
			case layers.TCPOptionKindSACKPermitted:
				opts.Set("sackPermitted", true)
			case layers.TCPOptionKindTimestamps:
				if len(opt.OptionData) == 8 {
					opts.Set("timestamps", scriptengine.NewOrdered().
						Set("val", binary.BigEndian.Uint32(opt.OptionData[0:4])).
						Set("ecr", binary.BigEndian.Uint32(opt.OptionData[4:8])))
				}
			}
		}
		if opts.Len() > 0 {
			tcp.Set("options", opts)
		}
		o.Set("tcp", tcp)
	}

	if l, _ := pkt.Layer(layers.LayerTypeUDP).(*layers.UDP); l != nil {
		o.Set("udp", scriptengine.NewOrdered().
			Set("srcPort", int(l.SrcPort)).
			Set("dstPort", int(l.DstPort)).
			Set("length", int(l.Length)))
	}

	if l, _ := pkt.Layer(layers.LayerTypeICMPv4).(*layers.ICMPv4); l != nil {
		o.Set("icmp", scriptengine.NewOrdered().
			Set("type", int(l.TypeCode.Type())).
			Set("code", int(l.TypeCode.Code())))
	} else if l, _ := pkt.Layer(layers.LayerTypeICMPv6).(*layers.ICMPv6); l != nil {
		o.Set("icmp", scriptengine.NewOrdered().
			Set("type", int(l.TypeCode.Type())).
			Set("code", int(l.TypeCode.Code())))
	}

	if l, _ := pkt.Layer(layers.LayerTypeDNS).(*layers.DNS); l != nil {
		dns := scriptengine.NewOrdered().
			Set("id", int(l.ID)).
			Set("qr", l.QR).
			Set("opcode", l.OpCode.String()).
			Set("rcode", l.ResponseCode.String())
		qs := make([]any, 0, len(l.Questions))
		for _, q := range l.Questions {
			qs = append(qs, scriptengine.NewOrdered().
				Set("name", string(q.Name)).
				Set("type", q.Type.String()))
		}
		dns.Set("questions", qs)
		ans := make([]any, 0, len(l.Answers))
		for _, rr := range l.Answers {
			ans = append(ans, scriptengine.NewOrdered().
				Set("name", string(rr.Name)).
				Set("type", rr.Type.String()).
				Set("data", dnsAnswerData(rr)))
		}
		dns.Set("answers", ans)
		o.Set("dns", dns)
	}

	if app := pkt.ApplicationLayer(); app != nil && len(app.Payload()) > 0 {
		o.Set("payload", app.Payload()) // []byte → Uint8Array
	}

	o.Set("bytes", data)
	return o
}

// dnsAnswerData renders a DNS resource record's value as a string, switching
// on the record type. Unknown / unparsed types fall back to a hex dump of the
// raw record data so nothing is silently dropped. All field reads are safe on
// the zero value, so a malformed record yields "" rather than panicking.
func dnsAnswerData(rr layers.DNSResourceRecord) string {
	switch rr.Type {
	case layers.DNSTypeA, layers.DNSTypeAAAA:
		if rr.IP != nil {
			return rr.IP.String()
		}
	case layers.DNSTypeCNAME:
		return string(rr.CNAME)
	case layers.DNSTypeNS:
		return string(rr.NS)
	case layers.DNSTypePTR:
		return string(rr.PTR)
	case layers.DNSTypeMX:
		return string(rr.MX.Name)
	case layers.DNSTypeSOA:
		return string(rr.SOA.MName)
	case layers.DNSTypeTXT:
		parts := make([]string, 0, len(rr.TXTs))
		for _, t := range rr.TXTs {
			parts = append(parts, string(t))
		}
		return strings.Join(parts, " ")
	}
	if len(rr.Data) > 0 {
		return fmt.Sprintf("%x", rr.Data)
	}
	return ""
}
