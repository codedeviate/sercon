package main

import (
	"net/http"
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
