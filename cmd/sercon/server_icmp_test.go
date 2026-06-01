package main

import (
	"os"
	"strings"
	"testing"
)

// TestICMPServer_ListenWithoutPrivileges: server.icmp.listen needs root /
// CAP_NET_RAW; without it the synchronous bind throws naming the requirement.
// If the environment permits raw ICMP, the listen succeeds and the test skips.
func TestICMPServer_ListenWithoutPrivileges(t *testing.T) {
	got := runSocketScript(t, `
		let outcome;
		try {
			const srv = server.icmp.listen({}, () => {});
			outcome = "listening";
			await srv.close();
		} catch (e) {
			outcome = "threw: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T (%v)", got, got)
	}
	if s == "listening" {
		t.Skip("raw ICMP permitted in this environment")
	}
	if !strings.Contains(s, "privileges") && !strings.Contains(s, "CAP_NET_RAW") && !strings.Contains(s, "root") {
		t.Errorf("expected privilege-related rejection, got %q", s)
	}
}

// TestICMPServer_NotAFunction: a non-function handler throws synchronously,
// before any socket is opened (so it runs without privileges).
func TestICMPServer_NotAFunction(t *testing.T) {
	got := runSocketScript(t, `
		let outcome;
		try {
			server.icmp.listen({}, 123);
			outcome = "no-throw";
		} catch (e) {
			outcome = "threw: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	s, _ := got.(string)
	if !strings.Contains(s, "threw:") || !strings.Contains(s, "handler") {
		t.Errorf("expected handler-required throw, got %q", s)
	}
}

// TestICMPServer_LoopbackReceive is a privileged end-to-end check: a server
// listens, a net.icmp client sends an echo to 127.0.0.1, and the server's
// handler fires. Skipped unless running as root.
func TestICMPServer_LoopbackReceive(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	got := runSocketScript(t, `
		const seen = await new Promise(async (res, rej) => {
			const timer = setTimeout(() => rej(new Error("timeout")), 3000);
			const srv = server.icmp.listen({}, (msg) => { clearTimeout(timer); res(msg.type); });
			const c = await net.icmp.open();
			await c.send({ to: "127.0.0.1", id: 1, seq: 1, payload: "ping" });
			await c.close();
			await srv.close();
		});
		__capture(typeof seen === "number" ? "received" : "nope");
	`)
	if got != "received" {
		t.Errorf("expected the server handler to fire, got %v", got)
	}
}
