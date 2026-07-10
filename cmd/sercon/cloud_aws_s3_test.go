package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// withMockAWS points every AWS client at ts (static creds, path-style) for the test.
func withMockAWS(t *testing.T, ts *httptest.Server) {
	t.Helper()
	prev := awsTestOptions
	awsTestOptions = &awsTestSeam{endpoint: ts.URL, httpClient: ts.Client()}
	t.Cleanup(func() { awsTestOptions = prev })
}

func TestAWSS3_ListBuckets(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><ListAllMyBucketsResult><Buckets><Bucket><Name>bucket-a</Name></Bucket><Bucket><Name>bucket-b</Name></Bucket></Buckets></ListAllMyBucketsResult>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3ListBuckets(context.Background(), awsConfig{}, awsS3Args{})
	if err != nil {
		t.Fatalf("listBuckets: %v", err)
	}
	m := out.(map[string]any)
	buckets, ok := m["Buckets"].([]any)
	if !ok || len(buckets) != 2 {
		t.Fatalf("expected 2 buckets, got %#v", m["Buckets"])
	}
}

func TestAWSS3_CreateBucket(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Location", "/my-bucket")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3CreateBucket(context.Background(), awsConfig{}, awsS3Args{bucket: "my-bucket"})
	if err != nil {
		t.Fatalf("createBucket: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/my-bucket" {
		t.Fatalf("expected path-style request to /my-bucket, got %s", gotPath)
	}
	m := out.(map[string]any)
	if m["Location"] != "/my-bucket" {
		t.Fatalf("expected Location /my-bucket, got %#v", m["Location"])
	}
}

func TestAWSS3_DeleteBucket(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3DeleteBucket(context.Background(), awsConfig{}, awsS3Args{bucket: "my-bucket"})
	if err != nil {
		t.Fatalf("deleteBucket: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/my-bucket" {
		t.Fatalf("expected path-style request to /my-bucket, got %s", gotPath)
	}
	if m, ok := out.(map[string]any); !ok || len(m) != 0 {
		t.Fatalf("expected empty {}, got %#v", out)
	}
}

func TestAWSS3_ListObjects(t *testing.T) {
	var gotPath, gotPrefix string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotPrefix = r.URL.Query().Get("prefix")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">
  <Name>my-bucket</Name>
  <Prefix>logs/</Prefix>
  <KeyCount>1</KeyCount>
  <MaxKeys>1000</MaxKeys>
  <IsTruncated>false</IsTruncated>
  <Contents>
    <Key>logs/one.txt</Key>
    <LastModified>2024-01-01T00:00:00.000Z</LastModified>
    <ETag>&quot;abc123&quot;</ETag>
    <Size>42</Size>
    <StorageClass>STANDARD</StorageClass>
  </Contents>
</ListBucketResult>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3ListObjects(context.Background(), awsConfig{}, awsS3Args{bucket: "my-bucket", prefix: "logs/"})
	if err != nil {
		t.Fatalf("listObjects: %v", err)
	}
	if gotPath != "/my-bucket" {
		t.Fatalf("expected path-style request to /my-bucket, got %s", gotPath)
	}
	if gotPrefix != "logs/" {
		t.Fatalf("expected prefix query param logs/, got %q", gotPrefix)
	}
	m := out.(map[string]any)
	contents, ok := m["Contents"].([]any)
	if !ok || len(contents) != 1 {
		t.Fatalf("expected 1 object, got %#v", m["Contents"])
	}
}

func TestAWSS3_HeadObject(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.Header().Set("Content-Length", "42")
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3HeadObject(context.Background(), awsConfig{}, awsS3Args{bucket: "my-bucket", key: "one.txt"})
	if err != nil {
		t.Fatalf("headObject: %v", err)
	}
	if gotMethod != http.MethodHead {
		t.Fatalf("expected HEAD, got %s", gotMethod)
	}
	if gotPath != "/my-bucket/one.txt" {
		t.Fatalf("expected path-style request to /my-bucket/one.txt, got %s", gotPath)
	}
	m := out.(map[string]any)
	if cl, ok := m["ContentLength"].(float64); !ok || cl != 42 {
		t.Fatalf("expected ContentLength 42, got %#v", m["ContentLength"])
	}
	if m["ContentType"] != "text/plain" {
		t.Fatalf("expected ContentType text/plain, got %#v", m["ContentType"])
	}
}

func TestAWSS3_GetObject(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3GetObject(context.Background(), awsConfig{}, awsS3Args{bucket: "my-bucket", key: "one.txt"})
	if err != nil {
		t.Fatalf("getObject: %v", err)
	}
	if gotPath != "/my-bucket/one.txt" {
		t.Fatalf("expected path-style request to /my-bucket/one.txt, got %s", gotPath)
	}
	m := out.(map[string]any)
	raw, ok := m["bytes"].([]byte)
	if !ok || string(raw) != "hello world" {
		t.Fatalf("expected bytes 'hello world', got %#v", m["bytes"])
	}
}

func TestAWSS3_PutObject(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("ETag", `"def456"`)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3PutObject(context.Background(), awsConfig{}, awsS3Args{bucket: "my-bucket", key: "one.txt", body: []byte("hello world")})
	if err != nil {
		t.Fatalf("putObject: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if gotPath != "/my-bucket/one.txt" {
		t.Fatalf("expected path-style request to /my-bucket/one.txt, got %s", gotPath)
	}
	if string(gotBody) != "hello world" {
		t.Fatalf("expected body 'hello world', got %q", gotBody)
	}
	m := out.(map[string]any)
	if m["ETag"] != `"def456"` {
		t.Fatalf("expected ETag, got %#v", m["ETag"])
	}
}

func TestAWSS3_DeleteObject(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3DeleteObject(context.Background(), awsConfig{}, awsS3Args{bucket: "my-bucket", key: "one.txt"})
	if err != nil {
		t.Fatalf("deleteObject: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
	if gotPath != "/my-bucket/one.txt" {
		t.Fatalf("expected path-style request to /my-bucket/one.txt, got %s", gotPath)
	}
	if m, ok := out.(map[string]any); !ok || len(m) != 0 {
		t.Fatalf("expected empty {}, got %#v", out)
	}
}

// TestAWSS3_ErrorPathThrows proves an AWS API error response is mapped end to
// end (SDK response -> smithy APIError -> mapAWSError) into a structured
// awsError, rather than a nil error or a resolved value. This establishes the
// error-path pattern that Tasks 4-11 copy into their own service test files.
func TestAWSS3_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><Error><Code>NoSuchBucket</Code><Message>The specified bucket does not exist</Message></Error>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	out, err := awsS3ListObjects(context.Background(), awsConfig{}, awsS3Args{bucket: "missing"})
	if err == nil {
		t.Fatalf("expected error, got nil (out=%#v)", out)
	}
	ae, ok := err.(awsError)
	if !ok {
		t.Fatalf("expected awsError, got %T: %v", err, err)
	}
	if ae.code == "" {
		t.Fatalf("expected non-empty error code, got %q (message=%q)", ae.code, ae.message)
	}
	if ae.status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", ae.status)
	}
	if ae.code != "NoSuchBucket" {
		t.Fatalf("expected code NoSuchBucket, got %q", ae.code)
	}
}

func TestAWSS3_ListBuckets_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><ListAllMyBucketsResult><Buckets><Bucket><Name>bucket-a</Name></Bucket></Buckets></ListAllMyBucketsResult>`))
	}))
	defer ts.Close()
	withMockAWS(t, ts)

	got := runCloudAWSScript(t, `
		const __result = await cloud.aws({ region: "eu-north-1" }).s3().listBuckets();
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object result, got %#v", got)
	}
	buckets, ok := m["Buckets"].([]any)
	if !ok || len(buckets) != 1 {
		t.Fatalf("expected 1 bucket, got %#v", m["Buckets"])
	}
}
