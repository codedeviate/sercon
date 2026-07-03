package main

import (
	"context"
	"strconv"
	"sync"
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

// TestServerHTTP_WebSocketUpgradeOrdering guards against the upgrade
// hijack race (#13): dispatchHandler's <-state.notify must not unblock
// (via markFinal) until websocket.Accept has completed its hijack of
// the connection. If markFinal fires first, the dispatcher goroutine
// can return from the handler — and the stdlib http.Server can write to
// or finalize the not-yet-hijacked connection — concurrently with
// Accept still hijacking it, corrupting the connection.
//
// A real network race is timing-dependent and unreliable to force
// deterministically in a unit test (Accept on localhost is fast), so
// this asserts the ORDERING directly via the wsUpgradeOrderHook test
// seam in server_ws.go: it records which of "accept" / "markFinal"
// fires first and requires "accept" to come first. Pre-fix, the
// production code called markFinal before Accept, so this test fails
// against that code (recorded order ["markFinal", "accept"]).
func TestServerHTTP_WebSocketUpgradeOrdering(t *testing.T) {
	var (
		mu     sync.Mutex
		events []string
	)
	wsUpgradeOrderHook = func(event string) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	}
	t.Cleanup(func() { wsUpgradeOrderHook = nil })

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
      for await (const msg of ws) { /* consume until the peer closes */ }
    },
  },
});

const probe = await net.probe.wss("ws://127.0.0.1:` + strconv.Itoa(port) + `/ws");
if (!probe.connected) throw new Error("wss probe failed");

await srv.close();
`
	if _, err := eng.Run(context.Background(), "ws_order.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), events...)
	mu.Unlock()
	want := []string{"accept", "markFinal"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("upgrade event order = %v, want %v (Accept must complete before markFinal signals the dispatcher)", got, want)
	}
}

// TestServerHTTP_WebSocketMessageRoundTrip exercises the full upgrade +
// message path end-to-end: dial, exchange a text frame both ways, then
// close cleanly. Run under `go test -race` this also guards against data
// races introduced by the upgrade-ordering fix (e.g. touching the
// hijacked connection or responseState from the wrong goroutine).
func TestServerHTTP_WebSocketMessageRoundTrip(t *testing.T) {
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
	if err := eng.Register("__ready", func() { ready <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	script := `
const srv = await server.http.listen({
  port: ` + strconv.Itoa(port) + `,
  routes: {
    "GET /ws": async (req, res) => {
      const ws = await res.upgradeWebSocket();
      for await (const msg of ws) {
        if (msg.type === "text" && msg.text === "ping") {
          await ws.send("pong");
        }
        if (msg.type === "text" && msg.text === "bye") {
          await ws.close(1000, "done");
          break;
        }
      }
      await srv.close();
    },
  },
});
__ready();
`
	done := make(chan error, 1)
	go func() { _, err := eng.Run(context.Background(), "ws_roundtrip.ts", script); done <- err }()

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

	if err := c.Write(ctx, websocket.MessageText, []byte("ping")); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	mt, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read pong: %v", err)
	}
	if mt != websocket.MessageText || string(data) != "pong" {
		t.Fatalf("got (%v, %q), want (Text, %q)", mt, data, "pong")
	}

	if err := c.Write(ctx, websocket.MessageText, []byte("bye")); err != nil {
		t.Fatalf("write bye: %v", err)
	}
	_, _, err = c.Read(ctx)
	if err == nil {
		t.Fatal("expected read after server close to fail with a close error")
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
