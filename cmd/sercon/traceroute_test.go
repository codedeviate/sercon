package main

import (
	"os"
	"strings"
	"testing"
)

// buildQuoted constructs a minimal quoted "IP header + 8 transport bytes" blob
// as it appears inside an ICMP time-exceeded body: a 20-byte IPv4 header
// (IHL=5) carrying `proto`, followed by the first 8 bytes of the transport
// header `payload8`.
func buildQuoted(proto byte, payload8 []byte) []byte {
	ip := make([]byte, 20)
	ip[0] = 0x45 // version 4, IHL 5
	ip[9] = proto
	return append(ip, payload8...)
}

func TestParseQuotedProbe(t *testing.T) {
	// ICMP echo first 8 bytes: type(8) code(0) csum(2) id(2) seq(2). seq=0x1234.
	icmp8 := []byte{8, 0, 0, 0, 0x00, 0x01, 0x12, 0x34}
	if id, ok := parseQuotedProbe(buildQuoted(1, icmp8), "icmp"); !ok || id != 0x1234 {
		t.Errorf("icmp: got %d ok=%v want 0x1234", id, ok)
	}
	// UDP first 8 bytes: srcPort(2) dstPort(2) len(2) csum(2). dstPort=0x82F2.
	udp8 := []byte{0xC0, 0x00, 0x82, 0xF2, 0, 8, 0, 0}
	if id, ok := parseQuotedProbe(buildQuoted(17, udp8), "udp"); !ok || id != 0x82F2 {
		t.Errorf("udp: got %d ok=%v want 0x82F2", id, ok)
	}
	// TCP first 8 bytes: srcPort(2) dstPort(2) seq(4). srcPort=0xC123.
	tcp8 := []byte{0xC1, 0x23, 0x00, 0x50, 0, 0, 0, 0}
	if id, ok := parseQuotedProbe(buildQuoted(6, tcp8), "tcp"); !ok || id != 0xC123 {
		t.Errorf("tcp: got %d ok=%v want 0xC123", id, ok)
	}
	// Too short → not ok.
	if _, ok := parseQuotedProbe([]byte{0x45, 0, 0}, "icmp"); ok {
		t.Error("short input should not parse")
	}
	// Honors IHL (header with options): IHL=6 (24-byte header).
	ipOpt := make([]byte, 24)
	ipOpt[0] = 0x46 // IHL 6
	ipOpt[9] = 1
	withOpts := append(ipOpt, icmp8...)
	if id, ok := parseQuotedProbe(withOpts, "icmp"); !ok || id != 0x1234 {
		t.Errorf("icmp w/ options: got %d ok=%v want 0x1234", id, ok)
	}
}

// TestTraceroute_NoPrivilegeRejects: without root, opening the raw ICMP socket
// fails and traceroute rejects with a privilege hint (skips if raw ICMP is
// permitted in this environment).
func TestTraceroute_NoPrivilegeRejects(t *testing.T) {
	got := runSocketScript(t, `
		let outcome;
		try {
			await net.probe.traceroute("127.0.0.1", { maxHops: 1 });
			outcome = "ok";
		} catch (e) {
			outcome = "threw: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	s, _ := got.(string)
	if s == "ok" {
		t.Skip("raw ICMP permitted in this environment")
	}
	if !strings.Contains(s, "privileges") && !strings.Contains(s, "CAP_NET_RAW") && !strings.Contains(s, "root") {
		t.Errorf("expected privilege rejection, got %q", s)
	}
}

// TestTraceroute_BadProtocol: an unknown protocol rejects before any socket.
func TestTraceroute_BadProtocol(t *testing.T) {
	got := runSocketScript(t, `
		let outcome;
		try {
			await net.probe.traceroute("127.0.0.1", { protocol: "sctp" });
			outcome = "ok";
		} catch (e) {
			outcome = "threw: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	s, _ := got.(string)
	if !strings.Contains(s, "threw:") || !strings.Contains(s, "protocol") {
		t.Errorf("expected protocol rejection, got %q", s)
	}
}

// TestTraceroute_LoopbackICMP: privileged — tracing 127.0.0.1 reaches in one
// hop. Skipped unless root.
func TestTraceroute_LoopbackICMP(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	got := runSocketScript(t, `
		const hops = await net.probe.traceroute("127.0.0.1", { protocol: "icmp", maxHops: 5, timeout: 1000, probes: 1 });
		const last = hops[hops.length - 1];
		__capture(last && last.reached && last.address === "127.0.0.1" ? "reached" : JSON.stringify(hops));
	`)
	if got != "reached" {
		t.Errorf("loopback ICMP trace: %v", got)
	}
}
