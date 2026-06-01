package main

import (
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
)

// TestRaw_OpenRejectsOrOpens: net.raw.open must be wired; opening either
// succeeds (root) or rejects with a privilege/platform hint. Either way the
// binding exists and returns a Promise.
func TestRaw_OpenRejectsOrOpens(t *testing.T) {
	got := runSocketScript(t, `
		let out = "";
		try {
			const h = await net.raw.open({ iface: "lo" });
			await h.close();
			out = "opened";
		} catch (e) {
			out = "rejected:" + e.message;
		}
		__capture(out);
	`)
	s, _ := got.(string)
	if !strings.HasPrefix(s, "opened") && !strings.HasPrefix(s, "rejected:") {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRaw_LoopbackRoundTrip(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("loopback round-trip is reliable only on Linux (macOS lo0 is DLT_NULL)")
	}
	if os.Geteuid() != 0 {
		t.Skip("needs root / CAP_NET_RAW for raw send + capture")
	}

	// A real listener on an OS-assigned port → a crafted SYN should draw SYN/ACK.
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			_ = c.Close()
		}
	}()
	openPort := ln.Addr().(*net.TCPAddr).Port

	// Open port: expect SYN/ACK. Drive the full JS path via net.raw.tcp.
	got := runSocketScript(t, fmt.Sprintf(`
		const r = await net.raw.tcp("127.0.0.1", %d, { iface: "lo", srcPort: 41000, timeout: 2000 });
		__capture(r && r.tcp ? { syn: r.tcp.flags.syn, ack: r.tcp.flags.ack, rst: r.tcp.flags.rst } : null);
	`, openPort))
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("open port: expected a reply object, got %#v (nil = no SYN/ACK captured)", got)
	}
	if m["syn"] != true || m["ack"] != true {
		t.Fatalf("open port: want SYN+ACK, got %#v", m)
	}

	// Closed port (1 is almost always closed): expect RST, or null if filtered.
	got2 := runSocketScript(t, `
		const r = await net.raw.tcp("127.0.0.1", 1, { iface: "lo", srcPort: 41001, timeout: 1500 });
		__capture(r && r.tcp ? { rst: r.tcp.flags.rst, syn: r.tcp.flags.syn } : null);
	`)
	if m2, ok := got2.(map[string]any); ok {
		if m2["rst"] != true {
			t.Fatalf("closed port: want RST, got %#v", m2)
		}
	} // null (no reply / filtered) is an acceptable outcome and not asserted.
}
