package main

import (
	"bytes"
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// decodeBuilt re-parses bytes produced by buildPacket so tests can assert on
// decoded fields (and implicitly that the bytes are a valid IPv4 packet). It
// fatals only on a decode error in the IP/L4 layers — not on application
// layers gopacket eagerly infers from well-known ports/protocols (e.g. port
// 53 → DNS, protocol 41 → IPv6), which fail on our synthetic test payloads.
func decodeBuilt(t *testing.T, b []byte) gopacket.Packet {
	t.Helper()
	pkt := gopacket.NewPacket(b, layers.LayerTypeIPv4, gopacket.DecodeOptions{NoCopy: true})
	if err := pkt.ErrorLayer(); err != nil {
		switch err.LayerType() {
		case layers.LayerTypeIPv4, layers.LayerTypeTCP, layers.LayerTypeUDP:
			t.Fatalf("decode error: %v", err.Error())
		}
	}
	return pkt
}

func TestBuildPacket_TCPSyn(t *testing.T) {
	spec := rawSpec{
		dst: net.IPv4(127, 0, 0, 1), src: net.IPv4(127, 0, 0, 1),
		proto: "tcp", ttl: 64, dstPort: 80, srcPort: 40000,
		flags: []string{"SYN"}, seq: 1, window: 65535,
	}
	b, err := buildPacket(spec)
	if err != nil {
		t.Fatalf("buildPacket: %v", err)
	}
	pkt := decodeBuilt(t, b)
	ip, _ := pkt.Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	if ip == nil || ip.TTL != 64 || ip.Protocol != layers.IPProtocolTCP {
		t.Fatalf("ip layer wrong: %+v", ip)
	}
	tcp, _ := pkt.Layer(layers.LayerTypeTCP).(*layers.TCP)
	if tcp == nil {
		t.Fatal("no tcp layer")
	}
	if !tcp.SYN || tcp.ACK || tcp.RST || tcp.FIN {
		t.Fatalf("flags wrong: SYN=%v ACK=%v RST=%v FIN=%v", tcp.SYN, tcp.ACK, tcp.RST, tcp.FIN)
	}
	if tcp.DstPort != 80 || tcp.SrcPort != 40000 || tcp.Seq != 1 {
		t.Fatalf("tcp fields wrong: %+v", tcp)
	}
	if tcp.Checksum == 0 {
		t.Fatal("tcp checksum not computed")
	}
}

func TestBuildPacket_FlagCombos(t *testing.T) {
	cases := []struct {
		name  string
		flags []string
		check func(*layers.TCP) bool
	}{
		{"synack", []string{"SYN", "ACK"}, func(t *layers.TCP) bool { return t.SYN && t.ACK }},
		{"rst", []string{"RST"}, func(t *layers.TCP) bool { return t.RST && !t.SYN }},
		{"fin", []string{"FIN"}, func(t *layers.TCP) bool { return t.FIN }},
		{"pshack", []string{"PSH", "ACK"}, func(t *layers.TCP) bool { return t.PSH && t.ACK }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, err := buildPacket(rawSpec{
				dst: net.IPv4(10, 0, 0, 1), src: net.IPv4(10, 0, 0, 2),
				proto: "tcp", ttl: 64, dstPort: 443, srcPort: 1234, flags: c.flags,
			})
			if err != nil {
				t.Fatalf("buildPacket: %v", err)
			}
			tcp, _ := decodeBuilt(t, b).Layer(layers.LayerTypeTCP).(*layers.TCP)
			if tcp == nil || !c.check(tcp) {
				t.Fatalf("flag check failed for %v: %+v", c.flags, tcp)
			}
		})
	}
}

func TestBuildPacket_UDP(t *testing.T) {
	b, err := buildPacket(rawSpec{
		dst: net.IPv4(10, 0, 0, 1), src: net.IPv4(10, 0, 0, 2),
		proto: "udp", ttl: 64, dstPort: 53, srcPort: 5000, payload: []byte("hi"),
	})
	if err != nil {
		t.Fatalf("buildPacket: %v", err)
	}
	pkt := decodeBuilt(t, b)
	udp, _ := pkt.Layer(layers.LayerTypeUDP).(*layers.UDP)
	if udp == nil || udp.DstPort != 53 || udp.SrcPort != 5000 {
		t.Fatalf("udp wrong: %+v", udp)
	}
	if !bytes.Equal(udp.Payload, []byte("hi")) {
		t.Fatalf("udp payload wrong: got %v", udp.Payload)
	}
}

func TestBuildPacket_RawProto(t *testing.T) {
	b, err := buildPacket(rawSpec{
		dst: net.IPv4(10, 0, 0, 1), src: net.IPv4(10, 0, 0, 2),
		proto: "ip", protocol: 41, ttl: 64, payload: []byte{0xde, 0xad},
	})
	if err != nil {
		t.Fatalf("buildPacket: %v", err)
	}
	ip, _ := decodeBuilt(t, b).Layer(layers.LayerTypeIPv4).(*layers.IPv4)
	if ip == nil || ip.Protocol != layers.IPProtocol(41) {
		t.Fatalf("raw proto wrong: %+v", ip)
	}
	if !bytes.Equal(ip.Payload, []byte{0xde, 0xad}) {
		t.Fatalf("raw proto payload wrong: got %v", ip.Payload)
	}
}

func TestBuildPacket_RejectsNonIPv4(t *testing.T) {
	_, err := buildPacket(rawSpec{
		dst: net.ParseIP("::1"), src: net.ParseIP("::1"),
		proto: "tcp", ttl: 64, dstPort: 80, srcPort: 40000, flags: []string{"SYN"},
	})
	if err == nil {
		t.Fatal("expected error for non-IPv4 src/dst")
	}
}

func TestBuildPacket_EmptyProtoDefaultsTCP(t *testing.T) {
	b, err := buildPacket(rawSpec{
		dst: net.IPv4(10, 0, 0, 1), src: net.IPv4(10, 0, 0, 2),
		proto: "", ttl: 64, dstPort: 443, srcPort: 1234, flags: []string{"SYN"},
	})
	if err != nil {
		t.Fatalf("buildPacket with empty proto: %v", err)
	}
	tcp, _ := decodeBuilt(t, b).Layer(layers.LayerTypeTCP).(*layers.TCP)
	if tcp == nil {
		t.Fatal("expected TCP layer for empty proto (default)")
	}
}

func TestApplyTCPFlags_Unknown(t *testing.T) {
	var tcp layers.TCP
	if err := applyTCPFlags(&tcp, []string{"SYN", "BOGUS"}); err == nil {
		t.Fatal("expected error for unknown flag")
	}
}
