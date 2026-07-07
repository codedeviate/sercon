package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildGlobalArgs(t *testing.T) {
	cases := []struct {
		name string
		opts map[string]any
		want []string
	}{
		{"empty", map[string]any{}, nil},
		{"headed", map[string]any{"headed": true}, []string{"--headed"}},
		{"headed-false-omitted", map[string]any{"headed": false}, nil},
		{"string-flags", map[string]any{"profile": "Default", "proxy": "http://127.0.0.1:8080"},
			[]string{"--profile", "Default", "--proxy", "http://127.0.0.1:8080"}},
		{"userAgent", map[string]any{"userAgent": "x"}, []string{"--user-agent", "x"}},
		{"unknown-ignored", map[string]any{"nope": "x"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := buildGlobalArgs(c.opts)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("buildGlobalArgs(%v) = %v, want %v", c.opts, got, c.want)
			}
		})
	}
}

func TestRegistrySessionIDsUnique(t *testing.T) {
	reg := &abRegistry{sessions: map[string]struct{}{}}
	a := reg.allocSession("")
	b := reg.allocSession("")
	if a == b {
		t.Fatalf("expected unique session ids, got %q twice", a)
	}
	if !strings.HasPrefix(a, "sercon-") {
		t.Fatalf("auto id should be prefixed sercon-, got %q", a)
	}
	if got := reg.allocSession("custom"); got != "custom" {
		t.Fatalf("explicit session name should pass through, got %q", got)
	}
	// allocSession records open sessions for cleanup.
	if len(reg.sessions) != 3 {
		t.Fatalf("expected 3 tracked sessions, got %d", len(reg.sessions))
	}
	reg.forget("custom")
	if _, ok := reg.sessions["custom"]; ok {
		t.Fatalf("forget should drop the session")
	}
}

func skipIfNoAgentBrowser(t *testing.T) {
	t.Helper()
	if !abAvailable() {
		t.Skip("agent-browser CLI not on PATH; skipping integration test")
	}
}

func TestLaunchCloseIntegration(t *testing.T) {
	skipIfNoAgentBrowser(t)
	reg := &abRegistry{sessions: map[string]struct{}{}}
	h := &abHandle{session: reg.allocSession(""), reg: reg}
	if _, err := h.close(context.Background(), struct{}{}); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !h.closed.Load() {
		t.Fatalf("handle should be marked closed")
	}
}

func TestNavArgs(t *testing.T) {
	if got := navArgs("open", "https://x"); !reflect.DeepEqual(got, []string{"open", "https://x"}) {
		t.Fatalf("open args = %v", got)
	}
	if got := navArgs("reload"); !reflect.DeepEqual(got, []string{"reload"}) {
		t.Fatalf("reload args = %v", got)
	}
	if got := navArgs("wait", "500"); !reflect.DeepEqual(got, []string{"wait", "500"}) {
		t.Fatalf("wait args = %v", got)
	}
}

func TestFrameArgs(t *testing.T) {
	cases := map[string][]string{
		"#klarna-checkout-iframe": {"frame", "#klarna-checkout-iframe"},
		"@e3":                     {"frame", "@e3"},
		"main":                    {"frame", "main"},
	}
	for arg, want := range cases {
		got := navArgs("frame", arg)
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("navArgs(frame, %q) = %v, want %v", arg, got, want)
		}
	}
}

func TestInteractArgs(t *testing.T) {
	if got := interactArgs("click", "#a"); !reflect.DeepEqual(got, []string{"click", "#a"}) {
		t.Fatalf("click = %v", got)
	}
	if got := interactArgs("fill", "#a", "hello"); !reflect.DeepEqual(got, []string{"fill", "#a", "hello"}) {
		t.Fatalf("fill = %v", got)
	}
	if got := interactArgs("scroll", "down", "200"); !reflect.DeepEqual(got, []string{"scroll", "down", "200"}) {
		t.Fatalf("scroll = %v", got)
	}
}

func TestSnapshotArgs(t *testing.T) {
	got := snapshotArgs(map[string]any{"interactive": true, "compact": true, "depth": float64(3), "selector": "#root"})
	want := []string{"snapshot", "-i", "-c", "-d", "3", "-s", "#root"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshotArgs = %v, want %v", got, want)
	}
	if got := snapshotArgs(map[string]any{}); !reflect.DeepEqual(got, []string{"snapshot"}) {
		t.Fatalf("empty snapshotArgs = %v", got)
	}
}

func TestFindArgs(t *testing.T) {
	got := abFindArgs("role", "button", "click", "")
	if !reflect.DeepEqual(got, []string{"find", "role", "button", "click"}) {
		t.Fatalf("abFindArgs = %v", got)
	}
	got = abFindArgs("text", "Submit", "fill", "hello")
	if !reflect.DeepEqual(got, []string{"find", "text", "Submit", "fill", "hello"}) {
		t.Fatalf("abFindArgs with text = %v", got)
	}
}

func TestSetArgs(t *testing.T) {
	if got := setArgs("viewport", "1920", "1080"); !reflect.DeepEqual(got, []string{"set", "viewport", "1920", "1080"}) {
		t.Fatalf("viewport = %v", got)
	}
	if got := setArgs("offline", "on"); !reflect.DeepEqual(got, []string{"set", "offline", "on"}) {
		t.Fatalf("offline = %v", got)
	}
	if got := recordArgs("start", "/tmp/x.webm"); !reflect.DeepEqual(got, []string{"record", "start", "/tmp/x.webm"}) {
		t.Fatalf("record start = %v", got)
	}
	if got := offlineArg(true); got != "on" {
		t.Fatalf("offlineArg(true) = %q", got)
	}
	if got := offlineArg(false); got != "off" {
		t.Fatalf("offlineArg(false) = %q", got)
	}
}

func TestScreenshotArgs(t *testing.T) {
	got := screenshotArgs(map[string]any{"selector": "#root", "full": true, "format": "jpeg", "quality": float64(80)})
	want := []string{"screenshot", "#root", "--full", "--screenshot-format", "jpeg", "--screenshot-quality", "80"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("screenshotArgs = %v, want %v", got, want)
	}
	if got := screenshotArgs(map[string]any{}); !reflect.DeepEqual(got, []string{"screenshot"}) {
		t.Fatalf("empty screenshotArgs = %v", got)
	}
}

func TestAbCapturePath(t *testing.T) {
	p, err := abCapturePath(`{"success":true,"data":{"path":"/tmp/shot.png"},"error":null}`)
	if err != nil || p != "/tmp/shot.png" {
		t.Fatalf("abCapturePath = %q, %v", p, err)
	}
	if _, err := abCapturePath(`{"success":true,"data":{},"error":null}`); err == nil {
		t.Fatalf("expected error when path missing")
	}
}

func TestOneShotNeedsURL(t *testing.T) {
	reg := &abRegistry{sessions: map[string]struct{}{}, defaults: map[string]any{}}
	// withEphemeral should error before allocating a session when url is empty.
	_, err := reg.withEphemeral(context.Background(), "", func(h *abHandle) (any, error) {
		return nil, nil
	})
	if err == nil {
		t.Fatalf("expected error for empty url")
	}
	if len(reg.sessions) != 0 {
		t.Fatalf("no session should be allocated on empty-url error, got %d", len(reg.sessions))
	}
}

func TestNetworkArgs(t *testing.T) {
	got, _ := routeArgs("**/api/*", map[string]any{"abort": true})
	if !reflect.DeepEqual(got, []string{"network", "route", "**/api/*", "--abort"}) {
		t.Fatalf("route abort = %v", got)
	}
	// abort route must not return an error.
	if _, err := routeArgs("**/api/*", map[string]any{"abort": true}); err != nil {
		t.Fatalf("route abort error = %v", err)
	}
	got, _ = routeArgs("**/d.json", map[string]any{"body": map[string]any{"mock": true}})
	if !reflect.DeepEqual(got, []string{"network", "route", "**/d.json", "--body", `{"mock":true}`}) {
		t.Fatalf("route body = %v", got)
	}
	if got := requestsArgs(map[string]any{"clear": true, "filter": "api", "method": "GET"}); !reflect.DeepEqual(got, []string{"network", "requests", "--clear", "--filter", "api", "--method", "GET"}) {
		t.Fatalf("requests = %v", got)
	}
}

func TestRunJSONClosedHandle(t *testing.T) {
	h := &abHandle{session: "x", reg: &abRegistry{sessions: map[string]struct{}{}}}
	h.closed.Store(true)
	if _, err := h.runJSON(context.Background(), "anything"); err == nil {
		t.Fatalf("expected error on a closed handle")
	}
}

func TestStorageAndCookieArgs(t *testing.T) {
	got := cookieSetArgs("sid", "abc", map[string]any{"domain": ".x.com", "httpOnly": true, "sameSite": "Lax"})
	want := []string{"cookies", "set", "sid", "abc", "--domain", ".x.com", "--sameSite", "Lax", "--httpOnly"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cookieSetArgs = %v, want %v", got, want)
	}
	// expires must be rendered as a plain integer, not scientific notation.
	gotExp := cookieSetArgs("s", "v", map[string]any{"expires": float64(1700000000)})
	wantExp := []string{"cookies", "set", "s", "v", "--expires", "1700000000"}
	if !reflect.DeepEqual(gotExp, wantExp) {
		t.Fatalf("cookieSetArgs expires = %v, want %v", gotExp, wantExp)
	}
	if got := storageArgs("local", "get", "k"); !reflect.DeepEqual(got, []string{"storage", "local", "get", "k"}) {
		t.Fatalf("storage get = %v", got)
	}
	if got := storageArgs("session", "clear"); !reflect.DeepEqual(got, []string{"storage", "session", "clear"}) {
		t.Fatalf("storage clear = %v", got)
	}
}

func TestTabArgs(t *testing.T) {
	if got := tabNewArgs("https://x", ""); !reflect.DeepEqual(got, []string{"tab", "new", "https://x"}) {
		t.Fatalf("tab new url = %v", got)
	}
	if got := tabNewArgs("https://x", "docs"); !reflect.DeepEqual(got, []string{"tab", "new", "--label", "docs", "https://x"}) {
		t.Fatalf("tab new label = %v", got)
	}
	if got := tabNewArgs("", ""); !reflect.DeepEqual(got, []string{"tab", "new"}) {
		t.Fatalf("tab new bare = %v", got)
	}
}

func TestDiffArgs(t *testing.T) {
	got := diffSnapshotArgs(map[string]any{"baseline": "/b.json", "selector": "#main", "compact": true, "depth": float64(2)})
	want := []string{"diff", "snapshot", "-b", "/b.json", "-s", "#main", "-c", "-d", "2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diffSnapshotArgs = %v, want %v", got, want)
	}
	got = diffScreenshotArgs(map[string]any{"baseline": "/base.png", "output": "/out.png", "threshold": float64(0.2)})
	want = []string{"diff", "screenshot", "--baseline", "/base.png", "-o", "/out.png", "-t", "0.2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("diffScreenshotArgs = %v, want %v", got, want)
	}
}

func TestDebugArgs(t *testing.T) {
	if got := profilerStartArgs(map[string]any{"categories": "v8,blink"}); !reflect.DeepEqual(got, []string{"profiler", "start", "--categories", "v8,blink"}) {
		t.Fatalf("profiler start = %v", got)
	}
	if got := profilerStartArgs(map[string]any{}); !reflect.DeepEqual(got, []string{"profiler", "start"}) {
		t.Fatalf("profiler start bare = %v", got)
	}
	if got := clipboardArgs("write", "hello"); !reflect.DeepEqual(got, []string{"clipboard", "write", "hello"}) {
		t.Fatalf("clipboard write = %v", got)
	}
	if got := clipboardArgs("read", ""); !reflect.DeepEqual(got, []string{"clipboard", "read"}) {
		t.Fatalf("clipboard read = %v", got)
	}
}

func TestMergeLaunchOpts(t *testing.T) {
	defaults := map[string]any{"headed": true, "proxy": "http://d"}
	opts := map[string]any{"proxy": "http://o", "userAgent": "ua"}
	got := mergeLaunchOpts(defaults, opts)
	// opts wins on conflict; union of keys.
	if got["headed"] != true {
		t.Fatalf("expected headed from defaults, got %v", got["headed"])
	}
	if got["proxy"] != "http://o" {
		t.Fatalf("expected opts.proxy to win, got %v", got["proxy"])
	}
	if got["userAgent"] != "ua" {
		t.Fatalf("expected userAgent from opts, got %v", got["userAgent"])
	}
	// inputs must not be mutated.
	if _, ok := defaults["userAgent"]; ok {
		t.Fatalf("mergeLaunchOpts mutated the defaults map")
	}
}

func TestCallTimeout(t *testing.T) {
	// Missing key → default (30 s).
	if got := callTimeout(map[string]any{}); got != abDefaultCallTimeout {
		t.Fatalf("missing key: want %s, got %s", abDefaultCallTimeout, got)
	}
	// Explicit 0 → disabled (no timeout).
	if got := callTimeout(map[string]any{"timeout": float64(0)}); got != 0 {
		t.Fatalf("explicit 0: want 0, got %s", got)
	}
	// Positive value → that duration in ms.
	if got := callTimeout(map[string]any{"timeout": float64(5000)}); got != 5*time.Second {
		t.Fatalf("5000 ms: want 5s, got %s", got)
	}
}

func TestReactArgs(t *testing.T) {
	if got := suspenseArgs(map[string]any{"onlyDynamic": true}); !reflect.DeepEqual(got, []string{"react", "suspense", "--only-dynamic"}) {
		t.Fatalf("suspense onlyDynamic = %v", got)
	}
	if got := suspenseArgs(map[string]any{}); !reflect.DeepEqual(got, []string{"react", "suspense"}) {
		t.Fatalf("suspense bare = %v", got)
	}
}

func TestAdvancedArgs(t *testing.T) {
	if got := streamEnableArgs(map[string]any{"port": float64(9000)}); !reflect.DeepEqual(got, []string{"stream", "enable", "--port", "9000"}) {
		t.Fatalf("stream enable port = %v", got)
	}
	if got := streamEnableArgs(map[string]any{}); !reflect.DeepEqual(got, []string{"stream", "enable"}) {
		t.Fatalf("stream enable bare = %v", got)
	}
	if got := chatArgs("hello there", map[string]any{"model": "gpt-x"}); !reflect.DeepEqual(got, []string{"chat", "hello there", "--model", "gpt-x"}) {
		t.Fatalf("chat = %v", got)
	}
	if got := batchArgs([]string{"open x", "snapshot"}, map[string]any{"bail": true}); !reflect.DeepEqual(got, []string{"batch", "--bail", "open x", "snapshot"}) {
		t.Fatalf("batch = %v", got)
	}
}

func TestAuthSaveArgs(t *testing.T) {
	got := authSaveArgs("prod", map[string]any{
		"url": "https://x/login", "username": "u",
		"usernameSelector": "#user",
	})
	want := []string{"auth", "save", "prod", "--url", "https://x/login", "--username", "u", "--username-selector", "#user", "--password-stdin"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("authSaveArgs = %v, want %v", got, want)
	}
}
