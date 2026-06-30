// cmd/sercon/codec_doc_rtf_test.go
package main

import (
	"strings"
	"testing"
)

func TestReadRTF_DestinationsAndEscapes(t *testing.T) {
	src := `{\rtf1\ansi\deff0 {\fonttbl{\f0 Times;}}\f0\fs24 Hello \b world\b0 .\par Second caf\'e9\par}`
	m, err := readRTF([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if m.format != "rtf" {
		t.Fatalf("format = %q", m.format)
	}
	// fonttbl content ("Times;") must NOT leak into the text.
	if got := m.paragraphs; len(got) != 2 {
		t.Fatalf("paragraphs = %v, want 2", got)
	}
	if m.paragraphs[0] != "Hello world ." && m.paragraphs[0] != "Hello world." {
		t.Fatalf("para0 = %q", m.paragraphs[0])
	}
	if m.paragraphs[1] != "Second café" {
		t.Fatalf("para1 = %q (want \"Second café\" — \\'e9 must decode via cp1252)", m.paragraphs[1])
	}
}

func TestRTF_RoundTrip(t *testing.T) {
	in := []string{"café €", "second {brace} line"}
	data, err := writeRTF(in)
	if err != nil {
		t.Fatal(err)
	}
	m, err := readRTF(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.paragraphs) != 2 {
		t.Fatalf("paragraphs = %v", m.paragraphs)
	}
	if m.paragraphs[0] != "café €" || m.paragraphs[1] != "second {brace} line" {
		t.Fatalf("round-trip = %v", m.paragraphs)
	}
}

func TestReadRTF_NotRtf(t *testing.T) {
	if _, err := readRTF([]byte("plain text")); err == nil {
		t.Fatal("expected error for non-RTF input")
	}
}

func TestReadRTF_UnicodeFallbackDelimiter(t *testing.T) {
	m, err := readRTF([]byte(`{\rtf1\ansi A\u352\par B}`))
	if err != nil {
		t.Fatal(err)
	}
	// \u352 decodes to U+0160 'Š'; the \par after it must NOT be eaten as the
	// \ucN fallback char (it's an RTF delimiter), so we still get 2 paragraphs.
	if len(m.paragraphs) != 2 || m.paragraphs[0] != "AŠ" || m.paragraphs[1] != "B" {
		t.Fatalf("paragraphs = %v, want [\"AŠ\" \"B\"]", m.paragraphs)
	}
}

func TestReadRTF_DeepNesting(t *testing.T) {
	src := "{\\rtf1" + strings.Repeat("{", 5000)
	if _, err := readRTF([]byte(src)); err == nil {
		t.Fatal("expected error for deeply nested RTF")
	}
}
