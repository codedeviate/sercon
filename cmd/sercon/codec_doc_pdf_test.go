// cmd/sercon/codec_doc_pdf_test.go
package main

import (
	"os"
	"strings"
	"testing"
)

// testdata/tiny.pdf was generated once at dev time with github.com/go-pdf/fpdf
// (dev-tool only — NOT a runtime dependency):
//
//	pdf := fpdf.New("P","mm","A4","")
//	pdf.AddPage(); pdf.SetFont("Helvetica","",14)
//	pdf.Cell(40,10,"sercon document codec fixture"); pdf.Ln(10)
//	pdf.Cell(40,10,"Hello from tiny.pdf")
//	pdf.OutputFileAndClose("cmd/sercon/testdata/tiny.pdf")
func TestReadPDF(t *testing.T) {
	data, err := os.ReadFile("testdata/tiny.pdf")
	if err != nil {
		t.Fatal(err)
	}
	m, err := readPDF(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.format != "pdf" {
		t.Fatalf("format = %q", m.format)
	}
	if !strings.Contains(m.text, "sercon document codec fixture") || !strings.Contains(m.text, "Hello from tiny.pdf") {
		t.Fatalf("text missing expected content: %q", m.text)
	}
	if len(m.paragraphs) == 0 {
		t.Fatalf("expected at least one paragraph")
	}
}

func TestReadPDF_NotPDF(t *testing.T) {
	if _, err := readPDF([]byte("not a pdf")); err == nil {
		t.Fatal("expected error for non-PDF input")
	}
}
