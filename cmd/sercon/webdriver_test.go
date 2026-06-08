package main

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"

	"github.com/codedeviate/sercon/pkg/scriptengine"
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

// --- Task 3 tests ---

func TestNavMethodNames(t *testing.T) {
	// jsObject must expose the nav methods (built with a real vm is heavy;
	// assert the wiring indirectly via a names list the impl exposes).
	want := []string{"get", "url", "title", "back", "forward", "refresh"}
	for _, n := range want {
		if !wdNavMethods[n] {
			t.Fatalf("nav method %q missing from wdNavMethods", n)
		}
	}
}

// --- Task 4 tests ---

func TestWdDeliverShot(t *testing.T) {
	data := []byte{1, 2, 3, 4}
	// no path → bytes
	o, err := wdDeliverShot(data, "")
	if err != nil {
		t.Fatal(err)
	}
	res := o.(*scriptengine.Ordered)
	if _, ok := res.Get("bytes"); !ok {
		t.Fatalf("expected bytes key, got %v", res)
	}
	// path → write + metadata
	tmp := t.TempDir() + "/shot.png"
	o, err = wdDeliverShot(data, tmp)
	if err != nil {
		t.Fatal(err)
	}
	res = o.(*scriptengine.Ordered)
	if v, _ := res.Get("path"); v != tmp {
		t.Fatalf("path = %v", v)
	}
}

// --- Task 5 tests ---

func TestCookieFromArg(t *testing.T) {
	c := cookieFromMap(map[string]any{"name": "sid", "value": "abc", "path": "/", "secure": true})
	if c.Name != "sid" || c.Value != "abc" || c.Path != "/" || !c.Secure {
		t.Fatalf("cookieFromMap = %+v", c)
	}
}

// --- Phase 2 Task 1 tests ---

func TestWdEnvError(t *testing.T) {
	cases := []struct {
		name   string
		value  string
		status int
		want   string
	}{
		{"error+message", `{"error":"no such alert","message":"no alert open"}`, 400, "no such alert: no alert open"},
		{"error only", `{"error":"invalid session id"}`, 404, "invalid session id"},
		{"message only", `{"message":"boom"}`, 500, "boom"},
		{"empty falls back to status", `{}`, 500, "HTTP 500"},
		{"null value falls back", `null`, 500, "HTTP 500"},
	}
	for _, c := range cases {
		got := wdEnvError([]byte(c.value), c.status)
		if got != c.want {
			t.Fatalf("%s: wdEnvError = %q want %q", c.name, got, c.want)
		}
	}
}

func TestToStringSlice(t *testing.T) {
	got := toStringSlice([]any{"a", "b", "c"})
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("toStringSlice = %v", got)
	}
	if toStringSlice("not a slice") != nil {
		t.Fatalf("expected nil for non-slice input")
	}
}

// --- Phase 2 Task 2 tests ---

func TestWindowMethodNames(t *testing.T) {
	for _, n := range []string{"windowHandles", "currentWindow", "switchToWindow", "newWindow", "closeWindow"} {
		if !wdWindowMethods[n] {
			t.Fatalf("window method %q missing from wdWindowMethods", n)
		}
	}
}

// --- Phase 2 Task 3 tests ---

func TestFrameBody(t *testing.T) {
	// index target
	b, err := wdFrameBody(float64(2))
	if err != nil || b["id"] != 2 {
		t.Fatalf("index body = %v, %v", b, err)
	}
	// element handle target (map with elementId)
	b, err = wdFrameBody(map[string]any{"elementId": "E1"})
	if err != nil {
		t.Fatal(err)
	}
	ref, ok := b["id"].(map[string]string)
	if !ok || ref[webElementKey] != "E1" {
		t.Fatalf("element body = %v", b)
	}
	// bad target (string)
	if _, err := wdFrameBody("by-name"); err == nil {
		t.Fatalf("expected error for string frame target")
	}
}

// --- Phase 2 Task 4 tests ---

func TestAlertMethodNames(t *testing.T) {
	for _, n := range []string{"acceptAlert", "dismissAlert", "alertText", "sendAlertText"} {
		if !wdAlertMethods[n] {
			t.Fatalf("alert method %q missing from wdAlertMethods", n)
		}
	}
}

// --- Phase 2 Task 5 tests ---

func TestRectBody(t *testing.T) {
	// only width/height given -> x,y are nil (driver keeps them)
	b := wdRectBody(map[string]any{"width": float64(800), "height": float64(600)})
	if b["width"] != 800 || b["height"] != 600 {
		t.Fatalf("rect body w/h = %v", b)
	}
	if b["x"] != nil || b["y"] != nil {
		t.Fatalf("absent x/y should be nil, got %v", b)
	}
	// all four given
	b = wdRectBody(map[string]any{"width": float64(1024), "height": float64(768), "x": float64(10), "y": float64(20)})
	if b["x"] != 10 || b["y"] != 20 {
		t.Fatalf("rect body x/y = %v", b)
	}
	// empty map -> all four keys present and nil
	b = wdRectBody(map[string]any{})
	for _, k := range []string{"width", "height", "x", "y"} {
		v, ok := b[k]
		if !ok || v != nil {
			t.Fatalf("empty rect body[%q] = %v (present=%v), want nil/present", k, v, ok)
		}
	}
}

func TestRectMethodNames(t *testing.T) {
	for _, n := range []string{"getWindowRect", "setWindowRect", "maximize", "minimize", "fullscreen"} {
		if !wdRectMethods[n] {
			t.Fatalf("rect method %q missing from wdRectMethods", n)
		}
	}
}
