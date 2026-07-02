package main

import (
	"bytes"
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

// "Växjö Båt" encoded as ISO-8859-1 (9 bytes) — invalid UTF-8, so it must be
// reachable as raw bytes (the original lossy `body` mangles å/ä/ö to U+FFFD).
var latin1VaxjoBat = []byte{0x56, 0xE4, 0x78, 0x6A, 0xF6, 0x20, 0x42, 0xE5, 0x74}

func TestHTTPRequest_BodyBytesLatin1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=ISO-8859-1")
		_, _ = w.Write(latin1VaxjoBat)
	}))
	defer srv.Close()
	got := runHTTPReqScript(t, `
		const r = await http.request("GET", `+"`"+srv.URL+"`"+`);
		const __result = r.bodyBytes;
	`)
	bs, ok := got.([]byte)
	if !ok {
		t.Fatalf("r.bodyBytes did not export as []byte: %T", got)
	}
	if !bytes.Equal(bs, latin1VaxjoBat) {
		t.Fatalf("r.bodyBytes = % x, want % x", bs, latin1VaxjoBat)
	}
}

func TestHTTPDo_BodyBytesLatin1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(latin1VaxjoBat)
	}))
	defer srv.Close()
	res, err := httpDo(context.Background(), http.MethodGet, srv.URL, "")
	if err != nil {
		t.Fatal(err)
	}
	bs, ok := res["bodyBytes"].([]byte)
	if !ok {
		t.Fatalf("httpDo bodyBytes not []byte: %T", res["bodyBytes"])
	}
	if !bytes.Equal(bs, latin1VaxjoBat) {
		t.Fatalf("httpDo bodyBytes = % x, want % x", bs, latin1VaxjoBat)
	}
}

func TestBytesFromExported(t *testing.T) {
	if b, err := bytesFromExported("abc"); err != nil || string(b) != "abc" {
		t.Errorf("string: %q %v", b, err)
	}
	if b, err := bytesFromExported([]byte{0, 255, 10}); err != nil || !bytes.Equal(b, []byte{0, 255, 10}) {
		t.Errorf("[]byte: %v %v", b, err)
	}
	ab := goja.New().NewArrayBuffer([]byte{1, 2, 3})
	if b, err := bytesFromExported(ab); err != nil || !bytes.Equal(b, []byte{1, 2, 3}) {
		t.Errorf("arraybuffer: %v %v", b, err)
	}
	if _, err := bytesFromExported(42); err == nil {
		t.Error("int should error")
	}
}

func TestHTTPRequest_BinaryBody(t *testing.T) {
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := runHTTPReqScript(t, `
		const b = new Uint8Array([0, 255, 10, 13, 127, 128]);
		const r = await http.request("POST", `+"`"+srv.URL+"`"+`, { body: b });
		const __result = String(r.status);
	`)
	if res != "200" {
		t.Errorf("status: %v", res)
	}
	if !bytes.Equal(got, []byte{0, 255, 10, 13, 127, 128}) {
		t.Errorf("server received bytes: %v", got)
	}
}
