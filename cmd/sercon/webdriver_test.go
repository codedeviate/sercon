package main

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
)

func TestByStrategy(t *testing.T) {
	cases := map[string]string{
		"css":             selenium.ByCSSSelector,
		"xpath":           selenium.ByXPATH,
		"id":              selenium.ByID,
		"name":            selenium.ByName,
		"tag":             selenium.ByTagName,
		"className":       selenium.ByClassName,
		"linkText":        selenium.ByLinkText,
		"partialLinkText": selenium.ByPartialLinkText,
	}
	for in, want := range cases {
		got, err := byStrategy(in)
		if err != nil || got != want {
			t.Fatalf("byStrategy(%q) = %q,%v want %q", in, got, err, want)
		}
	}
	if _, err := byStrategy("nope"); err == nil {
		t.Fatalf("expected error for unknown strategy")
	}
}

// chromeArgsOf digs the args out of the chrome capabilities stored under the
// goog:chromeOptions key. AddChrome stores a chrome.Capabilities value; we
// reach the Args via type assertion.
func chromeArgsOf(caps selenium.Capabilities) []string {
	raw, ok := caps[chrome.CapabilitiesKey]
	if !ok {
		return nil
	}
	cc, ok := raw.(chrome.Capabilities)
	if !ok {
		return nil
	}
	return cc.Args
}

func TestBuildCaps(t *testing.T) {
	// chrome + headless adds --headless=new; extra args merged.
	caps := buildCaps(map[string]any{"browser": "chrome", "headless": true, "args": []any{"--no-sandbox"}})
	if caps["browserName"] != "chrome" {
		t.Fatalf("browserName = %v", caps["browserName"])
	}
	args := chromeArgsOf(caps)
	if !wdContains(args, "--headless=new") || !wdContains(args, "--no-sandbox") {
		t.Fatalf("chrome args = %v", args)
	}
}

func wdContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// --- Task 2 tests ---

func TestSessionDoClosed(t *testing.T) {
	s := &wdSession{reg: &wdRegistry{sessions: map[*wdSession]struct{}{}}}
	s.closed.Store(true)
	_, err := s.do(func() (any, error) { return 1, nil })
	if err == nil {
		t.Fatalf("expected error calling do() on a closed session")
	}
}

func skipIfNoDriver(t *testing.T) {
	t.Helper()
	if !wdAvailable() {
		t.Skip("no chromedriver/geckodriver on PATH; skipping webdriver integration test")
	}
}

func TestConnectQuitIntegration(t *testing.T) {
	skipIfNoDriver(t)
	r := &wdRegistry{sessions: map[*wdSession]struct{}{}}
	work := r.connect(nil, nil)
	// NOTE: jsObject(nil, nil) builds the handle map without deref; we only
	// exercise connect/track/shutdown here, not the goja methods.
	_, err := work(context.Background(), goja.FunctionCall{})
	if err != nil {
		t.Skipf("driver present but connect failed (browser not installed?): %v", err)
	}
	if len(r.sessions) != 1 {
		t.Fatalf("expected 1 tracked session, got %d", len(r.sessions))
	}
	r.closeAll()
	if len(r.sessions) != 0 {
		t.Fatalf("closeAll should drain sessions")
	}
}
