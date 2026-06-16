package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
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
