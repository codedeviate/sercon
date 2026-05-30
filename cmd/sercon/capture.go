package main

import (
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

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
	o := scriptengine.NewOrdered()
	o.Set("ts", ci.Timestamp.UnixMilli()) // 0 if zero-value; callers may set Timestamp
	o.Set("length", ci.Length)
	o.Set("captureLength", ci.CaptureLength)
	o.Set("link", link.String())

	pkt := gopacket.NewPacket(data, link, gopacket.DecodeOptions{Lazy: true, NoCopy: true})

	if l, _ := pkt.Layer(layers.LayerTypeEthernet).(*layers.Ethernet); l != nil {
		o.Set("eth", scriptengine.NewOrdered().
			Set("src", l.SrcMAC.String()).
			Set("dst", l.DstMAC.String()).
			Set("type", l.EthernetType.String()))
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
		o.Set("tcp", scriptengine.NewOrdered().
			Set("srcPort", int(l.SrcPort)).
			Set("dstPort", int(l.DstPort)).
			Set("seq", l.Seq).
			Set("ack", l.Ack).
			Set("flags", scriptengine.NewOrdered().
				Set("syn", l.SYN).
				Set("ack", l.ACK).
				Set("fin", l.FIN).
				Set("rst", l.RST).
				Set("psh", l.PSH).
				Set("urg", l.URG)))
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

	if app := pkt.ApplicationLayer(); app != nil && len(app.Payload()) > 0 {
		o.Set("payload", app.Payload()) // []byte → Uint8Array
	}

	o.Set("bytes", data)
	return o
}
