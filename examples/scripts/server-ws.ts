// Demonstrates res.upgradeWebSocket — async iterator over WS frames.
// Self-tests the handshake via net.probe.wss. Echoes text frames back
// to the peer; closes cleanly on "bye".

const port = 38082;

const srv = await server.http.listen({
  port,
  routes: {
    "GET /ws": async (req: any, res: any) => {
      const ws = await res.upgradeWebSocket();
      for await (const msg of ws) {
        if (msg.type === "text" && msg.text === "bye") {
          await ws.close(1000, "bye");
          break;
        }
        if (msg.type === "text") {
          await ws.send("echo:" + msg.text);
        }
      }
    },
  },
});

runtime.log("listening on", srv.address);

const probe = await net.probe.wss(`ws://127.0.0.1:${port}/ws`);
runtime.assert.ok(probe.connected, "wss handshake");
runtime.log("handshake ms:", probe.handshakeMs);

await srv.close();
runtime.log("closed");
