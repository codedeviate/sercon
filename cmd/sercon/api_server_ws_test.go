package main

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestServerHTTP_WebSocket verifies the upgrade path end-to-end at the
// handshake level: a route calls res.upgradeWebSocket(), runs the
// iterator until the peer closes, then the script closes the server.
// Full message round-trip would need a script-side WebSocket dialer —
// net.probe.wss is handshake-only, which is exactly the surface we
// want to guard against regression here (upgrade returns, dispatcher
// unblocks, HoldRun release happens).
func TestServerHTTP_WebSocket(t *testing.T) {
	port := freePort(t)
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        10 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	script := `
const srv = await server.http.listen({
  port: ` + strconv.Itoa(port) + `,
  routes: {
    "GET /ws": async (req, res) => {
      const ws = await res.upgradeWebSocket();
      for await (const msg of ws) {
        if (msg.type === "text" && msg.text === "bye") {
          await ws.close(1000, "client said bye");
          break;
        }
      }
    },
  },
});

// Handshake-only self-test (net.probe.wss closes immediately after the
// 101 upgrade). The handler's for-await loop then sees the recv channel
// close, exits cleanly, and the HoldRun releases.
const probe = await net.probe.wss("ws://127.0.0.1:` + strconv.Itoa(port) + `/ws");
if (!probe.connected) throw new Error("wss probe failed");

await srv.close();
`
	if _, err := eng.Run(context.Background(), "ws.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}
