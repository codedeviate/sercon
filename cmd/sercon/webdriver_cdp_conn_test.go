package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// echoCDP is a minimal CDP-shaped WS server: for each request it emits a stray
// event (no id, must be ignored) then a {id,result} echoing method+sessionId.
func echoCDP(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close(websocket.StatusNormalClosure, "")
		ctx := context.Background()
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var msg struct {
				ID        int    `json:"id"`
				Method    string `json:"method"`
				SessionID string `json:"sessionId"`
			}
			_ = json.Unmarshal(data, &msg)
			_ = c.Write(ctx, websocket.MessageText, []byte(`{"method":"Some.event","params":{}}`))
			reply := fmt.Sprintf(`{"id":%d,"result":{"echoMethod":%q,"echoSession":%q}}`, msg.ID, msg.Method, msg.SessionID)
			_ = c.Write(ctx, websocket.MessageText, []byte(reply))
		}
	}))
	return srv, "ws" + strings.TrimPrefix(srv.URL, "http")
}

func TestCDPConn_CallCorrelatesAndRoutes(t *testing.T) {
	srv, wsURL := echoCDP(t)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, err := dialCDP(ctx, wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer c.close()

	r1, err := c.callMap("SESS1", "Foo.bar", map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("call1: %v", err)
	}
	if r1["echoMethod"] != "Foo.bar" || r1["echoSession"] != "SESS1" {
		t.Fatalf("call1 wrong echo: %v", r1)
	}
	r2, err := c.callMap("", "Baz.qux", nil)
	if err != nil {
		t.Fatalf("call2: %v", err)
	}
	if r2["echoMethod"] != "Baz.qux" || r2["echoSession"] != "" {
		t.Fatalf("call2 wrong echo: %v", r2)
	}
}

func TestCDPConn_CloseFailsPending(t *testing.T) {
	srv, wsURL := echoCDP(t)
	defer srv.Close()
	c, err := dialCDP(context.Background(), wsURL)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	c.close()
	if _, err := c.callMap("", "Foo.bar", nil); err == nil {
		t.Fatal("call after close should error")
	}
}

func TestBrowserWSURLFromCaps(t *testing.T) {
	// se:cdp takes precedence and is returned verbatim.
	s := &wdSession{}
	if got, err := s.browserWSURLFromCaps(map[string]any{"se:cdp": "ws://grid/cdp"}); err != nil || got != "ws://grid/cdp" {
		t.Fatalf("se:cdp: got %q err %v", got, err)
	}
	// neither se:cdp nor debuggerAddress → error.
	if _, err := s.browserWSURLFromCaps(map[string]any{}); err == nil {
		t.Fatal("expected error when no CDP endpoint is advertised")
	}
	// debuggerAddress → resolved via /json/version.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://browser/devtools/browser/abc"}`))
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")
	caps := map[string]any{"goog:chromeOptions": map[string]any{"debuggerAddress": addr}}
	if got, err := s.browserWSURLFromCaps(caps); err != nil || got != "ws://browser/devtools/browser/abc" {
		t.Fatalf("debuggerAddress: got %q err %v", got, err)
	}
}

// TestFetchBrowserWSURL_CapsOversizedResponse verifies that a /json/version
// response body over DefaultMaxHTTPBodyBytes surfaces the readAllCapped
// size-limit error instead of silently proceeding with truncated bytes (the
// original bug: the error return was discarded with `body, _ := ...`).
func TestFetchBrowserWSURL_CapsOversizedResponse(t *testing.T) {
	const chunkSize = 1 << 20 // 1 MB
	chunk := make([]byte, chunkSize)
	over := int64(DefaultMaxHTTPBodyBytes) + chunkSize // safely over the cap

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		written := int64(0)
		for written < over {
			n, err := w.Write(chunk)
			if err != nil {
				return
			}
			written += int64(n)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	addr := strings.TrimPrefix(srv.URL, "http://")
	_, err := fetchBrowserWSURL(addr)
	if err == nil {
		t.Fatal("expected an error for an over-cap CDP response, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maxBytes limit") {
		t.Fatalf("expected a maxBytes-limit error, got: %v", err)
	}
}
