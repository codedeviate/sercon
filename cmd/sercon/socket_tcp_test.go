package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runSocketScript builds an engine with the full reserved-global surface
// (so net.tcp/udp/icmp are present), registers __capture, runs the script
// body, and returns the captured value. Modeled on runHTTPReqScript.
func runSocketScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "s.ts", body); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestTCP_ConnectWriteOnDataClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() { // echo once, then close
		c, err := ln.Accept()
		if err != nil {
			return
		}
		buf := make([]byte, 64)
		n, _ := c.Read(buf)
		_, _ = c.Write(buf[:n])
		_ = c.Close()
	}()

	host, port, _ := net.SplitHostPort(ln.Addr().String())
	got := runSocketScript(t, fmt.Sprintf(`
		const c = await net.tcp.connect(%q, %s);
		const chunks = [];
		c.onData(ev => { chunks.push(ev.text); });
		await new Promise(res => { c.onClose(() => res()); c.write("ping"); });
		__capture(chunks.join(""));
	`, host, port))
	if got != "ping" {
		t.Errorf("tcp echo: got %q want %q", got, "ping")
	}
}

func TestTCP_ConnectRefusedRejects(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 5 * time.Second})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await net.tcp.connect("127.0.0.1", "1");`)
	if err == nil {
		t.Fatal("expected connect to reject")
	}
	if !strings.Contains(err.Error(), "net.tcp.connect") {
		t.Errorf("expected net.tcp.connect in error; got %v", err)
	}
}
