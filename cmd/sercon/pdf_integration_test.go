// cmd/sercon/pdf_integration_test.go
package main

import (
	"context"
	"strings"
	"testing"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

const samplePDF = "testdata/sample.pdf"

func skipNoPoppler(t *testing.T, bin string) {
	t.Helper()
	if !toolAvailable(bin) {
		t.Skipf("%s not on PATH — skipping poppler integration test", bin)
	}
}

// callArgs builds a FunctionCall from positional argument values (on a throwaway
// runtime; the ops only Export() their args).
func callArgs(vm *goja.Runtime, vs ...any) goja.FunctionCall {
	args := make([]goja.Value, len(vs))
	for i, v := range vs {
		args[i] = vm.ToValue(v)
	}
	return goja.FunctionCall{Arguments: args}
}

func TestPdfInfoOp(t *testing.T) {
	skipNoPoppler(t, "pdfinfo")
	vm := goja.New()
	got, err := pdfInfoOp(context.Background(), callArgs(vm, samplePDF))
	if err != nil {
		t.Fatalf("pdfInfoOp: %v", err)
	}
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("info returned %T, want map", got)
	}
	if m["pages"] != 1 {
		t.Fatalf("pages = %v, want 1", m["pages"])
	}
}

func TestPdfToImageOp_Bytes(t *testing.T) {
	skipNoPoppler(t, "pdftoppm")
	vm := goja.New()
	got, err := pdfToImageOp(context.Background(), callArgs(vm, samplePDF, map[string]any{
		"page": 1, "format": "png",
	}))
	if err != nil {
		t.Fatalf("pdfToImageOp: %v", err)
	}
	o, ok := got.(*scriptengine.Ordered)
	if !ok {
		t.Fatalf("toImage returned %T, want *scriptengine.Ordered", got)
	}
	bv, _ := o.Get("bytes")
	b, _ := bv.([]byte)
	// PNG magic: 0x89 'P' 'N' 'G'.
	if len(b) < 4 || b[0] != 0x89 || b[1] != 'P' || b[2] != 'N' || b[3] != 'G' {
		t.Fatalf("expected PNG magic, got %v (len %d)", b[:min(4, len(b))], len(b))
	}
}

func TestPdfToTextOp(t *testing.T) {
	skipNoPoppler(t, "pdftotext")
	vm := goja.New()
	got, err := pdfToTextOp(context.Background(), callArgs(vm, samplePDF))
	if err != nil {
		t.Fatalf("pdfToTextOp: %v", err)
	}
	txt, _ := got.(string)
	if !strings.Contains(txt, "Hello") {
		t.Fatalf("text %q should contain Hello", txt)
	}
}

func TestPdfToHTMLOp(t *testing.T) {
	skipNoPoppler(t, "pdftohtml")
	vm := goja.New()
	got, err := pdfToHTMLOp(context.Background(), callArgs(vm, samplePDF))
	if err != nil {
		t.Fatalf("pdfToHTMLOp: %v", err)
	}
	html, _ := got.(string)
	if !strings.Contains(strings.ToLower(html), "<html") {
		t.Fatalf("expected HTML output, got %q", html[:min(60, len(html))])
	}
}

func TestPdfVersionOp(t *testing.T) {
	skipNoPoppler(t, "pdftoppm")
	got, err := pdfVersionOp(context.Background(), goja.FunctionCall{})
	if err != nil {
		t.Fatalf("pdfVersionOp: %v", err)
	}
	v, _ := got.(string)
	if !strings.Contains(strings.ToLower(v), "version") {
		t.Fatalf("expected a version line, got %q", v)
	}
}
