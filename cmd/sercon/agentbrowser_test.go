package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/dop251/goja"
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
	if _, err := h.close(context.Background(), goja.FunctionCall{}); err != nil {
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
	got := findArgs("role", "button", "click", "")
	if !reflect.DeepEqual(got, []string{"find", "role", "button", "click"}) {
		t.Fatalf("findArgs = %v", got)
	}
	got = findArgs("text", "Submit", "fill", "hello")
	if !reflect.DeepEqual(got, []string{"find", "text", "Submit", "fill", "hello"}) {
		t.Fatalf("findArgs with text = %v", got)
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
