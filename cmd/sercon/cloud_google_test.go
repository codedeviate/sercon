package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

func TestGoogleConfig_ClientOptions(t *testing.T) {
	cfg := googleConfig{
		credentialsFile: "/tmp/sa.json",
		quotaProject:    "qp",
		scopes:          []string{"https://www.googleapis.com/auth/cloud-platform"},
	}
	// Base options present, and injected test opts appended last.
	opts := cfg.clientOptions(option.WithoutAuthentication())
	if len(opts) < 4 {
		t.Fatalf("expected creds+scope+quota+injected options, got %d", len(opts))
	}
	// An empty config yields no credential options (pure ADC), but still a slice.
	if got := (googleConfig{}).clientOptions(); got == nil {
		t.Fatal("clientOptions must never return nil")
	}
}

func TestGoogleConfig_CredsNeverLogged(t *testing.T) {
	cfg := googleConfig{credentialsFile: "/secret/path.json", credentialsJSON: []byte(`{"private_key":"X"}`)}
	s := cfg.String()
	if want := "path.json"; strings.Contains(s, want) || strings.Contains(s, "private_key") || strings.Contains(s, "X") {
		t.Fatalf("googleConfig.String() must redact credentials, leaked in: %q", s)
	}
}

func TestMapGoogleError(t *testing.T) {
	src := &googleapi.Error{Code: http.StatusNotFound, Message: "bucket not found"}
	ge, ok := mapGoogleError(src).(googleError)
	if !ok {
		t.Fatalf("expected googleError, got %T", mapGoogleError(src))
	}
	f := ge.ErrorFields()
	if f["code"] != 404 || f["status"] != "Not Found" {
		t.Fatalf("bad fields: %#v", f)
	}
	if ge.Error() == "" || !contains(ge.Error(), "bucket not found") {
		t.Fatalf("message should include the API message, got %q", ge.Error())
	}
}

func TestGoogleCall_GET(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/compute/v1/projects/p/zones/z/instances" || r.URL.Query().Get("maxResults") != "5" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"name":"vm-1"}]}`))
	}))
	defer ts.Close()

	cfg := googleConfig{}
	got, err := googleCallWork(context.Background(), cfg, googleCallArgs{
		endpointBase: ts.URL, // test-only override field (see impl)
		api:          "compute", version: "v1", httpMethod: "GET",
		path:   "/compute/v1/projects/p/zones/z/instances",
		params: map[string]string{"maxResults": "5"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	b, _ := json.Marshal(got)
	if !contains(string(b), "vm-1") {
		t.Fatalf("expected instance in response, got %s", b)
	}
}

func TestGoogleCall_ErrorThrows(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":404,"message":"nope"}}`))
	}))
	defer ts.Close()
	_, err := googleCallWork(context.Background(), googleConfig{}, googleCallArgs{
		endpointBase: ts.URL, api: "compute", version: "v1", httpMethod: "GET", path: "/x",
	})
	ge, ok := err.(googleError)
	if !ok || ge.code != 404 {
		t.Fatalf("expected googleError code 404, got %v (%T)", err, err)
	}
}
