package main

import (
	"fmt"
	rand "math/rand/v2"
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

// parseRawSpec parses and validates a send-opts map into a rawSpec. On a
// validation failure it returns a non-empty message suitable for a TypeError;
// on success it returns the spec and "". src may be set or left nil (the caller
// fills nil from the egress interface). It is privilege-free and socket-free so
// the validation rules can be unit-tested directly.
func parseRawSpec(m map[string]any) (rawSpec, string) {
	var o rawSpec
	o.proto = strings.ToLower(optString(m, "proto", "tcp"))
	switch o.proto {
	case "tcp", "udp", "ip":
	default:
		return o, fmt.Sprintf("send: proto must be 'tcp', 'udp', or 'ip', got %q", o.proto)
	}

	dstStr := optString(m, "dst", "")
	if dstStr == "" {
		return o, "send: opts.dst (destination IPv4 address) required"
	}
	o.dst = net.ParseIP(dstStr)
	if o.dst == nil || o.dst.To4() == nil {
		return o, fmt.Sprintf("send: opts.dst must be an IPv4 address, got %q", dstStr)
	}
	if srcStr := optString(m, "src", ""); srcStr != "" {
		o.src = net.ParseIP(srcStr)
		if o.src == nil || o.src.To4() == nil {
			return o, fmt.Sprintf("send: opts.src must be an IPv4 address, got %q", srcStr)
		}
	}

	o.ttl = optInt(m, "ttl", 64)
	if o.ttl < 1 || o.ttl > 255 {
		return o, fmt.Sprintf("send: ttl must be 1..255, got %d", o.ttl)
	}
	o.ipID = optInt(m, "ipId", 0)
	o.protocol = optInt(m, "protocol", 0)
	o.window = optInt(m, "window", 65535)
	o.seq = uint32(optInt(m, "seq", 0))
	o.ack = uint32(optInt(m, "ack", 0))

	o.dstPort = optInt(m, "dstPort", 0)
	if (o.proto == "tcp" || o.proto == "udp") && (o.dstPort < 1 || o.dstPort > 65535) {
		return o, fmt.Sprintf("send: dstPort must be 1..65535 for %s, got %d", o.proto, o.dstPort)
	}
	o.srcPort = optInt(m, "srcPort", 0)
	if o.srcPort == 0 {
		o.srcPort = 32768 + rand.IntN(28000) // random high port
	}
	if o.srcPort < 1 || o.srcPort > 65535 {
		return o, fmt.Sprintf("send: srcPort must be 1..65535, got %d", o.srcPort)
	}

	if raw, ok := m["flags"]; ok {
		if arr, ok := raw.([]any); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					o.flags = append(o.flags, s)
				}
			}
		}
	}
	if len(o.flags) == 0 && o.proto == "tcp" {
		o.flags = []string{"SYN"}
	}
	if pv, ok := m["payload"]; ok {
		o.payload = exportPayload(pv)
	}
	return o, ""
}
