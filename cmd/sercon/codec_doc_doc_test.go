// cmd/sercon/codec_doc_doc_test.go
package main

import (
	"os"
	"testing"
)

// readDOC is exercised only when a real .doc fixture is present: there is no
// pure-Go .doc writer to synthesize one (same precedent as the skip-gated
// audio MP3/OGG decode tests). Drop a real Word 97-2003 file at
// cmd/sercon/testdata/tiny.doc to activate this test. The probe verified
// doc2txt extracts real .doc content, so the wrapper risk is low.
func TestReadDOC(t *testing.T) {
	const path = "testdata/tiny.doc"
	if _, err := os.Stat(path); err != nil {
		t.Skip("no testdata/tiny.doc fixture; drop a real .doc to activate")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := readDOC(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.format != "doc" || m.text == "" {
		t.Fatalf("got %+v", m)
	}
}

func TestReadDOC_NotDoc(t *testing.T) {
	if _, err := readDOC([]byte("not an OLE2 doc")); err == nil {
		t.Fatal("expected error for non-DOC input")
	}
}
