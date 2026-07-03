package main

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func TestWDCommandTimeout(t *testing.T) {
	cases := []struct {
		opts map[string]any
		want time.Duration
	}{
		{nil, wdDefaultCommandTimeout},
		{map[string]any{}, wdDefaultCommandTimeout},
		{map[string]any{"commandTimeout": float64(5000)}, 5 * time.Second},
		{map[string]any{"commandTimeout": int64(1500)}, 1500 * time.Millisecond},
		{map[string]any{"commandTimeout": float64(0)}, 0},  // disabled
		{map[string]any{"commandTimeout": float64(-1)}, 0}, // disabled
		{map[string]any{"commandTimeout": "nope"}, wdDefaultCommandTimeout},
	}
	for i, c := range cases {
		if got := wdCommandTimeout(c.opts); got != c.want {
			t.Errorf("case %d: got %v, want %v", i, got, c.want)
		}
	}
}

// hangingWD embeds selenium.WebDriver (leaving every other method nil / to
// panic-on-call) so it satisfies the interface while only overriding Quit,
// which blocks for blockFor before returning. Used to prove shutdown()
// bounds a wedged driver teardown instead of hanging the Run.
type hangingWD struct {
	selenium.WebDriver
	blockFor time.Duration
}

func (h *hangingWD) Quit() error {
	time.Sleep(h.blockFor)
	return nil
}

// shutdown must bound wd.Quit() with wdCloseTimeout so a wedged driver
// process can't hang the Run (or the explicit quit() call) forever.
func TestShutdown_BoundsHangingQuit(t *testing.T) {
	orig := wdCloseTimeout
	wdCloseTimeout = 30 * time.Millisecond
	defer func() { wdCloseTimeout = orig }()

	s := &wdSession{
		reg: &wdRegistry{sessions: map[*wdSession]struct{}{}},
		wd:  &hangingWD{blockFor: 5 * time.Second},
	}
	start := time.Now()
	s.shutdown()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("shutdown took %v, want bounded near the shrunk wdCloseTimeout (30ms)", elapsed)
	}
}

// closeAll drives shutdown() for every tracked session; it must inherit the
// same bound rather than hanging on a wedged Quit.
func TestCloseAll_BoundsHangingQuit(t *testing.T) {
	orig := wdCloseTimeout
	wdCloseTimeout = 30 * time.Millisecond
	defer func() { wdCloseTimeout = orig }()

	r := &wdRegistry{sessions: map[*wdSession]struct{}{}}
	s := &wdSession{reg: r, wd: &hangingWD{blockFor: 5 * time.Second}}
	r.track(s)

	start := time.Now()
	r.closeAll()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("closeAll took %v, want bounded near the shrunk wdCloseTimeout (30ms)", elapsed)
	}
}

// TestWDShutdownCancelsCtx verifies shutdown() cancels the session context so
// an in-flight raw command unblocks promptly.
func TestWDShutdownCancelsCtx(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &wdSession{
		reg:    &wdRegistry{sessions: map[*wdSession]struct{}{}},
		ctx:    ctx,
		cancel: cancel,
	}
	if s.ctx.Err() != nil {
		t.Fatal("ctx canceled before shutdown")
	}
	s.shutdown()
	if s.ctx.Err() == nil {
		t.Fatal("shutdown() did not cancel the session ctx")
	}
}

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

// --- Phase 2 Task 6 tests ---

func TestHoverViewport(t *testing.T) {
	body := wdHoverViewport(150, 250)
	moves := body["actions"].([]any)[0].(map[string]any)["actions"].([]any)
	if len(moves) != 2 {
		t.Fatalf("expected 2 pointer moves (anchor + target), got %d", len(moves))
	}
	for i, m := range moves {
		if m.(map[string]any)["origin"] != "viewport" {
			t.Fatalf("move %d must use viewport origin, got %v", i, m)
		}
	}
	// second move lands on the target centre; the anchor is a different point
	// (so the move is non-zero-distance).
	if moves[1].(map[string]any)["x"] != 150 || moves[1].(map[string]any)["y"] != 250 {
		t.Fatalf("target move = %v, want (150,250)", moves[1])
	}
	if moves[0].(map[string]any)["y"] == moves[1].(map[string]any)["y"] {
		t.Fatalf("anchor must differ from target so the move fires events")
	}
}

func TestDragViewport(t *testing.T) {
	body := wdDragViewport(10, 20, 300, 400)
	acts := body["actions"].([]any)[0].(map[string]any)["actions"].([]any)
	if len(acts) != 5 {
		t.Fatalf("expected 5 pointer actions (anchor, moveSrc, down, moveDst, up), got %d", len(acts))
	}
	if acts[1].(map[string]any)["x"] != 10 || acts[1].(map[string]any)["y"] != 20 {
		t.Fatalf("second action should move onto src (10,20), got %v", acts[1])
	}
	if acts[2].(map[string]any)["type"] != "pointerDown" {
		t.Fatalf("third action should be pointerDown, got %v", acts[2])
	}
	if acts[3].(map[string]any)["x"] != 300 || acts[3].(map[string]any)["y"] != 400 {
		t.Fatalf("fourth action should move onto dst (300,400), got %v", acts[3])
	}
	if acts[4].(map[string]any)["type"] != "pointerUp" {
		t.Fatalf("fifth action should be pointerUp, got %v", acts[4])
	}
}

func TestKeyChordActions(t *testing.T) {
	body := wdKeyChordActions([]string{"Control", "a"})
	acts := body["actions"].([]any)[0].(map[string]any)["actions"].([]any)
	if len(acts) != 4 {
		t.Fatalf("expected 4 key actions, got %d", len(acts))
	}
	if acts[0].(map[string]any)["type"] != "keyDown" || acts[0].(map[string]any)["value"] != "Control" {
		t.Fatalf("first action should be keyDown Control: %v", acts[0])
	}
	if acts[3].(map[string]any)["type"] != "keyUp" || acts[3].(map[string]any)["value"] != "Control" {
		t.Fatalf("last action should be keyUp Control (reverse release): %v", acts[3])
	}
}

// --- Phase 2 Task 7 tests ---

func TestIsElementRef(t *testing.T) {
	if !wdIsElementRef(map[string]any{webElementKey: "E1"}) {
		t.Fatalf("a map with the W3C element key should be an element ref")
	}
	if !wdIsElementRef(map[string]any{wdLegacyElementKey: "E2"}) {
		t.Fatalf("a map with the legacy ELEMENT key should be an element ref")
	}
	if wdIsElementRef(map[string]any{"foo": "bar"}) {
		t.Fatalf("a plain object is not an element ref")
	}
	if wdIsElementRef("hello") || wdIsElementRef(float64(42)) {
		t.Fatalf("scalars are not element refs")
	}
}

func TestWDFrameBody_Cases(t *testing.T) {
	// index
	if b, err := wdFrameBody(float64(2)); err != nil || b["id"] != 2 {
		t.Fatalf("index: %v %v", b, err)
	}
	// element handle
	b, err := wdFrameBody(map[string]any{"elementId": "E1"})
	if err != nil {
		t.Fatal(err)
	}
	if id, ok := b["id"].(map[string]string); !ok || id[webElementKey] != "E1" {
		t.Fatalf("element body: %#v", b)
	}
	// a bare string is NOT a wdFrameBody case (the binding find-resolves it);
	// wdFrameBody must reject it so a mis-route is caught.
	if _, err := wdFrameBody("#sel"); err == nil {
		t.Fatal("wdFrameBody should reject a raw string (binding resolves selectors)")
	}
}

// --- 0004 wait/click tests ---

func TestReadySuffix(t *testing.T) {
	cases := []struct {
		visible, enabled bool
		want             string
	}{
		{false, false, ""},
		{true, false, "/visible"},
		{false, true, "/enabled"},
		{true, true, "/visible/enabled"},
	}
	for _, c := range cases {
		if got := readySuffix(c.visible, c.enabled); got != c.want {
			t.Fatalf("readySuffix(%v,%v)=%q want %q", c.visible, c.enabled, got, c.want)
		}
	}
}

func TestClickWhenReadyArgs(t *testing.T) {
	// findArgsWD underpins clickWhenReady; it must reject a missing value.
	call := goja.FunctionCall{} // no args
	if _, _, err := findArgsWD(call); err == nil {
		t.Fatal("findArgsWD should error with no args (clickWhenReady requires by+value)")
	}
}
