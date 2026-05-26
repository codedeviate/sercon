package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func wsEchoServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		ctx := r.Context()
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			if err := c.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func runWSSScript(t *testing.T, body string) any {
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
	if _, err := eng.Run(context.Background(), "w.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestNetWSS_HandshakeAndPing(t *testing.T) {
	url := wsEchoServer(t)
	got := runWSSScript(t, `
		const r = await net.wss("`+url+`");
		const __result = [r.connected, r.status, r.pingMs >= 0].join(",");
	`)
	if got != "true,101,true" {
		t.Errorf("handshake+ping: %v (want true,101,true)", got)
	}
}

func TestNetWSS_NoPing(t *testing.T) {
	url := wsEchoServer(t)
	got := runWSSScript(t, `
		const r = await net.wss("`+url+`", { ping: false });
		const __result = [r.connected, r.pingMs].join(",");
	`)
	if got != "true,-1" {
		t.Errorf("no-ping: %v (want true,-1)", got)
	}
}

func TestNetWSS_BadURLThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return netNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await net.wss("ws://127.0.0.1:1/", { timeout: 500 });`)
	if err == nil {
		t.Fatal("expected handshake error")
	}
	if !strings.Contains(err.Error(), "net.wss") {
		t.Errorf("expected net.wss prefix; got %v", err)
	}
}

func TestNetWSS_EmptyURLThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return netNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await net.wss("");`); err == nil {
		t.Error("empty url should throw")
	}
}
