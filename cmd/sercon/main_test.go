package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// An oversized response body must not be read unbounded into memory: httpDo
// backs net.http.get/post/request, and a misbehaving or hostile endpoint
// could otherwise OOM the process via a huge or slow-drip response.
func TestHTTPDo_CapsOversizedResponseBody(t *testing.T) {
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

	_, err := httpDo(context.Background(), http.MethodGet, srv.URL, "")
	if err == nil {
		t.Fatal("expected an error for an over-cap response body, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maxBytes limit") {
		t.Fatalf("expected a maxBytes-limit error, got: %v", err)
	}
}
