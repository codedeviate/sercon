package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// fakeMemcached implements just enough of the memcached text protocol
// (set / get / delete) for the binding tests. Real memcached isn't
// available offline and there's no maintained pure-Go in-process
// server, so this minimal stand-in proves the wire round-trip.
func fakeMemcached(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	var mu sync.Mutex
	store := map[string][]byte{}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				r := bufio.NewReader(c)
				for {
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					f := strings.Fields(strings.TrimSpace(line))
					if len(f) == 0 {
						continue
					}
					switch f[0] {
					case "set": // set <key> <flags> <exp> <bytes>
						n, _ := strconv.Atoi(f[4])
						buf := make([]byte, n+2) // +2 for trailing \r\n
						_, _ = readFull(r, buf)
						mu.Lock()
						store[f[1]] = buf[:n]
						mu.Unlock()
						_, _ = c.Write([]byte("STORED\r\n"))
					case "get", "gets":
						mu.Lock()
						v, ok := store[f[1]]
						mu.Unlock()
						if ok {
							// "gets" wants a trailing CAS unique; harmless on "get".
							_, _ = fmt.Fprintf(c, "VALUE %s 0 %d 1\r\n%s\r\nEND\r\n", f[1], len(v), v)
						} else {
							_, _ = c.Write([]byte("END\r\n"))
						}
					case "delete":
						mu.Lock()
						_, ok := store[f[1]]
						delete(store, f[1])
						mu.Unlock()
						if ok {
							_, _ = c.Write([]byte("DELETED\r\n"))
						} else {
							_, _ = c.Write([]byte("NOT_FOUND\r\n"))
						}
					default:
						_, _ = c.Write([]byte("ERROR\r\n"))
					}
				}
			}(c)
		}
	}()
	return ln.Addr().String()
}

func readFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func runMemcachedScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("memcached", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return memcachedNamespace(vm, loop)
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
	if _, err := eng.Run(context.Background(), "m.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestMemcached_SetGetDelete(t *testing.T) {
	addr := fakeMemcached(t)
	got := runMemcachedScript(t, `
		const m = await memcached.open("`+addr+`");
		await m.set("k", "hello");
		const v = await m.get("k");
		const miss = await m.get("absent");
		const del = await m.delete("k");
		const after = await m.get("k");
		const __result = [v, miss === null, del, after === null].join(",");
	`)
	if got != "hello,true,true,true" {
		t.Errorf("set/get/delete: %v", got)
	}
}

func TestMemcached_DeleteMissReturnsFalse(t *testing.T) {
	addr := fakeMemcached(t)
	got := runMemcachedScript(t, `
		const m = await memcached.open("`+addr+`");
		const __result = await m.delete("never-existed");
	`)
	if got != false {
		t.Errorf("delete miss: %v (want false)", got)
	}
}

func TestMemcached_EmptyAddrThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("memcached", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return memcachedNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await memcached.open("");`); err == nil {
		t.Error("empty addr should throw")
	}
}
