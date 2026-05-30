package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// cookieServer: /login sets a session cookie; /whoami echoes it back
// (or "anonymous" if absent); /ua echoes the User-Agent.
func cookieServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc123", Path: "/"})
			_, _ = w.Write([]byte("logged in"))
		case "/whoami":
			c, err := r.Cookie("session")
			if err != nil {
				_, _ = w.Write([]byte("anonymous"))
				return
			}
			_, _ = w.Write([]byte("session=" + c.Value))
		case "/ua":
			_, _ = w.Write([]byte(r.Header.Get("User-Agent")))
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func runBrowserScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("browser", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return browserNamespace(vm, loop)
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
	if _, err := eng.Run(context.Background(), "b.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// The cookie jar persists across requests: login sets a cookie, the
// next request replays it automatically.
func TestBrowser_CookieJarPersists(t *testing.T) {
	url := cookieServer(t)
	got := runBrowserScript(t, `
		const b = await browser.open();
		const before = await b.get("`+url+`/whoami");
		await b.get("`+url+`/login");
		const after = await b.get("`+url+`/whoami");
		const __result = [before.body, after.body].join("|");
	`)
	if got != "anonymous|session=abc123" {
		t.Errorf("cookie persistence: %v", got)
	}
}

// setUserAgent / setHeader are replayed on every request.
func TestBrowser_HeadersReplayed(t *testing.T) {
	url := cookieServer(t)
	got := runBrowserScript(t, `
		const b = await browser.open();
		b.setUserAgent("sercon-test/1.0");
		const r = await b.get("`+url+`/ua");
		const __result = r.body;
	`)
	if got != "sercon-test/1.0" {
		t.Errorf("UA replay: %v", got)
	}
}

// cookies() inspects the jar for a URL.
func TestBrowser_CookiesInspection(t *testing.T) {
	url := cookieServer(t)
	got := runBrowserScript(t, `
		const b = await browser.open();
		await b.get("`+url+`/login");
		const cookies = await b.cookies("`+url+`/");
		const __result = cookies.map(c => c.name + "=" + c.value).join(",");
	`)
	if got != "session=abc123" {
		t.Errorf("cookies inspect: %v", got)
	}
}

// Empty URL throws.
func TestBrowser_EmptyURLThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("browser", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return browserNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `const b = await browser.open(); await b.get("");`)
	if err == nil {
		t.Fatal("empty url should throw")
	}
	if !strings.Contains(err.Error(), "url required") {
		t.Errorf("expected url-required; got %v", err)
	}
}
