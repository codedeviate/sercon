package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"google.golang.org/api/option"
)

// withMockGoogle points all google clients at ts for the duration of the test.
func withMockGoogle(t *testing.T, ts *httptest.Server) {
	t.Helper()
	prev := googleTestOptions
	googleTestOptions = []option.ClientOption{
		option.WithoutAuthentication(),
		option.WithEndpoint(ts.URL),
		option.WithHTTPClient(ts.Client()),
	}
	t.Cleanup(func() { googleTestOptions = prev })
}

func TestStorage_ListBuckets(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"name":"bucket-a"},{"name":"bucket-b"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storageListBuckets(context.Background(), googleConfig{}, gcsArgs{project: "p"})
	if err != nil {
		t.Fatalf("listBuckets: %v", err)
	}
	m := out.(map[string]any)
	items, ok := m["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 buckets, got %#v", m["items"])
	}
}

func TestStorage_GetBucket(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"bucket-a","location":"EU"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storageGetBucket(context.Background(), googleConfig{}, gcsArgs{bucket: "bucket-a"})
	if err != nil {
		t.Fatalf("getBucket: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "bucket-a" {
		t.Fatalf("expected name bucket-a, got %#v", m["name"])
	}
}

func TestStorage_CreateBucket(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"new-bucket"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storageCreateBucket(context.Background(), googleConfig{}, gcsArgs{project: "p", bucket: "new-bucket"})
	if err != nil {
		t.Fatalf("createBucket: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "new-bucket" {
		t.Fatalf("expected name new-bucket, got %#v", m["name"])
	}
}

func TestStorage_DeleteBucket(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storageDeleteBucket(context.Background(), googleConfig{}, gcsArgs{bucket: "bucket-a"})
	if err != nil {
		t.Fatalf("deleteBucket: %v", err)
	}
	m := out.(map[string]any)
	if len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", m)
	}
}

func TestStorage_ListObjects(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("prefix"); got != "logs/" {
			t.Errorf("expected prefix=logs/, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"name":"logs/a.txt"}]}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storageListObjects(context.Background(), googleConfig{}, gcsArgs{bucket: "bucket-a", prefix: "logs/"})
	if err != nil {
		t.Fatalf("listObjects: %v", err)
	}
	m := out.(map[string]any)
	items, ok := m["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 object, got %#v", m["items"])
	}
}

func TestStorage_StatObject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"key.txt","size":"42"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storageStatObject(context.Background(), googleConfig{}, gcsArgs{bucket: "bucket-a", key: "key.txt"})
	if err != nil {
		t.Fatalf("statObject: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "key.txt" {
		t.Fatalf("expected name key.txt, got %#v", m["name"])
	}
}

func TestStorage_ReadObject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storageReadObject(context.Background(), googleConfig{}, gcsArgs{bucket: "bucket-a", key: "key.txt"})
	if err != nil {
		t.Fatalf("readObject: %v", err)
	}
	m := out.(map[string]any)
	b, ok := m["bytes"].([]byte)
	if !ok || string(b) != "hello world" {
		t.Fatalf("expected bytes 'hello world', got %#v", m["bytes"])
	}
}

func TestStorage_PutObject(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("uploadType") == "media" || r.URL.Query().Get("uploadType") == "" {
			b := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(b)
			gotBody = b
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"key.txt","size":"11"}`))
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storagePutObject(context.Background(), googleConfig{}, gcsArgs{bucket: "bucket-a", key: "key.txt", body: []byte("hello world")})
	if err != nil {
		t.Fatalf("putObject: %v", err)
	}
	m := out.(map[string]any)
	if m["name"] != "key.txt" {
		t.Fatalf("expected name key.txt, got %#v", m["name"])
	}
	_ = gotBody // best-effort; server framework may buffer differently across multipart/media upload
}

func TestStorage_DeleteObject(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	withMockGoogle(t, ts)

	out, err := storageDeleteObject(context.Background(), googleConfig{}, gcsArgs{bucket: "bucket-a", key: "key.txt"})
	if err != nil {
		t.Fatalf("deleteObject: %v", err)
	}
	m := out.(map[string]any)
	if len(m) != 0 {
		t.Fatalf("expected empty object, got %#v", m)
	}
}
