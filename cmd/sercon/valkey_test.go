package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func runValkeyScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("valkey", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return valkeyNamespace(vm, loop)
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
	if _, err := eng.Run(context.Background(), "v.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// miniredis speaks RESP, so it stands in for a Valkey server. This also
// exercises the valkey:// scheme normalisation (valkey://addr -> redis://addr).
func TestValkey_SetGetDelPing(t *testing.T) {
	mr := miniredis.RunT(t)
	url := "valkey://" + mr.Addr()
	got := runValkeyScript(t, `
		const r = await valkey.open("`+url+`");
		const pong = await r.ping();
		await r.do("SET", "greeting", "hello");
		const val = await r.do("GET", "greeting");
		const missing = await r.do("GET", "nope");
		await r.do("DEL", "greeting");
		const after = await r.do("GET", "greeting");
		await r.close();
		const __result = [pong, val, missing === null, after === null].join(",");
	`)
	if got != "PONG,hello,true,true" {
		t.Errorf("set/get/del: %v", got)
	}
}

// A plain redis:// URL must also work (the scheme normaliser passes it through).
func TestValkey_AcceptsRedisScheme(t *testing.T) {
	mr := miniredis.RunT(t)
	url := "redis://" + mr.Addr()
	got := runValkeyScript(t, `
		const r = await valkey.open("`+url+`");
		await r.do("SET", "k", "v");
		const v = await r.do("GET", "k");
		await r.close();
		const __result = v;
	`)
	if got != "v" {
		t.Errorf("redis:// scheme via valkey.open: %v", got)
	}
}

func TestValkey_BadURLThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("valkey", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return valkeyNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await valkey.open("not-a-url");`); err == nil {
		t.Error("bad url should throw")
	}
}

func TestValkey_PingFailsOnDeadServer(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("valkey", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return valkeyNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await valkey.open("valkey://127.0.0.1:1?dial_timeout=0.5");`)
	if err == nil {
		t.Fatal("expected ping failure")
	}
	if !strings.Contains(err.Error(), "valkey.open") {
		t.Errorf("expected valkey.open prefix; got %v", err)
	}
}

func TestNormalizeValkeyURL(t *testing.T) {
	cases := map[string]string{
		"valkey://h:6379/0":  "redis://h:6379/0",
		"valkeys://h:6379/0": "rediss://h:6379/0",
		"redis://h:6379/0":   "redis://h:6379/0",
		"rediss://h:6379/0":  "rediss://h:6379/0",
		"not-a-url":          "not-a-url",
	}
	for in, want := range cases {
		if got := normalizeValkeyURL(in); got != want {
			t.Errorf("normalizeValkeyURL(%q) = %q, want %q", in, got, want)
		}
	}
}
