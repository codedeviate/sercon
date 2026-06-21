package main

import (
	"strings"
	"testing"
)

func TestCDPExec_FirefoxRejected(t *testing.T) {
	s := &wdSession{browser: "firefox"}
	_, err := s.cdpExec("Browser.getVersion", nil)
	if err == nil {
		t.Fatal("expected firefox to be rejected for CDP, got nil error")
	}
	if !strings.Contains(err.Error(), "Chrome-only") {
		t.Fatalf("error should mention Chrome-only, got: %v", err)
	}
}
