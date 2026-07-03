package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/icmp"

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

// TestServerTCP_AcceptErrorReleasesHoldAndCloses guards the fix for #14: the
// accept loop used to treat ANY Accept() error as "closed via srv.close()"
// and just return, without calling doClose() — so a genuine (non-close())
// accept error left the HoldRun sentinel outstanding forever and the
// listener's fd leaked. This closes the raw net.Listener out from under the
// accept loop (via a test-only hook, bypassing srv.close()/doClose()
// entirely) to simulate exactly that: a fatal Accept() error that did NOT
// originate from the explicit-close path. Pre-fix this hangs (RunFile never
// returns, because the sentinel is only cleared by doClose()); post-fix the
// accept loop's error exit calls doClose() itself and RunFile returns
// promptly.
func TestServerTCP_AcceptErrorReleasesHoldAndCloses(t *testing.T) {
	var (
		lnMu sync.Mutex
		ln   net.Listener
	)
	tcpListenerHookForTest = func(l net.Listener) {
		lnMu.Lock()
		ln = l
		lnMu.Unlock()
	}
	defer func() { tcpListenerHookForTest = nil }()

	dir := t.TempDir()
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     dir,
		DisableConsole: true,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "server.ts")
	script := `
const srv = server.tcp.listen({ port: 0 }, conn => {});
// Deliberately never call srv.close(). Park forever so the only way this
// Run can end is via the accept loop's own error-exit path.
await new Promise(() => {});
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.RunFile(context.Background(), scriptPath)
		runErr <- err
	}()

	var captured net.Listener
	for i := 0; i < 200; i++ {
		lnMu.Lock()
		captured = ln
		lnMu.Unlock()
		if captured != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if captured == nil {
		t.Fatal("accept loop's listener was never captured by the test hook")
	}

	// Close the raw fd directly — NOT via srv.close()/doClose(). The next
	// Accept() call fails with a genuine, non-close()-path error.
	if err := captured.Close(); err != nil {
		t.Fatalf("closing listener out from under the accept loop: %v", err)
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("RunFile returned an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunFile did not return after a non-close() accept error; the HoldRun sentinel leaked")
	}
}

// TestServerUDP_ReadErrorReleasesHoldAndCloses is the UDP analogue of
// TestServerTCP_AcceptErrorReleasesHoldAndCloses: closes the raw *net.UDPConn
// out from under the read loop (bypassing srv.close()) and asserts the Run
// still ends promptly.
func TestServerUDP_ReadErrorReleasesHoldAndCloses(t *testing.T) {
	var (
		connMu sync.Mutex
		conn   *net.UDPConn
	)
	udpConnHookForTest = func(c *net.UDPConn) {
		connMu.Lock()
		conn = c
		connMu.Unlock()
	}
	defer func() { udpConnHookForTest = nil }()

	dir := t.TempDir()
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     dir,
		DisableConsole: true,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "server.ts")
	script := `
const srv = server.udp.listen({ port: 0 }, (msg, reply) => {});
await new Promise(() => {});
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.RunFile(context.Background(), scriptPath)
		runErr <- err
	}()

	var captured *net.UDPConn
	for i := 0; i < 200; i++ {
		connMu.Lock()
		captured = conn
		connMu.Unlock()
		if captured != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if captured == nil {
		t.Fatal("read loop's conn was never captured by the test hook")
	}

	if err := captured.Close(); err != nil {
		t.Fatalf("closing conn out from under the read loop: %v", err)
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("RunFile returned an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunFile did not return after a non-close() read error; the HoldRun sentinel leaked")
	}
}

// TestServerICMP_ReadErrorReleasesHoldAndCloses is the ICMP analogue. Raw
// ICMP needs root / CAP_NET_RAW, so this skips otherwise (same guard as
// TestICMPServer_LoopbackReceive).
func TestServerICMP_ReadErrorReleasesHoldAndCloses(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	var (
		connMu sync.Mutex
		conn   *icmp.PacketConn
	)
	icmpConnHookForTest = func(c *icmp.PacketConn) {
		connMu.Lock()
		conn = c
		connMu.Unlock()
	}
	defer func() { icmpConnHookForTest = nil }()

	dir := t.TempDir()
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     dir,
		DisableConsole: true,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "server.ts")
	script := `
const srv = server.icmp.listen({}, (msg, reply) => {});
await new Promise(() => {});
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.RunFile(context.Background(), scriptPath)
		runErr <- err
	}()

	var captured *icmp.PacketConn
	for i := 0; i < 200; i++ {
		connMu.Lock()
		captured = conn
		connMu.Unlock()
		if captured != nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if captured == nil {
		t.Fatal("read loop's conn was never captured by the test hook")
	}

	if err := captured.Close(); err != nil {
		t.Fatalf("closing conn out from under the read loop: %v", err)
	}

	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("RunFile returned an error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunFile did not return after a non-close() read error; the HoldRun sentinel leaked")
	}
}
