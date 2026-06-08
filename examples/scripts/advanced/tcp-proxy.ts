// Demonstrates a minimal TCP proxy using server.tcp.listen + net.tcp.connect.
// An upstream echo server reflects every chunk back to the sender.
// A proxy server accepts client connections, dials the upstream, and relays
// bytes in both directions (client→upstream and upstream→client).
// A test client connects to the proxy, sends a payload, and asserts it
// receives the echoed payload back through the proxy.

// ── upstream echo server (port 0 = OS-chosen ephemeral) ──────────────────────
const upstream: any = await server.tcp.listen({ port: 0 }, (conn: any) => {
  conn.onData((ev: any) => conn.write(ev.bytes));
  conn.onError((e: any) => runtime.log("upstream conn error:", String(e)));
});

const upstreamPort = Number(upstream.address.split(":").pop());
runtime.log("upstream echo server on", upstream.address);

// ── proxy server ──────────────────────────────────────────────────────────────
const proxy: any = await server.tcp.listen({ port: 0 }, async (clientConn: any) => {
  // Dial the upstream for each accepted client connection.
  const upConn: any = await net.tcp.connect("127.0.0.1", String(upstreamPort));

  // client → upstream
  clientConn.onData((ev: any) => upConn.write(ev.bytes));
  clientConn.onClose(() => upConn.close());
  clientConn.onError((e: any) => runtime.log("proxy client error:", String(e)));

  // upstream → client
  upConn.onData((ev: any) => clientConn.write(ev.bytes));
  upConn.onClose(() => clientConn.close());
  upConn.onError((e: any) => runtime.log("proxy upstream error:", String(e)));
});

const proxyPort = Number(proxy.address.split(":").pop());
runtime.log("proxy server on", proxy.address);

// ── test client ───────────────────────────────────────────────────────────────
const PAYLOAD = "hello-through-proxy";

const client: any = await net.tcp.connect("127.0.0.1", String(proxyPort));

let resolveEcho: (s: string) => void;
const echoed = new Promise<string>((resolve) => {
  resolveEcho = resolve;
});
client.onData((ev: any) => resolveEcho(ev.text));

await client.write(PAYLOAD);

// Wait for the echo with a generous timeout.
const received = await Promise.race([
  echoed,
  new Promise<string>((_, reject) =>
    setTimeout(() => reject(new Error("timed out waiting for proxied echo")), 5000)
  ),
]);

runtime.log("received back:", JSON.stringify(received));
runtime.assert.equal(received, PAYLOAD, "proxy round-trip payload mismatch");

await client.close();
await proxy.close();
await upstream.close();

runtime.log("PASS");
