package main

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// TestCaptureSpike confirms the pure-Go gopacket subset (gopacket + layers)
// serializes and decodes a frame — the basis for net.capture decode. This
// file is a throwaway spike (Task 1); delete after the cgo cross-compile gate.
func TestCaptureSpike(t *testing.T) {
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	eth := &layers.Ethernet{
		SrcMAC:       net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0x55},
		DstMAC:       net.HardwareAddr{0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb},
		EthernetType: layers.EthernetTypeIPv4,
	}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2)}
	udp := &layers.UDP{SrcPort: 1234, DstPort: 53}
	_ = udp.SetNetworkLayerForChecksum(ip)
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, udp, gopacket.Payload([]byte("hi"))); err != nil {
		t.Fatalf("serialize: %v", err)
	}

	pkt := gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
	l, ok := pkt.Layer(layers.LayerTypeUDP).(*layers.UDP)
	if !ok {
		t.Fatalf("no UDP layer: %v", pkt)
	}
	if l.DstPort != 53 {
		t.Fatalf("dstPort = %d, want 53", l.DstPort)
	}
}
