package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAzureBlob_ListContainers exercises the "list" pager against a mock
// server returning the REST-XML EnumerationResults shape azblob expects for
// ServiceClient.ListContainersSegment.
func TestAzureBlob_ListContainers(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<EnumerationResults ServiceEndpoint="` + r.Host + `">
	<Containers>
		<Container>
			<Name>container1</Name>
			<Properties>
				<Last-Modified>Mon, 01 Jan 2024 00:00:00 GMT</Last-Modified>
				<Etag>"0x1"</Etag>
			</Properties>
		</Container>
		<Container>
			<Name>container2</Name>
			<Properties>
				<Last-Modified>Mon, 01 Jan 2024 00:00:00 GMT</Last-Modified>
				<Etag>"0x2"</Etag>
			</Properties>
		</Container>
	</Containers>
</EnumerationResults>`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureBlobListContainers(context.Background(), azureConfig{}, ts.URL, azureBlobArgs{})
	if err != nil {
		t.Fatalf("azureBlobListContainers error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 2 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["Name"] != "container1" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
	second, ok := vals[1].(map[string]any)
	if !ok || second["Name"] != "container2" {
		t.Fatalf("unexpected second element: %#v", vals[1])
	}
}

// TestAzureBlob_ListBlobs exercises the "listBlobs" pager against a mock
// server returning the REST-XML EnumerationResults shape azblob expects for
// ContainerClient.ListBlobFlatSegment.
func TestAzureBlob_ListBlobs(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<EnumerationResults ServiceEndpoint="` + r.Host + `" ContainerName="mycontainer">
	<Blobs>
		<Blob>
			<Name>blob1.txt</Name>
			<Properties>
				<Last-Modified>Mon, 01 Jan 2024 00:00:00 GMT</Last-Modified>
				<Etag>"0x1"</Etag>
				<Content-Length>5</Content-Length>
			</Properties>
		</Blob>
	</Blobs>
</EnumerationResults>`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureBlobListBlobs(context.Background(), azureConfig{}, ts.URL, azureBlobArgs{container: "mycontainer"})
	if err != nil {
		t.Fatalf("azureBlobListBlobs error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 1 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["Name"] != "blob1.txt" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

// TestAzureBlob_Download exercises DownloadStream against a mock server that
// returns raw bytes (not JSON/XML) — this is the wire format for a blob's
// content.
func TestAzureBlob_Download(t *testing.T) {
	want := []byte("hello blob world")
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(want)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(want)
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureBlobDownload(context.Background(), azureConfig{}, ts.URL, azureBlobArgs{container: "c", blob: "b"})
	if err != nil {
		t.Fatalf("azureBlobDownload error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", out)
	}
	got, ok := m["bytes"].([]byte)
	if !ok {
		t.Fatalf("expected []byte bytes field, got %T", m["bytes"])
	}
	if string(got) != string(want) {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// TestAzureBlob_Upload exercises UploadBuffer (single-shot "Put Blob" for a
// small buffer) against a mock server returning 201 Created.
func TestAzureBlob_Upload(t *testing.T) {
	var gotMethod string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("ETag", `"0xUP"`)
		w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		w.WriteHeader(http.StatusCreated)
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureBlobUpload(context.Background(), azureConfig{}, ts.URL, azureBlobArgs{
		container: "c", blob: "b.txt", body: []byte("payload"),
	})
	if err != nil {
		t.Fatalf("azureBlobUpload error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
	if string(gotBody) != "payload" {
		t.Fatalf("expected body %q, got %q", "payload", gotBody)
	}
	if out == nil {
		t.Fatal("expected a non-nil result")
	}
}

// TestAzureBlob_DeleteBlob exercises DeleteBlob against a mock server
// returning 202 Accepted (the only status code the SDK accepts for delete).
func TestAzureBlob_DeleteBlob(t *testing.T) {
	var gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	out, err := azureBlobDeleteBlob(context.Background(), azureConfig{}, ts.URL, azureBlobArgs{container: "c", blob: "b.txt"})
	if err != nil {
		t.Fatalf("azureBlobDeleteBlob error: %v", err)
	}
	if gotMethod != http.MethodDelete {
		t.Fatalf("expected DELETE, got %s", gotMethod)
	}
	m, ok := out.(map[string]any)
	if !ok || len(m) != 0 {
		t.Fatalf("expected empty map result, got %#v", out)
	}
}

// TestAzureBlob_ListContainers_ViaJS exercises the full JS surface:
// cloud.azure(cfg).blob(url).listContainers() — the account URL is the
// httptest server's own URL, templated straight into the script body (the
// accessor takes the URL as a plain JS argument, not a config field).
func TestAzureBlob_ListContainers_ViaJS(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<EnumerationResults ServiceEndpoint="` + r.Host + `">
	<Containers>
		<Container>
			<Name>fromjs</Name>
			<Properties>
				<Last-Modified>Mon, 01 Jan 2024 00:00:00 GMT</Last-Modified>
				<Etag>"0x1"</Etag>
			</Properties>
		</Container>
	</Containers>
</EnumerationResults>`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	got := runCloudAzureScript(t, fmt.Sprintf(`
		const az = cloud.azure({});
		const __result = await az.blob(%q).listContainers();
	`, ts.URL))
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T: %#v", got, got)
	}
	vals, ok := m["value"].([]any)
	if !ok || len(vals) != 1 {
		t.Fatalf("unexpected decoded shape: %#v", m)
	}
	first, ok := vals[0].(map[string]any)
	if !ok || first["Name"] != "fromjs" {
		t.Fatalf("unexpected first element: %#v", vals[0])
	}
}

// TestAzureBlob_ErrorPathThrows exercises the error path: a non-2xx response
// from the mock server must be mapped through mapAzureError into an
// azureError, not returned as a plain error or panic.
func TestAzureBlob_ErrorPathThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<Error><Code>BlobNotFound</Code><Message>The specified blob does not exist.</Message></Error>`))
	}))
	defer ts.Close()
	withMockAzure(t, ts)

	_, err := azureBlobDownload(context.Background(), azureConfig{}, ts.URL, azureBlobArgs{container: "c", blob: "missing.txt"})
	if err == nil {
		t.Fatal("expected an error for a 404 response")
	}
	ae, ok := err.(azureError)
	if !ok {
		t.Fatalf("expected azureError, got %T: %v", err, err)
	}
	if ae.status != http.StatusNotFound {
		t.Fatalf("expected status 404, got %d", ae.status)
	}
}
