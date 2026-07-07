package main

import (
	"strconv"
	"strings"
	"testing"
)

// --port-override is documented to replace the port of EVERY
// server.*.listen (except icmp, which has no port). tcpListen/udpListen
// ignored it. Set the override and assert a listen({port:0}) actually binds
// the override port.
func TestServerTCP_PortOverrideApplied(t *testing.T) {
	override := freePort(t)
	servePortOverride = override
	defer func() { servePortOverride = 0 }()
	got := runSocketScript(t, `
		const srv = server.tcp.listen({ port: 0 }, conn => {});
		__capture(srv.address);
		srv.close();
	`)
	addr, _ := got.(string)
	if !strings.HasSuffix(addr, ":"+strconv.Itoa(override)) {
		t.Errorf("tcp --port-override ignored: address=%q, want port %d", addr, override)
	}
}

func TestServerUDP_PortOverrideApplied(t *testing.T) {
	override := freePort(t)
	servePortOverride = override
	defer func() { servePortOverride = 0 }()
	got := runSocketScript(t, `
		const srv = server.udp.listen({ port: 0 }, msg => {});
		__capture(srv.address);
		srv.close();
	`)
	addr, _ := got.(string)
	if !strings.HasSuffix(addr, ":"+strconv.Itoa(override)) {
		t.Errorf("udp --port-override ignored: address=%q, want port %d", addr, override)
	}
}

// server.icmp.listen documents opts as optional, so listen(handler) with no
// opts must be accepted. Binding a raw ICMP socket needs privileges, so the
// call may still fail at bind without root — but it must NOT fail with the
// argument-parse error "handler function required".
func TestServerICMP_ListenAcceptsHandlerOnly(t *testing.T) {
	got := runSocketScript(t, `
		let msg = "ok";
		try {
			const s = server.icmp.listen((m, reply) => {});
			if (s && s.close) s.close();
		} catch (e) { msg = String(e); }
		__capture(msg);
	`)
	msg, _ := got.(string)
	if strings.Contains(msg, "handler function required") {
		t.Errorf("icmp.listen(handler) wrongly rejected the supplied handler: %s", msg)
	}
}
