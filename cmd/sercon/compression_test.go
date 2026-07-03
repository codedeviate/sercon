package main

import (
	"bytes"
	"compress/gzip"
	"strings"
	"testing"

	"github.com/golang/snappy"
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
			out, err := decompressBytes(algo, c, 0)
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
	if _, err := decompressBytes("nope", []byte("x"), 0); err == nil {
		t.Fatal("decompress: expected error for unknown algo")
	}
}

// A small crafted gzip blob can inflate to gigabytes (a "decompression
// bomb"); decompressBytes must cap the output and error rather than
// io.ReadAll-ing an unbounded amount into memory. Zeros compress extremely
// well, so a few MiB of them makes a tiny compressed blob that still
// exceeds a deliberately small maxBytes.
func TestCompression_DecompressMaxBytes(t *testing.T) {
	payload := bytes.Repeat([]byte{0}, 4<<20) // 4 MiB of zeros
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	compressed := buf.Bytes()

	if _, err := decompressBytes("gzip", compressed, 1<<20); err == nil {
		t.Fatal("expected error for output exceeding maxBytes")
	} else if !strings.Contains(err.Error(), "exceeds maxBytes limit") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Exactly at the cap must succeed — off-by-one boundary correctness.
	out, err := decompressBytes("gzip", compressed, int64(len(payload)))
	if err != nil {
		t.Fatalf("decompress at exact cap: %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Fatal("decompress at exact cap: mismatch")
	}

	// A normal small payload still round-trips under the default cap (0 ->
	// DefaultMaxDecompressBytes).
	small := []byte("hello world")
	var sbuf bytes.Buffer
	sgz := gzip.NewWriter(&sbuf)
	if _, err := sgz.Write(small); err != nil {
		t.Fatalf("gzip write small: %v", err)
	}
	if err := sgz.Close(); err != nil {
		t.Fatalf("gzip close small: %v", err)
	}
	out2, err := decompressBytes("gzip", sbuf.Bytes(), 0)
	if err != nil {
		t.Fatalf("decompress small: %v", err)
	}
	if !bytes.Equal(out2, small) {
		t.Fatal("small payload mismatch")
	}
}

// snappy has no streaming reader — decompressBytes must guard it via the
// format's own length header (snappy.DecodedLen) rather than a
// io.LimitReader, so it needs its own cap test.
func TestCompression_DecompressMaxBytes_Snappy(t *testing.T) {
	payload := bytes.Repeat([]byte{0}, 4<<20)
	compressed := snappy.Encode(nil, payload)

	if _, err := decompressBytes("snappy", compressed, 1<<20); err == nil {
		t.Fatal("expected error for snappy output exceeding maxBytes")
	} else if !strings.Contains(err.Error(), "exceeds maxBytes limit") {
		t.Fatalf("unexpected error: %v", err)
	}

	out, err := decompressBytes("snappy", compressed, int64(len(payload)))
	if err != nil {
		t.Fatalf("decompress snappy at exact cap: %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Fatal("snappy mismatch")
	}
}
