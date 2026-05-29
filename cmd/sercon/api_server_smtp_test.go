package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestServerSMTP_ListenAndClose exercises the basic listener lifecycle:
// bind a port, immediately close, verify Run returns cleanly. The
// round-trip test that actually delivers a message lives in
// api_server_smtp_roundtrip_test.go (Task 2, after net.email.send exists).
func TestServerSMTP_ListenAndClose(t *testing.T) {
	port := freePort(t)
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        5 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	script := `
const srv = await server.smtp.listen({
  port: ` + strconv.Itoa(port) + `,
  hostname: "test.local",
  handlers: {
    onMail: () => true,
    onRcpt: () => true,
    onData: () => true,
  },
});
await srv.close();
`
	if _, err := eng.Run(context.Background(), "smtp.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestServerSMTP_AsyncHandlerRejects proves an async (Promise-returning)
// handler that resolves false actually rejects the stage — the regression
// guard for the bug where a returned Promise was treated as a truthy accept.
func TestServerSMTP_AsyncHandlerRejects(t *testing.T) {
	port := freePort(t)
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        15 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	script := `
const srv = await server.smtp.listen({
  port: ` + strconv.Itoa(port) + `,
  hostname: "test.local",
  handlers: {
    onMail: async (env) => false,   // async reject
    onRcpt: () => true,
    onData: () => true,
  },
});
await srv.stopped;   // block until the engine context is cancelled
`
	runErrCh := make(chan error, 1)
	go func() {
		_, err := eng.Run(ctx, "async.ts", script)
		runErrCh <- err
	}()

	addr := "127.0.0.1:" + strconv.Itoa(port)
	var conn net.Conn
	var derr error
	for i := 0; i < 60; i++ {
		conn, derr = net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if derr == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if derr != nil {
		t.Fatalf("dial: %v", derr)
	}
	defer conn.Close()

	br := bufio.NewReader(conn)
	readLine := func() string {
		line, _ := br.ReadString('\n')
		return strings.TrimRight(line, "\r\n")
	}
	readLine() // 220 greeting
	fmt.Fprintf(conn, "EHLO test\r\n")
	for { // drain EHLO multiline reply: last line is "250 ..." (space at idx 3)
		l := readLine()
		if len(l) >= 4 && l[3] == ' ' {
			break
		}
		if l == "" {
			break
		}
	}
	fmt.Fprintf(conn, "MAIL FROM:<a@b.com>\r\n")
	resp := readLine()
	if !strings.HasPrefix(resp, "5") {
		t.Fatalf("expected 5xx rejection for async-false onMail, got: %q", resp)
	}
	fmt.Fprintf(conn, "QUIT\r\n")

	cancel()
	select {
	case <-runErrCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}
