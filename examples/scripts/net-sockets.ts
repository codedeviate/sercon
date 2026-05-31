// Raw client sockets: net.tcp.connect / net.udp.open / net.icmp.open.
// These are long-lived, bidirectional sockets with a push/callback read
// model (onData / onMessage, onClose, onError, close) — unlike the
// one-shot net.probe.* helpers.
//
// The runnable assertion below is an OFFLINE-SAFE UDP loopback round-trip,
// so this script passes in CI with no network. TCP and ICMP usage are
// shown for reference only:
//
//   // TCP (needs a peer to dial — scripts can't bind a TCP server yet):
//   const t = await net.tcp.connect("example.com", "80", { timeout: 5000 });
//   t.onData(ev => runtime.log("recv", ev.bytes.length, "bytes:", ev.text));
//   t.onClose(() => runtime.log("tcp closed"));
//   t.onError(e => runtime.log("tcp error", String(e)));
//   await t.write("GET / HTTP/1.0\r\n\r\n");
//   runtime.log("remote", t.remote, "local", t.local);
//   await t.close();
//
//   // ICMP (raw socket — needs root / CAP_NET_RAW; open() rejects otherwise).
//   // send() has two modes: Echo { to, type?, code?, id?, seq?, payload? }
//   // and raw { to, type, code?, body } (body marshalled verbatim).
//   const ic = await net.icmp.open({ network: "ip4" });
//   ic.onMessage(ev => runtime.log("icmp", ev.type, ev.code, "from", ev.address));
//   await ic.send({ to: "127.0.0.1", id: 1, seq: 1, payload: "ping" });
//   // hand-built destination-unreachable (type 3, code 1) — raw body:
//   await ic.send({ to: "127.0.0.1", type: 3, code: 1, body: new Uint8Array([0, 0, 0, 0]) });
//   await ic.close();

// --- UDP loopback self-test (the part that actually runs) ---

// Bind a server socket on an OS-chosen port, then read the port back from
// srv.local (format "127.0.0.1:PORT").
const srv: any = await net.udp.open({ bind: "127.0.0.1:0" });
const port = Number(srv.local.split(":").pop());
runtime.log("udp server bound on", srv.local);

// A promise that resolves with the first datagram's text.
let resolveMsg: (s: string) => void;
const received = new Promise<string>((resolve) => {
  resolveMsg = resolve;
});
srv.onMessage((ev: any) => {
  runtime.log("server got", JSON.stringify(ev.text), "from", ev.address + ":" + ev.port);
  resolveMsg(ev.text);
});

// Connected client pointed at the server; send one datagram.
const cli: any = await net.udp.open({ host: "127.0.0.1", port });
await cli.send("hello-sockets");

// Await the message with a sane timeout so a lost datagram fails loudly.
const text = await Promise.race([
  received,
  new Promise<string>((_, reject) =>
    setTimeout(() => reject(new Error("timed out waiting for UDP datagram")), 2000)
  ),
]);

runtime.assert.equal(text, "hello-sockets", "UDP loopback payload mismatch");
runtime.log("udp loopback round-trip ok:", text);

await cli.close();
await srv.close();
runtime.log("closed");
