package main

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

func mustMAC(s string) net.HardwareAddr {
	mac, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return mac
}

func TestDecodePacket_EthIPv4UDP(t *testing.T) {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2)}
	udp := &layers.UDP{SrcPort: 1234, DstPort: 53}
	_ = udp.SetNetworkLayerForChecksum(ip)
	if err := gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{SrcMAC: mustMAC("00:11:22:33:44:55"), DstMAC: mustMAC("66:77:88:99:aa:bb"), EthernetType: layers.EthernetTypeIPv4},
		ip, udp,
		gopacket.Payload([]byte("hi"))); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	data := buf.Bytes()
	o := decodePacket(data, layers.LinkTypeEthernet, gopacket.CaptureInfo{Length: len(data), CaptureLength: len(data)})
	m := o.ToMap()
	ip4, ok := m["ip"].(map[string]any)
	if !ok {
		t.Fatalf("no ip layer: %#v", m)
	}
	u, ok := m["udp"].(map[string]any)
	if !ok {
		t.Fatalf("no udp layer: %#v", m)
	}
	if ip4["src"] != "10.0.0.1" {
		t.Fatalf("ip.src = %v, want 10.0.0.1", ip4["src"])
	}
	if u["dstPort"] != 53 {
		t.Fatalf("udp.dstPort = %v, want 53", u["dstPort"])
	}
}

func TestDecodePacket_TCPFlags(t *testing.T) {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolTCP, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2)}
	tcp := &layers.TCP{SrcPort: 1234, DstPort: 80, SYN: true}
	_ = tcp.SetNetworkLayerForChecksum(ip)
	if err := gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{SrcMAC: mustMAC("00:11:22:33:44:55"), DstMAC: mustMAC("66:77:88:99:aa:bb"), EthernetType: layers.EthernetTypeIPv4},
		ip, tcp); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	data := buf.Bytes()
	o := decodePacket(data, layers.LinkTypeEthernet, gopacket.CaptureInfo{Length: len(data), CaptureLength: len(data)})
	m := o.ToMap()
	tc, ok := m["tcp"].(map[string]any)
	if !ok {
		t.Fatalf("no tcp layer: %#v", m)
	}
	flags, ok := tc["flags"].(map[string]any)
	if !ok {
		t.Fatalf("no tcp.flags: %#v", tc)
	}
	if flags["syn"] != true {
		t.Fatalf("tcp.flags.syn = %v, want true", flags["syn"])
	}
}

func TestDecodePacket_Truncated(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("decodePacket panicked on truncated input: %v", r)
		}
	}()
	data := []byte{0x00, 0x01, 0x02}
	o := decodePacket(data, layers.LinkTypeEthernet, gopacket.CaptureInfo{Length: 3, CaptureLength: 3})
	m := o.ToMap()
	if _, ok := m["bytes"]; !ok {
		t.Fatalf("missing bytes key: %#v", m)
	}
	if m["length"] != 3 {
		t.Fatalf("length = %v, want 3", m["length"])
	}
}
