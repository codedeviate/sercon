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
