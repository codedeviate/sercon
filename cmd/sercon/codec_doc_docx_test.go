// cmd/sercon/codec_doc_docx_test.go
package main

import "testing"

func TestDOCX_RoundTrip(t *testing.T) {
	in := []string{"First paragraph", "Second paragraph", "Third"}
	data, err := writeDOCX(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("empty docx")
	}
	m, err := readDOCX(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.format != "docx" {
		t.Fatalf("format = %q", m.format)
	}
	if len(m.paragraphs) != len(in) {
		t.Fatalf("got %d paragraphs, want %d: %v", len(m.paragraphs), len(in), m.paragraphs)
	}
	for i := range in {
		if m.paragraphs[i] != in[i] {
			t.Fatalf("paragraph %d = %q, want %q", i, m.paragraphs[i], in[i])
		}
	}
}

func TestReadDOCX_NotDocx(t *testing.T) {
	if _, err := readDOCX([]byte("PKnot really a docx")); err == nil {
		t.Fatal("expected error for non-DOCX input")
	}
}
