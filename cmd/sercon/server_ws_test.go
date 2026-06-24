package main

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/coder/websocket"

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

// TestServerHTTP_WebSocketCloseCode verifies that a peer-initiated close frame's
// code + reason are captured and surfaced on the socket object as ws.closeCode /
// ws.closeReason once the message iterator ends.
func TestServerHTTP_WebSocketCloseCode(t *testing.T) {
	port := freePort(t)
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        10 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{}, 1)
	got := make(chan [2]string, 1)
	if err := eng.Register("__ready", func() { ready <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__reportClose", func(code int64, reason string) {
		got <- [2]string{strconv.FormatInt(code, 10), reason}
	}); err != nil {
		t.Fatal(err)
	}
	script := `
const srv = await server.http.listen({
  port: ` + strconv.Itoa(port) + `,
  routes: {
    "GET /ws": async (req, res) => {
      const ws = await res.upgradeWebSocket();
      for await (const msg of ws) { /* consume until the peer closes */ }
      __reportClose(ws.closeCode ?? 0, ws.closeReason ?? "");
      await srv.close();
    },
  },
});
__ready();
`
	done := make(chan error, 1)
	go func() { _, err := eng.Run(context.Background(), "ws_close.ts", script); done <- err }()

	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("server never became ready")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(ctx, "ws://127.0.0.1:"+strconv.Itoa(port)+"/ws", nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	// Peer-initiated clean close with a private-use code + reason.
	_ = c.Close(websocket.StatusCode(4321), "go-bye")

	select {
	case v := <-got:
		if v[0] != "4321" || v[1] != "go-bye" {
			t.Fatalf("captured close info = %v, want [4321 go-bye]", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("close info never reported")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not shut down after close")
	}
}
