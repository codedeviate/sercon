package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestStuffrWasm_Decompresses(t *testing.T) {
	b, err := stuffrWasm()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b[:4]), "\x00asm") {
		t.Fatalf("not a wasm module: % x", b[:4])
	}
	if len(b) <= 1<<20 {
		t.Fatalf("module too small: %d", len(b))
	}
	b2, err := stuffrWasm()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, b2) || &b[0] != &b2[0] {
		t.Fatal("second call did not return the same slice")
	}
}

func TestStuffrVersion_NonEmpty(t *testing.T) {
	if v := stuffrVersion(); !strings.HasPrefix(v, "stuffr ") {
		t.Fatalf("version = %q", v)
	}
}

func TestStuffrPersonalities_FromVersionJSON(t *testing.T) {
	p := stuffrPersonalities()
	for _, n := range []string{"gzip", "unzip", "zstdcat"} {
		if !p[n] {
			t.Errorf("missing personality %q", n)
		}
	}
	if p["pack"] {
		t.Error("pack must not be a personality")
	}
}
