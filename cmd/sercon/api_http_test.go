package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func runHTTPReqScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("http", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{"request": scriptengine.PromisifyAsync(vm, loop, httpRequestCall)}
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
	if _, err := eng.Run(context.Background(), "h.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestHTTPRequest_GetStatusHeadersBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "abc")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	}))
	defer srv.Close()
	got := runHTTPReqScript(t, `
		const r = await http.request("GET", `+"`"+srv.URL+"`"+`);
		const __result = [r.status, r.ok, r.headers["x-custom"], r.body].join(",");
	`)
	if got != "418,false,abc,hi" {
		t.Errorf("get: %v", got)
	}
}

func TestHTTPRequest_PostBodyHeadersAuth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		b, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Echo-Auth", u+":"+p)
		w.Header().Set("X-Echo-Hdr", r.Header.Get("X-Sent"))
		_, _ = w.Write(b)
	}))
	defer srv.Close()
	got := runHTTPReqScript(t, `
		const r = await http.request("POST", `+"`"+srv.URL+"`"+`, {
			body: "payload", headers: { "X-Sent": "v" },
			username: "user", password: "pass",
		});
		const __result = [r.body, r.headers["x-echo-auth"], r.headers["x-echo-hdr"]].join(",");
	`)
	if got != "payload,user:pass,v" {
		t.Errorf("post: %v", got)
	}
}

func TestHTTPRequest_4xxDoesNotThrow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()
	got := runHTTPReqScript(t, `
		const r = await http.request("GET", `+"`"+srv.URL+"`"+`);
		const __result = [r.status, r.ok].join(",");
	`)
	if got != "404,false" {
		t.Errorf("4xx: %v", got)
	}
}

func TestHTTPRequest_RetryOn5xx(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	got := runHTTPReqScript(t, `
		const r = await http.request("GET", `+"`"+srv.URL+"`"+`, { retry: 3 });
		const __result = [r.status, r.body].join(",");
	`)
	if got != "200,ok" {
		t.Errorf("retry: %v (hits=%d)", got, atomic.LoadInt32(&hits))
	}
	if atomic.LoadInt32(&hits) != 3 {
		t.Errorf("expected 3 hits, got %d", hits)
	}
}

func TestHTTPRequest_NoFollow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/from" {
			http.Redirect(w, r, "/to", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("landed"))
	}))
	defer srv.Close()
	got := runHTTPReqScript(t, `
		const r = await http.request("GET", `+"`"+srv.URL+"/from`"+`, { follow: false });
		const __result = r.status;
	`)
	if got != int64(302) {
		t.Errorf("no-follow: %v (want 302)", got)
	}
}

func TestHTTPRequest_TransportErrorThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("http", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{"request": scriptengine.PromisifyAsync(vm, loop, httpRequestCall)}
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await http.request("GET", "http://127.0.0.1:1/");`)
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !strings.Contains(err.Error(), "http.request") {
		t.Errorf("expected http.request prefix; got %v", err)
	}
}

func TestHTTPRequest_InputValidation(t *testing.T) {
	if !strings.Contains(mustErr(t, `await http.request("", "http://x");`), "method") {
		t.Error("empty method should mention method")
	}
	if !strings.Contains(mustErr(t, `await http.request("GET", "");`), "url") {
		t.Error("empty url should mention url")
	}
}

func mustErr(t *testing.T, src string) string {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("http", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{"request": scriptengine.PromisifyAsync(vm, loop, httpRequestCall)}
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", src)
	if err == nil {
		t.Fatal("expected error")
	}
	return err.Error()
}
