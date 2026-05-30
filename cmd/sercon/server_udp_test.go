package main

import "testing"

func TestServerUDP_EchoRoundTrip(t *testing.T) {
	got := runSocketScript(t, `
		const srv = server.udp.listen({ port: 0 }, (msg, reply) => { reply(msg.bytes); });
		const port = Number(srv.address.split(":").pop());   // "udp/127.0.0.1:PORT"
		const u = await net.udp.open({ host: "127.0.0.1", port });
		const out = await new Promise(res => { u.onMessage(ev => res(ev.text)); u.send("hello-udp"); });
		await u.close(); srv.close();
		__capture(out);
	`)
	if got != "hello-udp" {
		t.Errorf("server udp echo: got %q", got)
	}
}

func TestServerUDP_BindErrorThrows(t *testing.T) {
	got := runSocketScript(t, `
		const a = server.udp.listen({ port: 0 }, () => {});
		const port = Number(a.address.split(":").pop());
		let threw = false;
		try {
			server.udp.listen({ host: "127.0.0.1", port }, () => {});
		} catch (e) {
			threw = true;
		}
		a.close();
		__capture(threw);
	`)
	if got != true {
		t.Errorf("server udp double-bind: expected throw, got %v", got)
	}
}
