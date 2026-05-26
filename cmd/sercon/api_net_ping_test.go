package main

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func runPingScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return netNamespace(vm, loop)
	}); err != nil {
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
	if _, err := eng.Run(context.Background(), "p.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// TCP ping against a localhost listener: all packets received, 0% loss.
func TestNetPing_TCPReachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	got := runPingScript(t, `
		const r = await net.ping("127.0.0.1", { mode: "tcp", port: "`+port+`", count: 3 });
		const __result = [r.sent, r.received, r.lossPercent, r.mode].join(",");
	`)
	if got != "3,3,0,tcp" {
		t.Errorf("reachable: %v (want 3,3,0,tcp)", got)
	}
}

// TCP ping to a closed port: received 0, loss 100%, no throw.
func TestNetPing_TCPUnreachable(t *testing.T) {
	got := runPingScript(t, `
		const r = await net.ping("127.0.0.1", { mode: "tcp", port: "1", count: 2, timeout: 500 });
		const __result = [r.received, r.lossPercent].join(",");
	`)
	if got != "0,100" {
		t.Errorf("unreachable: %v (want 0,100)", got)
	}
}

// Bad hostname throws (resolve failure), not a silent 100% loss.
func TestNetPing_BadHostThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 5 * time.Second})
	if err := eng.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return netNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts",
		`await net.ping("no-such-host.invalid", { mode: "tcp", count: 1, timeout: 500 });`)
	if err == nil {
		t.Fatal("expected resolve error")
	}
}

// Unknown mode and missing host throw.
func TestNetPing_Validation(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return netNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await net.ping("x", { mode: "udp" });`); err == nil {
		t.Error("unknown mode should throw")
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await net.ping("");`); err == nil {
		t.Error("empty host should throw")
	}
}
