package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/ipv4"
)

// TestICMP_MarshalParseRoundTrip exercises the privilege-free core: marshal an
// ICMPv4 echo request, parse it back, and confirm id / seq / payload survive
// and the type is echo. MUST pass in CI (no raw socket needed).
func TestICMP_MarshalParseRoundTrip(t *testing.T) {
	const (
		id  = 0x1234
		seq = 7
	)
	payload := []byte("hello-icmp")

	b, err := marshalEcho("ip4", id, seq, payload)
	if err != nil {
		t.Fatalf("marshalEcho: %v", err)
	}

	typ, code, body, err := parseICMP("ip4", b)
	if err != nil {
		t.Fatalf("parseICMP: %v", err)
	}
	if typ != int(ipv4.ICMPTypeEcho) {
		t.Errorf("type: got %d want %d (echo)", typ, int(ipv4.ICMPTypeEcho))
	}
	if code != 0 {
		t.Errorf("code: got %d want 0", code)
	}
	// body is the marshalled Echo body: 2 bytes id, 2 bytes seq, then data.
	if len(body) < 4 {
		t.Fatalf("body too short: %d bytes", len(body))
	}
	gotID := int(body[0])<<8 | int(body[1])
	gotSeq := int(body[2])<<8 | int(body[3])
	if gotID != id {
		t.Errorf("id: got %d want %d", gotID, id)
	}
	if gotSeq != seq {
		t.Errorf("seq: got %d want %d", gotSeq, seq)
	}
	if !bytes.Equal(body[4:], payload) {
		t.Errorf("payload: got %q want %q", body[4:], payload)
	}
}

// TestICMP_OpenWithoutPrivilegesErrors opens a raw ICMP socket from a script.
// Without root / CAP_NET_RAW the open rejects with an error naming the
// privilege requirement; if the environment permits raw ICMP (e.g. a
// privileged container) the open resolves and the test skips.
func TestICMP_OpenWithoutPrivilegesErrors(t *testing.T) {
	got := runSocketScript(t, `
		let outcome;
		try {
			const h = await net.icmp.open();
			await h.close();
			outcome = "resolved";
		} catch (e) {
			outcome = "rejected: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string outcome, got %T (%v)", got, got)
	}
	if s == "resolved" {
		t.Skip("raw ICMP permitted in this environment")
	}
	if !strings.Contains(s, "privileges") && !strings.Contains(s, "CAP_NET_RAW") && !strings.Contains(s, "root") {
		t.Errorf("expected privilege-related rejection, got %q", s)
	}
}

// TestICMP_LoopbackEcho is a privileged end-to-end check: open a raw ICMP
// socket, send an echo request to 127.0.0.1, and confirm onMessage fires.
// Skipped unless running as root.
func TestICMP_LoopbackEcho(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	got := runSocketScript(t, `
		const h = await net.icmp.open();
		const out = await new Promise((res, rej) => {
			const timer = setTimeout(() => rej(new Error("timeout")), 3000);
			h.onMessage(ev => { clearTimeout(timer); res(ev.type); });
			h.onError(e => { clearTimeout(timer); rej(new Error(e)); });
			h.send({ to: "127.0.0.1", id: 0x4321, seq: 1, payload: "ping" });
		});
		await h.close();
		__capture(out);
	`)
	// EchoReply == 0 for ICMPv4.
	if got != int64(0) && got != 0 {
		t.Errorf("expected echo reply (type 0), got %v (%T)", got, got)
	}
}
