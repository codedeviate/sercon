package main

import (
	"strings"
	"testing"

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
