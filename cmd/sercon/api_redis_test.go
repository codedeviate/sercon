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

func runRedisScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("redis", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return redisNamespace(vm, loop)
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
	if _, err := eng.Run(context.Background(), "r.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestRedis_SetGetDelPing(t *testing.T) {
	mr := miniredis.RunT(t)
	url := "redis://" + mr.Addr()
	got := runRedisScript(t, `
		const r = await redis.open("`+url+`");
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

func TestRedis_ListAndHash(t *testing.T) {
	mr := miniredis.RunT(t)
	url := "redis://" + mr.Addr()
	got := runRedisScript(t, `
		const r = await redis.open("`+url+`");
		await r.do("RPUSH", "list", "a", "b", "c");
		const items = await r.do("LRANGE", "list", "0", "-1");
		await r.do("HSET", "h", "f1", "v1");
		const hv = await r.do("HGET", "h", "f1");
		await r.close();
		const __result = [items.join("+"), hv].join("|");
	`)
	if got != "a+b+c|v1" {
		t.Errorf("list/hash: %v", got)
	}
}

func TestRedis_BadURLThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("redis", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return redisNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await redis.open("not-a-url");`); err == nil {
		t.Error("bad url should throw")
	}
}

func TestRedis_PingFailsOnDeadServer(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("redis", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return redisNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await redis.open("redis://127.0.0.1:1?dial_timeout=0.5");`)
	if err == nil {
		t.Fatal("expected ping failure")
	}
	if !strings.Contains(err.Error(), "redis.open") {
		t.Errorf("expected redis.open prefix; got %v", err)
	}
}
