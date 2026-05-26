package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// runHTTP drives execHTTP through its real (method, url, opts) signature
// without a goja runtime. Mirrors runShell in api_exec_test.go.
type httpCall struct {
	method string
	url    string
	opts   map[string]any
}

func runHTTP(t *testing.T, c httpCall) (map[string]any, error) {
	t.Helper()
	vm := goja.New()
	args := []goja.Value{vm.ToValue(c.method), vm.ToValue(c.url)}
	if c.opts != nil {
		args = append(args, vm.ToValue(c.opts))
	}
	call := goja.FunctionCall{Arguments: args}
	return execHTTP(context.Background(), call)
}

// skipIfNoBackend skips the test when the named backend isn't on PATH.
// "auto" requires at least one of recon/curl.
func skipIfNoBackend(t *testing.T, name string) {
	t.Helper()
	switch name {
	case "auto":
		if _, e1 := exec.LookPath("recon"); e1 == nil {
			return
		}
		if _, e2 := exec.LookPath("curl"); e2 == nil {
			return
		}
		t.Skip("neither recon nor curl on PATH")
	default:
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s not on PATH", name)
		}
	}
}

// Baseline GET against a local httptest server. We don't care which
// backend ran; we just need status + headers + body to round-trip.
func TestExecHTTP_GetReturnsStatusHeadersBody(t *testing.T) {
	skipIfNoBackend(t, "auto")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Custom", "hello")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, "hi from server")
	}))
	defer srv.Close()

	out, err := runHTTP(t, httpCall{method: "GET", url: srv.URL})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out["status"].(int) != http.StatusTeapot {
		t.Errorf("status: %v", out["status"])
	}
	if out["body"].(string) != "hi from server" {
		t.Errorf("body: %q", out["body"])
	}
	headers := out["headers"].(map[string]string)
	if headers["x-custom"] != "hello" {
		t.Errorf("x-custom header: %q (full headers: %v)", headers["x-custom"], headers)
	}
	if b := out["backend"].(string); b != "recon" && b != "curl" {
		t.Errorf("backend label: %q", b)
	}
}

// POST with a body and custom headers. The handler echoes both back so we
// can confirm stdin-piping and -H flags actually reached the server.
func TestExecHTTP_PostBodyAndHeaders(t *testing.T) {
	skipIfNoBackend(t, "auto")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Method", r.Method)
		w.Header().Set("X-Got-Header", r.Header.Get("X-Sent"))
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	out, err := runHTTP(t, httpCall{
		method: "post",
		url:    srv.URL,
		opts: map[string]any{
			"body":    "payload-bytes",
			"headers": map[string]any{"X-Sent": "abc"},
		},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out["status"].(int) != http.StatusOK {
		t.Errorf("status: %v", out["status"])
	}
	if out["body"].(string) != "payload-bytes" {
		t.Errorf("body: %q", out["body"])
	}
	headers := out["headers"].(map[string]string)
	if headers["x-method"] != "POST" {
		t.Errorf("method-echo: %q", headers["x-method"])
	}
	if headers["x-got-header"] != "abc" {
		t.Errorf("custom header echo: %q", headers["x-got-header"])
	}
}

// 4xx and 5xx are routine HTTP outcomes — they must NOT throw.
func TestExecHTTP_4xxDoesNotThrow(t *testing.T) {
	skipIfNoBackend(t, "auto")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	out, err := runHTTP(t, httpCall{method: "GET", url: srv.URL})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out["status"].(int) != http.StatusNotFound {
		t.Errorf("status: %v", out["status"])
	}
}

// Transport errors — unresolvable host, refused connection — must throw.
// Use a port we know nothing's listening on so we exercise the connect
// path rather than DNS.
func TestExecHTTP_TransportErrorThrows(t *testing.T) {
	skipIfNoBackend(t, "auto")
	_, err := runHTTP(t, httpCall{method: "GET", url: "http://127.0.0.1:1/"})
	if err == nil {
		t.Fatal("expected transport error, got nil")
	}
}

// Context timeout from `opts.timeout` must throw — not a non-zero exit.
func TestExecHTTP_TimeoutThrows(t *testing.T) {
	skipIfNoBackend(t, "auto")
	if runtime.GOOS == "windows" {
		t.Skip("flaky on Windows test runners")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte("late"))
	}))
	defer srv.Close()

	start := time.Now()
	_, err := runHTTP(t, httpCall{
		method: "GET",
		url:    srv.URL,
		opts:   map[string]any{"timeout": int64(200)},
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed > 1500*time.Millisecond {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

// `backend: "curl"` must pick curl even when recon is on PATH. Skipped
// when curl is missing (extremely rare).
func TestExecHTTP_BackendCurlForced(t *testing.T) {
	skipIfNoBackend(t, "curl")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	out, err := runHTTP(t, httpCall{
		method: "GET",
		url:    srv.URL,
		opts:   map[string]any{"backend": "curl"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out["backend"].(string) != "curl" {
		t.Errorf("backend: %q", out["backend"])
	}
}

// `backend: "recon"` must pick recon when present. Skip when recon is
// not on PATH (most CI runners).
func TestExecHTTP_BackendReconForced(t *testing.T) {
	skipIfNoBackend(t, "recon")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()
	out, err := runHTTP(t, httpCall{
		method: "GET",
		url:    srv.URL,
		opts:   map[string]any{"backend": "recon"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out["backend"].(string) != "recon" {
		t.Errorf("backend: %q", out["backend"])
	}
}

// Empty / missing method or URL must throw before we even spawn.
func TestExecHTTP_InputValidation(t *testing.T) {
	if _, err := runHTTP(t, httpCall{method: "", url: "http://x"}); err == nil {
		t.Errorf("empty method should error")
	}
	if _, err := runHTTP(t, httpCall{method: "GET", url: ""}); err == nil {
		t.Errorf("empty url should error")
	}
	if _, err := runHTTP(t, httpCall{
		method: "GET", url: "http://x",
		opts: map[string]any{"backend": "wget"},
	}); err == nil {
		t.Errorf("unknown backend should error")
	}
}

// Header parser: chained redirect blocks must resolve to the final 200,
// not the intermediate 301. Both backends concatenate blocks when -L is
// active.
func TestParseHeaderFile_RedirectChain(t *testing.T) {
	raw := []byte(strings.Join([]string{
		"HTTP/1.1 301 Moved\r\n",
		"Location: /elsewhere\r\n",
		"\r\n",
		"HTTP/1.1 200 OK\r\n",
		"Content-Type: text/plain\r\n",
		"\r\n",
	}, ""))
	status, headers, err := parseHeaderFile(raw)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if status != 200 {
		t.Errorf("status: %d (want 200)", status)
	}
	if headers["content-type"] != "text/plain" {
		t.Errorf("content-type: %q", headers["content-type"])
	}
}

// Recon's `-D` output prefixes the status line with an extra `HTTP/`
// token. Parser must look for the first 3-digit field rather than
// indexing positionally.
func TestParseStatusCode_ReconQuirkyStatusLine(t *testing.T) {
	got, err := parseStatusCode("HTTP/HTTP/2.0 418 I'm a teapot")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got != 418 {
		t.Errorf("status: %d", got)
	}
}

// Status line with no usable code must surface a clear error.
func TestParseStatusCode_NoCode(t *testing.T) {
	_, err := parseStatusCode("garbage with no number")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(fmt.Sprint(err), "no status code") {
		t.Errorf("error wording: %v", err)
	}
}
