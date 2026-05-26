package main

import (
	"bytes"
	"strings"
	"testing"
)

// Every supported algorithm must round-trip ("hello world" -> compress ->
// decompress -> "hello world"). Smaller smoke than a corpus fuzz but enough
// to prove the writer/reader plumbing is wired correctly at the byte level.
func TestCompression_RoundTrip(t *testing.T) {
	payload := []byte("the quick brown fox jumps over the lazy dog — repeat: " +
		strings.Repeat("sercon-compress ", 32))
	for _, algo := range compressionAlgos {
		t.Run(algo, func(t *testing.T) {
			c, err := compressBytes(algo, payload)
			if err != nil {
				t.Fatalf("compress: %v", err)
			}
			if len(c) == 0 {
				t.Fatalf("compress: empty output")
			}
			out, err := decompressBytes(algo, c)
			if err != nil {
				t.Fatalf("decompress: %v", err)
			}
			if !bytes.Equal(out, payload) {
				t.Fatalf("round-trip mismatch (got %d bytes, want %d)", len(out), len(payload))
			}
		})
	}
}

// Unknown algorithm names must surface a clean error rather than panic or
// return empty bytes (which a caller would then mis-treat as success).
func TestCompression_UnknownAlgorithm(t *testing.T) {
	if _, err := compressBytes("nope", []byte("x")); err == nil {
		t.Fatal("compress: expected error for unknown algo")
	}
	if _, err := decompressBytes("nope", []byte("x")); err == nil {
		t.Fatal("decompress: expected error for unknown algo")
	}
}
