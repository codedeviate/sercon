package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/gopacket/gopacket/pcapgo"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func mustMAC(s string) net.HardwareAddr {
	mac, err := net.ParseMAC(s)
	if err != nil {
		panic(err)
	}
	return mac
}

// buildUDPFrame serializes an Ethernet/IPv4/UDP frame with the given dst port
// and a small payload — the fixture both file round-trip tests write and read.
func buildUDPFrame(t *testing.T, dstPort layers.UDPPort) []byte {
	t.Helper()
	buf := gopacket.NewSerializeBuffer()
	opts := gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}
	ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IPv4(10, 0, 0, 1), DstIP: net.IPv4(10, 0, 0, 2)}
	udp := &layers.UDP{SrcPort: 1234, DstPort: dstPort}
	_ = udp.SetNetworkLayerForChecksum(ip)
	if err := gopacket.SerializeLayers(buf, opts,
		&layers.Ethernet{SrcMAC: mustMAC("00:11:22:33:44:55"), DstMAC: mustMAC("66:77:88:99:aa:bb"), EthernetType: layers.EthernetTypeIPv4},
		ip, udp,
		gopacket.Payload([]byte("hi"))); err != nil {
		t.Fatalf("serialize: %v", err)
	}
	return buf.Bytes()
}

// runCaptureScript is runSocketScript plus an extra Go register applied before
// Run — used to inject the __frame []byte fixture the file round-trip needs.
func runCaptureScript(t *testing.T, body string, registers map[string]any) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	for name, val := range registers {
		if err := eng.Register(name, val); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := eng.Run(context.Background(), "s.ts", body); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// TestCaptureFile_GoRoundTrip proves the writer/reader/decode chain offline,
// without the engine: serialize a frame, write it to a .pcap via pcapgo, read
// it back, decode it, and assert the UDP dst port survives.
func TestCaptureFile_GoRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rt.pcap")
	frame := buildUDPFrame(t, 4242)

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := pcapgo.NewWriter(f)
	if err := w.WriteFileHeader(262144, layers.LinkTypeEthernet); err != nil {
		t.Fatal(err)
	}
	n := len(frame)
	if err := w.WritePacket(gopacket.CaptureInfo{Timestamp: time.Now(), CaptureLength: n, Length: n}, frame); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	rf, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer rf.Close()
	r, err := pcapgo.NewReader(rf)
	if err != nil {
		t.Fatal(err)
	}
	data, ci, err := r.ReadPacketData()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	o := decodePacket(data, r.LinkType(), ci)
	m := o.ToMap()
	udp, ok := m["udp"].(map[string]any)
	if !ok {
		t.Fatalf("no udp layer: %#v", m)
	}
	if udp["dstPort"] != 4242 {
		t.Fatalf("udp.dstPort = %v, want 4242", udp["dstPort"])
	}
}

// TestCaptureFile_ScriptRoundTrip drives the JS-facing bindings end to end:
// write a known frame with net.capture.toFile, then read it back with
// net.capture.openFile and assert the decoded dst port.
func TestCaptureFile_ScriptRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rt.pcap")
	frame := buildUDPFrame(t, 4242)

	got := runCaptureScript(t, fmt.Sprintf(`
		const w = net.capture.toFile(%q);
		w.write(__frame);
		await w.close();
		const ports = [];
		await net.capture.openFile(%q, p => ports.push(p.udp.dstPort));
		__capture(String(ports[0]));
	`, path, path), map[string]any{"__frame": frame})

	if got != "4242" {
		t.Fatalf("script round-trip: got %q want %q", got, "4242")
	}
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

func TestCaptureInterfaces_ListsLoopback(t *testing.T) {
	got := runSocketScript(t, `
		const ifs = net.capture.interfaces();
		if (!Array.isArray(ifs) || ifs.length === 0) throw new Error("no interfaces");
		const lo = ifs.find(i => i.loopback);
		__capture(lo ? "lo:" + (typeof lo.name) + ":" + Array.isArray(lo.addresses) : "none");
	`)
	if got != "lo:string:true" {
		t.Fatalf("interfaces(): expected a loopback iface with name+addresses; got %q", got)
	}
}
