// Raw TCP echo server + client round-trip, fully offline.
//
// server.tcp.listen binds an inbound TCP listener; the connection handler
// receives the SAME handle shape as net.tcp.connect (onData/onClose/
// onError/write/close, remote/local). Here the server echoes every chunk
// straight back. port:0 binds an OS-chosen ephemeral port; we read it back
// from srv.address ("tcp/127.0.0.1:PORT").

const srv: any = await server.tcp.listen({ port: 0 }, (conn: any) => {
  conn.onData((ev: any) => conn.write(ev.bytes));   // echo bytes back
  conn.onError((e: any) => runtime.log("server conn error", String(e)));
});

const port = Number(srv.address.split(":").pop());
runtime.log("tcp echo server on", srv.address);

// Client: connect, register the echo listener, then send a message.
const c: any = await net.tcp.connect("127.0.0.1", port);

let resolveEcho: (s: string) => void;
const echoed = new Promise<string>((resolve) => {
  resolveEcho = resolve;
});
c.onData((ev: any) => resolveEcho(ev.text));

await c.write("hello-tcp");

// Await the echo with a timeout so a lost byte fails loudly.
const text = await Promise.race([
  echoed,
  new Promise<string>((_, reject) =>
    setTimeout(() => reject(new Error("timed out waiting for TCP echo")), 2000)
  ),
]);

if (text !== "hello-tcp") {
  throw new Error(`TCP echo mismatch: got ${JSON.stringify(text)}`);
}
runtime.log("tcp echo round-trip ok:", text);

await c.close();
await srv.close();
runtime.log("closed");
