package main

import (
	"strings"
	"testing"

	"github.com/saintfish/chardet"
	"golang.org/x/text/encoding/htmlindex"
)

// Round-trip a handful of representative encodings: pure-ASCII targets,
// a single-byte European encoding (Latin-1 / Windows-1252), and a
// multi-byte CJK one (Shift_JIS, GBK). The text in each test is chosen
// so every character has a representation in the target encoding.
func TestCharset_RoundTrip(t *testing.T) {
	cases := []struct {
		name, charset, text string
	}{
		{name: "utf-8", charset: "UTF-8", text: "hello world 1234 — UTF-8"},
		{name: "iso-8859-1", charset: "ISO-8859-1", text: "café crème — 1985"},
		{name: "windows-1252", charset: "Windows-1252", text: "smart “quotes” and €5"},
		{name: "shift-jis", charset: "Shift_JIS", text: "こんにちは"},
		{name: "gbk", charset: "GBK", text: "你好"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			enc, err := htmlindex.Get(c.charset)
			if err != nil {
				t.Fatalf("htmlindex.Get(%q): %v", c.charset, err)
			}
			encoded, err := enc.NewEncoder().Bytes([]byte(c.text))
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			decoded, err := enc.NewDecoder().Bytes(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if string(decoded) != c.text {
				t.Fatalf("round-trip mismatch:\n got %q\nwant %q", string(decoded), c.text)
			}
		})
	}
}

// Unknown charset names must surface a clean error from htmlindex.Get so
// scripts can react sensibly (try a fallback, surface to the user, …).
func TestCharset_UnknownCharset(t *testing.T) {
	if _, err := htmlindex.Get("totally-fake-encoding"); err == nil {
		t.Fatal("expected error for unknown charset")
	}
}

// chardet should identify a sufficiently large Latin-1 sample as a
// Western single-byte encoding. We don't pin the exact charset name —
// chardet may report ISO-8859-1, ISO-8859-2, or Windows-1252 depending
// on character frequencies — but the input should NOT be classified as
// UTF-8 because it contains byte 0xE9 (é in Latin-1) with no UTF-8
// continuation prefix.
func TestCharset_DetectLatin1NotUTF8(t *testing.T) {
	enc, _ := htmlindex.Get("ISO-8859-1")
	sample, err := enc.NewEncoder().Bytes([]byte(strings.Repeat("café crème — un éléphant marche dans la rue. ", 20)))
	if err != nil {
		t.Fatal(err)
	}
	// Use chardet directly so this test stays a pure-Go check without
	// spinning up a goja runtime.
	out, err := charsetDetectInline(sample)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.ToUpper(out["charset"].(string))
	if got == "UTF-8" {
		t.Errorf("expected non-UTF-8 detection, got %q", got)
	}
	if confidence, ok := out["confidence"].(int); ok && confidence < 20 {
		t.Errorf("low confidence: %d", confidence)
	}
}

// charsetDetectInline runs the same detection pipeline charsetDetect uses but
// without the goja shim — keeps the test offline and skips the
// PromisifyAsync round-trip.
func charsetDetectInline(in []byte) (map[string]any, error) {
	// reuse charsetDetect by faking a goja FunctionCall is more setup than
	// needed; call the upstream chardet directly with the same logic.
	results, err := chardet.NewTextDetector().DetectAll(in)
	if err != nil {
		return nil, err
	}
	top := results[0]
	return map[string]any{
		"charset":    top.Charset,
		"confidence": top.Confidence,
		"language":   top.Language,
	}, nil
}
