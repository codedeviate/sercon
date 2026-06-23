package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebFetch_DefaultUserAgentAndStatus(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		if r.URL.Path == "/missing" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	// Default UA injected, 2xx body returned.
	body, _, err := loadBytes(context.Background(), srv.URL+"/", nil)
	if err != nil {
		t.Fatalf("loadBytes: %v", err)
	}
	if string(body) != "hello" {
		t.Fatalf("body = %q, want hello", body)
	}
	if gotUA != defaultWebUserAgent {
		t.Fatalf("UA = %q, want %q", gotUA, defaultWebUserAgent)
	}

	// Caller-supplied userAgent overrides the default.
	_, _, _ = loadBytes(context.Background(), srv.URL+"/", map[string]any{"userAgent": "my-bot/1.0"})
	if gotUA != "my-bot/1.0" {
		t.Fatalf("override UA = %q, want my-bot/1.0", gotUA)
	}

	// Non-2xx throws (returns error).
	if _, _, err := loadBytes(context.Background(), srv.URL+"/missing", nil); err == nil {
		t.Fatalf("expected error on 404, got nil")
	}
}
