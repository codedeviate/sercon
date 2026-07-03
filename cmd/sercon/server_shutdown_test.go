package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestServerHTTP_GracefulShutdown starts an HTTP listener via the engine on
// a random port with a script that stays parked on srv.stopped, then drives
// eng.GracefulShutdown from a separate goroutine — exactly as `sercon serve`
// does on SIGTERM/SIGINT. It asserts that GracefulShutdown closes the
// listener, that RunFile returns well within the shutdown window (the script
// never calls close() itself), and that the port stops accepting.
func TestServerHTTP_GracefulShutdown(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	// No per-script Timeout: the server stays up until GracefulShutdown.
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     dir,
		DisableConsole: true,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}

	p := strconv.Itoa(port)
	scriptPath := filepath.Join(dir, "server.ts")
	script := `
const srv = await server.http.listen({
  port: ` + p + `,
  routes: { "GET /": (req, res) => res.text("ok") },
});
// Park until the listener is closed out from under us (by GracefulShutdown).
await srv.stopped;
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.RunFile(context.Background(), scriptPath)
		runErr <- err
	}()

	// Wait for the listener to come up.
	addr := "127.0.0.1:" + p
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

	// Drive graceful shutdown from a separate goroutine, as the serve signal
	// handler does. Generous 5s window; assert everything returns well under.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	shutErr := make(chan error, 1)
	go func() { shutErr <- eng.GracefulShutdown(shutdownCtx) }()

	select {
	case err := <-shutErr:
		if err != nil {
			t.Fatalf("GracefulShutdown: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("GracefulShutdown did not return within 3s (well under the 5s window)")
	}

	// With the listener closed, RunFile must return promptly.
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("RunFile returned an error after graceful shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("RunFile did not return after GracefulShutdown")
	}

	// The port must no longer accept connections.
	if c, err := net.DialTimeout("tcp", addr, 300*time.Millisecond); err == nil {
		_ = c.Close()
		t.Fatal("port still accepting connections after graceful shutdown")
	}
}
