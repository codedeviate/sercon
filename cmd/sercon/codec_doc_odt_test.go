// cmd/sercon/codec_doc_odt_test.go
package main

import "testing"

func TestODT_RoundTrip(t *testing.T) {
	in := []string{"First & only", "Second <b>para</b>"}
	data, err := writeODT(in)
	if err != nil {
		t.Fatal(err)
	}
	m, err := readODT(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.format != "odt" {
		t.Fatalf("format = %q", m.format)
	}
	if len(m.paragraphs) != 2 || m.paragraphs[0] != "First & only" || m.paragraphs[1] != "Second <b>para</b>" {
		t.Fatalf("round-trip = %v (escaping must survive)", m.paragraphs)
	}
}

func TestReadODT_NotOdt(t *testing.T) {
	if _, err := readODT([]byte("not a zip")); err == nil {
		t.Fatal("expected error for non-ODT input")
	}
}
