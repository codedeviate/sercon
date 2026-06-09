package main

import (
	"net"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
)

// buildFrame serializes an Ethernet/IPv4/{TCP|UDP} frame with the given proto,
// src/dst IPs and ports, and returns the parsed gopacket.Packet.
func buildFrame(t *testing.T, proto layers.IPProtocol, srcIP, dstIP string, srcPort, dstPort int) gopacket.Packet {
	t.Helper()
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: proto, SrcIP: net.ParseIP(srcIP).To4(), DstIP: net.ParseIP(dstIP).To4()}
	eth := &layers.Ethernet{SrcMAC: mustMAC("00:11:22:33:44:55"), DstMAC: mustMAC("66:77:88:99:aa:bb"), EthernetType: layers.EthernetTypeIPv4}

	var transport gopacket.SerializableLayer
	switch proto {
	case layers.IPProtocolTCP:
		tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: layers.TCPPort(dstPort)}
		_ = tcp.SetNetworkLayerForChecksum(ip)
		transport = tcp
	case layers.IPProtocolUDP:
		udp := &layers.UDP{SrcPort: layers.UDPPort(srcPort), DstPort: layers.UDPPort(dstPort)}
		_ = udp.SetNetworkLayerForChecksum(ip)
		transport = udp
	default:
		t.Fatalf("buildFrame: unsupported proto %v", proto)
	}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, transport, gopacket.Payload([]byte("hi"))); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

// buildFrame6 serializes an Ethernet/IPv6/TCP frame.
func buildFrame6(t *testing.T, srcIP, dstIP string, srcPort, dstPort int) gopacket.Packet {
	t.Helper()
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	ip := &layers.IPv6{Version: 6, HopLimit: 64, NextHeader: layers.IPProtocolTCP, SrcIP: net.ParseIP(srcIP), DstIP: net.ParseIP(dstIP)}
	tcp := &layers.TCP{SrcPort: layers.TCPPort(srcPort), DstPort: layers.TCPPort(dstPort)}
	_ = tcp.SetNetworkLayerForChecksum(ip)
	eth := &layers.Ethernet{SrcMAC: mustMAC("00:11:22:33:44:55"), DstMAC: mustMAC("66:77:88:99:aa:bb"), EthernetType: layers.EthernetTypeIPv6}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, tcp, gopacket.Payload([]byte("hi"))); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

// buildICMPFrame serializes an Ethernet/IPv4/ICMPv4 echo-request frame.
func buildICMPFrame(t *testing.T, srcIP, dstIP string) gopacket.Packet {
	t.Helper()
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolICMPv4, SrcIP: net.ParseIP(srcIP).To4(), DstIP: net.ParseIP(dstIP).To4()}
	icmp := &layers.ICMPv4{TypeCode: layers.CreateICMPv4TypeCode(layers.ICMPv4TypeEchoRequest, 0)}
	eth := &layers.Ethernet{SrcMAC: mustMAC("00:11:22:33:44:55"), DstMAC: mustMAC("66:77:88:99:aa:bb"), EthernetType: layers.EthernetTypeIPv4}
	if err := gopacket.SerializeLayers(buf, opts, eth, ip, icmp, gopacket.Payload([]byte("hi"))); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return gopacket.NewPacket(buf.Bytes(), layers.LayerTypeEthernet, gopacket.Default)
}

func TestCaptureFilter(t *testing.T) {
	tcp80 := buildFrame(t, layers.IPProtocolTCP, "10.0.0.1", "10.0.0.2", 1234, 80)
	udp53 := buildFrame(t, layers.IPProtocolUDP, "10.0.0.3", "8.8.8.8", 5000, 53)
	icmp4 := buildICMPFrame(t, "10.0.0.1", "10.0.0.2")
	v6tcp := buildFrame6(t, "fe80::1", "fe80::2", 1111, 443)

	cases := []struct {
		expr string
		pkt  gopacket.Packet
		want bool
	}{
		{"tcp", tcp80, true}, {"tcp", udp53, false},
		{"udp", udp53, true}, {"icmp", icmp4, true},
		{"port 80", tcp80, true}, {"port 80", udp53, false},
		{"dst port 80", tcp80, true}, {"src port 80", tcp80, false},
		{"host 10.0.0.1", tcp80, true}, {"src host 10.0.0.1", tcp80, true}, {"dst host 10.0.0.1", tcp80, false},
		{"ip6", v6tcp, true}, {"ip", tcp80, true}, {"ip", v6tcp, false},
		{"tcp and port 80", tcp80, true}, {"tcp port 80", tcp80, true}, // implicit and
		{"udp or icmp", icmp4, true}, {"not udp", tcp80, true}, {"not udp", udp53, false},
		{"tcp and (port 80 or port 443)", tcp80, true},
		// CIDR (net X/Y)
		{"net 10.0.0.0/8", tcp80, true}, {"net 10.0.0.0/8", udp53, true},
		{"net 192.168.0.0/16", tcp80, false},
		{"src net 10.0.0.0/30", tcp80, true}, {"dst net 10.0.0.0/30", tcp80, true},
		{"src net 10.0.0.3/32", tcp80, false}, {"src net 10.0.0.3/32", udp53, true},
		{"dst net 8.8.8.0/24", udp53, true},
		{"net 2001:db8::/32", v6tcp, false}, {"net fe80::/16", v6tcp, true},
		// portrange
		{"portrange 70-90", tcp80, true}, {"portrange 70-90", udp53, false},
		{"portrange 50-60", tcp80, false},
		{"dst portrange 79-81", tcp80, true}, {"src portrange 79-81", tcp80, false},
		{"src portrange 1000-2000", tcp80, true}, // src port 1234
		{"udp portrange 1-100", udp53, true},      // dst port 53
	}
	for _, c := range cases {
		f, err := compileFilter(c.expr)
		if err != nil {
			t.Fatalf("compile %q: %v", c.expr, err)
		}
		if got := f.match(c.pkt); got != c.want {
			t.Errorf("%q: got %v want %v", c.expr, got, c.want)
		}
	}
}

func TestCaptureFilter_Invalid(t *testing.T) {
	for _, expr := range []string{
		"port", "host", "tcp and", "(tcp", "port abc", "blah",
		"net", "net not-a-cidr", "net 10.0.0.1", // bare IP, no mask
		"portrange", "portrange 80", "portrange 80-", "portrange -80",
		"portrange abc-90", "portrange 90-80", // low > high
		"src net", "dst portrange",
	} {
		if _, err := compileFilter(expr); err == nil {
			t.Errorf("expected error for %q", expr)
		}
	}
}
