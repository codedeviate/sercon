package main

import (
	"fmt"
	"net"
	"strings"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// rawSpec is the parsed, validated description of one packet to send. src may
// be nil at parse time (filled from the egress interface before sending).
// Wire-bounded fields (ttl, ipID, window, ports) are truncated to their wire
// widths by the serializer and are expected to be range-validated by the caller
// (parseRawSpec) before a rawSpec is constructed.
type rawSpec struct {
	dst      net.IP
	src      net.IP
	proto    string // "tcp" (default) | "udp" | "ip"
	protocol int    // IP protocol number, used when proto == "ip"
	ttl      int
	ipID     int
	dstPort  int
	srcPort  int
	flags    []string // TCP flag names (case-insensitive)
	seq      uint32
	ack      uint32
	window   int
	payload  []byte
}

// tcpFlagSetters maps a normalized (upper-case) TCP flag name to the field it
// sets on a layers.TCP. NS/ECE/CWR are included for completeness.
var tcpFlagSetters = map[string]func(*layers.TCP){
	"SYN": func(t *layers.TCP) { t.SYN = true },
	"ACK": func(t *layers.TCP) { t.ACK = true },
	"RST": func(t *layers.TCP) { t.RST = true },
	"FIN": func(t *layers.TCP) { t.FIN = true },
	"PSH": func(t *layers.TCP) { t.PSH = true },
	"URG": func(t *layers.TCP) { t.URG = true },
	"ECE": func(t *layers.TCP) { t.ECE = true },
	"CWR": func(t *layers.TCP) { t.CWR = true },
	"NS":  func(t *layers.TCP) { t.NS = true },
}

// applyTCPFlags sets the named flags on tcp. Unknown names return an error so a
// typo surfaces rather than silently sending a flagless segment.
func applyTCPFlags(tcp *layers.TCP, flags []string) error {
	for _, f := range flags {
		set, ok := tcpFlagSetters[strings.ToUpper(strings.TrimSpace(f))]
		if !ok {
			return fmt.Errorf("unknown TCP flag %q", f)
		}
		set(tcp)
	}
	return nil
}

// buildPacket serializes a full IPv4 packet (IP header + L4) to wire bytes,
// computing IP and L4 checksums and fixing lengths. proto selects the L4:
// "tcp" (default), "udp", or "ip" (raw payload under spec.protocol).
func buildPacket(spec rawSpec) ([]byte, error) {
	ip := &layers.IPv4{
		Version: 4,
		TTL:     uint8(spec.ttl),
		Id:      uint16(spec.ipID),
		SrcIP:   spec.src.To4(),
		DstIP:   spec.dst.To4(),
	}
	if ip.SrcIP == nil || ip.DstIP == nil {
		return nil, fmt.Errorf("buildPacket: src/dst must be IPv4 (src=%v dst=%v)", spec.src, spec.dst)
	}
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}

	switch spec.proto {
	case "udp":
		ip.Protocol = layers.IPProtocolUDP
		udp := &layers.UDP{SrcPort: layers.UDPPort(spec.srcPort), DstPort: layers.UDPPort(spec.dstPort)}
		if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
			return nil, err
		}
		if err := gopacket.SerializeLayers(buf, opts, ip, udp, gopacket.Payload(spec.payload)); err != nil {
			return nil, err
		}
	case "ip":
		ip.Protocol = layers.IPProtocol(spec.protocol)
		if err := gopacket.SerializeLayers(buf, opts, ip, gopacket.Payload(spec.payload)); err != nil {
			return nil, err
		}
	default: // "tcp"
		ip.Protocol = layers.IPProtocolTCP
		tcp := &layers.TCP{
			SrcPort: layers.TCPPort(spec.srcPort),
			DstPort: layers.TCPPort(spec.dstPort),
			Seq:     spec.seq,
			Ack:     spec.ack,
			Window:  uint16(spec.window),
		}
		if err := applyTCPFlags(tcp, spec.flags); err != nil {
			return nil, err
		}
		if err := tcp.SetNetworkLayerForChecksum(ip); err != nil {
			return nil, err
		}
		if err := gopacket.SerializeLayers(buf, opts, ip, tcp, gopacket.Payload(spec.payload)); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}
