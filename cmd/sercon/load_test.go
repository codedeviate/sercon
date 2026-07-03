package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClassifyTarget(t *testing.T) {
	private := []string{"127.0.0.1", "::1", "10.1.2.3", "192.168.0.5", "172.16.9.9", "localhost", "0.0.0.0", "169.254.1.1", "fc00::1"}
	public := []string{"8.8.8.8", "1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"}
	for _, h := range private {
		if classifyTarget(h) {
			t.Errorf("%s classified public, want private", h)
		}
	}
	for _, h := range public {
		if !classifyTarget(h) {
			t.Errorf("%s classified private, want public", h)
		}
	}
}

func TestPercentiles(t *testing.T) {
	xs := make([]float64, 100)
	for i := range xs {
		xs[i] = float64(i + 1) // 1..100
	}
	got := percentiles(xs, 50, 95, 99, 0, 100)
	want := []float64{50, 95, 99, 1, 100}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 0.0001 {
			t.Errorf("p%v = %v, want %v", []float64{50, 95, 99, 0, 100}[i], got[i], want[i])
		}
	}
	if len(percentiles(nil, 50)) != 1 || percentiles(nil, 50)[0] != 0 {
		t.Error("empty input should yield zeros")
	}
}

func TestLoadHTTP_EndToEnd(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/fail", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got := runCaptureScript(t, fmt.Sprintf(`
		const r = await net.load.http({ url: %q, requests: 100, concurrency: 8 });
		__capture(r);
	`, srv.URL+"/ok"), nil)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("report not an object: %#v", got)
	}
	if m["sent"] != int64(100) && m["sent"] != 100 {
		t.Fatalf("sent = %v, want 100", m["sent"])
	}
	if fmt.Sprintf("%v", m["completed"]) != "100" {
		t.Fatalf("completed = %v, want 100", m["completed"])
	}
	lat, _ := m["latency"].(map[string]any)
	if lat == nil {
		t.Fatalf("no latency block: %#v", m)
	}
}

// TestLoadHTTP_PublicNeedsConfirm covers the dual-use guardrail at the Go level
// (deterministic, no engine / no traffic): a public IP classifies as public so
// the binding refuses it without confirm:true, while loopback is allowed.
func TestLoadHTTP_PublicNeedsConfirm(t *testing.T) {
	if !targetIsPublic(context.Background(), "8.8.8.8") {
		t.Error("8.8.8.8 should be public (guard requires confirm:true)")
	}
	if targetIsPublic(context.Background(), "127.0.0.1") {
		t.Error("127.0.0.1 should be private (guard allows it)")
	}
}

// TestRedirectGuard_BlocksPublicWithoutConfirm covers the redirect-hop guard
// at the Go level (deterministic, no engine / no traffic): a redirect whose
// Location is a public IP literal must be refused without confirm:true,
// while a redirect to loopback is allowed. IP literals classify without a
// DNS lookup (see targetIsPublic), so this trips reliably offline.
func TestRedirectGuard_BlocksPublicWithoutConfirm(t *testing.T) {
	ctx := context.Background()
	guard := redirectGuard(ctx, false)

	pub, err := url.Parse("http://8.8.8.8/next")
	if err != nil {
		t.Fatal(err)
	}
	if gerr := guard(&http.Request{URL: pub}, []*http.Request{{}}); gerr == nil {
		t.Fatal("expected a redirect-to-public-host error, got nil")
	} else if !strings.Contains(gerr.Error(), "public host") {
		t.Fatalf("error = %q, want it to mention the public host refusal", gerr)
	}

	loop, err := url.Parse("http://127.0.0.1/next")
	if err != nil {
		t.Fatal(err)
	}
	if gerr := guard(&http.Request{URL: loop}, []*http.Request{{}}); gerr != nil {
		t.Fatalf("redirect to loopback should be allowed, got %v", gerr)
	}
}

// TestRedirectGuard_ConfirmAllowsPublic asserts confirm:true lets a redirect
// to a public host through, matching the initial-URL guard's behaviour.
func TestRedirectGuard_ConfirmAllowsPublic(t *testing.T) {
	guard := redirectGuard(context.Background(), true)
	pub, err := url.Parse("http://8.8.8.8/next")
	if err != nil {
		t.Fatal(err)
	}
	if gerr := guard(&http.Request{URL: pub}, []*http.Request{{}}); gerr != nil {
		t.Fatalf("confirm:true should allow a redirect to a public host, got %v", gerr)
	}
}

// TestRedirectGuard_CapsRedirectChain asserts the 10-hop cap applies
// regardless of confirm, independent of the public-host check.
func TestRedirectGuard_CapsRedirectChain(t *testing.T) {
	guard := redirectGuard(context.Background(), true)
	loop, err := url.Parse("http://127.0.0.1/next")
	if err != nil {
		t.Fatal(err)
	}
	via := make([]*http.Request, 10)
	if gerr := guard(&http.Request{URL: loop}, via); gerr == nil {
		t.Fatal("expected a redirect-cap error after 10 hops, got nil")
	} else if !strings.Contains(gerr.Error(), "10 redirects") {
		t.Fatalf("error = %q, want it to mention the redirect cap", gerr)
	}
}

// TestLoadHTTP_FollowsNonPublicRedirect is an end-to-end regression check:
// net.load.http against a loopback server that 302s to another loopback
// server must still complete normally, proving the new CheckRedirect hook
// doesn't break ordinary same-network redirects.
func TestLoadHTTP_FollowsNonPublicRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()

	got := runCaptureScript(t, fmt.Sprintf(`
		const r = await net.load.http({ url: %q, requests: 5, concurrency: 2 });
		__capture(r);
	`, redirector.URL), nil)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("report not an object: %#v", got)
	}
	if fmt.Sprintf("%v", m["completed"]) != "5" {
		t.Fatalf("completed = %v, want 5 (redirect to loopback should still be followed)", m["completed"])
	}
}
