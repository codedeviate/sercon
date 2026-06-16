package main

import (
	"math"
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
