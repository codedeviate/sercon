package main

import (
	"testing"
)

func TestServerTCP_EchoRoundTrip(t *testing.T) {
	got := runSocketScript(t, `
		const srv = server.tcp.listen({ port: 0 }, conn => {
			conn.onData(ev => { conn.write(ev.bytes); });   // echo
		});
		const port = Number(srv.address.split(":").pop());   // "tcp/127.0.0.1:PORT"
		const c = await net.tcp.connect("127.0.0.1", port);
		const out = await new Promise(res => { c.onData(ev => res(ev.text)); c.write("hello-server"); });
		await c.close();
		srv.close();
		__capture(out);
	`)
	if got != "hello-server" {
		t.Errorf("server tcp echo: got %q", got)
	}
}

func TestServerTCP_BindErrorThrows(t *testing.T) {
	got := runSocketScript(t, `
		const srv = server.tcp.listen({ port: 0 }, conn => {});
		const port = Number(srv.address.split(":").pop());
		let marker = "no-throw";
		try {
			const dup = server.tcp.listen({ port: port }, conn => {});
			dup.close();
		} catch (e) {
			marker = "threw";
		}
		srv.close();
		__capture(marker);
	`)
	if got != "threw" {
		t.Errorf("expected duplicate bind to throw, got %q", got)
	}
}
