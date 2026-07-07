package main

import (
	"bufio"
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// An open SSE stream must not stall graceful shutdown for the full timeout.
// net/http.Server.Shutdown waits on non-idle connections, and a streaming
// SSE response is never idle, so the listener's shutdown must actively tear
// down live SSE streams. Here we open a real SSE client, then drive
// GracefulShutdown with a generous ctx and assert it returns well under it.
func TestServerHTTP_SSEDoesNotStallShutdown(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: dir, DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	p := strconv.Itoa(port)
	scriptPath := filepath.Join(dir, "server.ts")
	script := `
const srv = await server.http.listen({
  port: ` + p + `,
  routes: { "GET /events": (req, res) => { res.sse(); /* keep the stream open */ } },
});
await srv.stopped;
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() { _, err := eng.RunFile(context.Background(), scriptPath); runErr <- err }()

	addr := "127.0.0.1:" + p
	// Wait for the listener.
	up := false
	for i := 0; i < 200; i++ {
		if c, err := net.DialTimeout("tcp", addr, 100*time.Millisecond); err == nil {
			_ = c.Close()
			up = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if !up {
		t.Fatal("server never came up")
	}

	// Open an SSE stream and read through the response headers so the stream
	// is genuinely established (pump running, dispatch parked) before we
	// shut down. Leave the connection open.
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("GET /events HTTP/1.1\r\nHost: " + addr + "\r\n\r\n")); err != nil {
		t.Fatal(err)
	}
	br := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("reading SSE response headers: %v", err)
		}
		if strings.TrimSpace(line) == "" { // blank line ends headers
			break
		}
	}
	_ = conn.SetReadDeadline(time.Time{})

	// Drive graceful shutdown with a generous window; it must return well
	// under it because the SSE stream is torn down instead of blocking.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	shutErr := make(chan error, 1)
	start := time.Now()
	go func() { shutErr <- eng.GracefulShutdown(shutdownCtx) }()

	select {
	case err := <-shutErr:
		if err != nil {
			t.Fatalf("GracefulShutdown: %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("GracefulShutdown took %s — the open SSE stream stalled it", elapsed)
		}
	case <-time.After(3500 * time.Millisecond):
		t.Fatal("GracefulShutdown stalled on the open SSE stream")
	}

	select {
	case <-runErr:
	case <-time.After(2 * time.Second):
		t.Fatal("RunFile did not return after shutdown")
	}
}
