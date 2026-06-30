// cmd/sercon/codec_doc_goja_test.go
package main

import (
	"os"
	"testing"

	"github.com/dop251/goja"
)

func docVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	obj := vm.NewObject()
	for k, v := range docNamespace(vm) {
		_ = obj.Set(k, v)
	}
	_ = vm.Set("doc", obj)
	return vm
}

func TestDocGoja_Formats(t *testing.T) {
	vm := docVM(t)
	v, err := vm.RunString(`
		const f = doc.formats();
		[f.pdf.read, f.pdf.write, f.docx.write, f.rtf.write, f.odt.write, f.doc.write].join(",");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "true,false,true,true,true,false" {
		t.Fatalf("got %q", v.String())
	}
}

func TestDocGoja_DocxRoundTrip(t *testing.T) {
	vm := docVM(t)
	v, err := vm.RunString(`
		const out = doc.write({ paragraphs: ["alpha", "beta"] }, { format: "docx" });
		const back = doc.read(out.bytes, { format: "docx" });
		back.format + "|" + back.paragraphs.length + "|" + back.paragraphs[0] + "|" + back.paragraphs[1];
	`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "docx|2|alpha|beta" {
		t.Fatalf("got %q", v.String())
	}
}

func TestDocGoja_ReadPDF(t *testing.T) {
	vm := docVM(t)
	data, err := os.ReadFile("testdata/tiny.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if err := vm.Set("pdfb", data); err != nil {
		t.Fatal(err)
	}
	v, err := vm.RunString(`const d = doc.read(pdfb); d.format + "|" + (d.text.indexOf("Hello from tiny.pdf") >= 0);`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "pdf|true" {
		t.Fatalf("got %q", v.String())
	}
}

func TestDocGoja_WriteReadOnlyThrows(t *testing.T) {
	vm := docVM(t)
	if _, err := vm.RunString(`doc.write({ text: "x" }, { format: "pdf" })`); err == nil {
		t.Fatal("write to pdf must throw (read-only)")
	}
	if _, err := vm.RunString(`doc.write({ text: "x" }, { dest: "/tmp/x.doc" })`); err == nil {
		t.Fatal("write to .doc dest must throw (read-only)")
	}
}

func TestDocGoja_NullParagraphNoNilLeak(t *testing.T) {
	vm := docVM(t)
	// A JS null paragraph element must not leak Go's "<nil>" into the output.
	v, err := vm.RunString(`
		const out = doc.write({ paragraphs: ["a", null, "b"] }, { format: "rtf" });
		const back = doc.read(out.bytes, { format: "rtf" });
		back.paragraphs.join("|");
	`)
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got == "a|<nil>|b" || got != "a|b" {
		t.Fatalf("got %q (want \"a|b\" — null paragraph dropped, no <nil> leak)", got)
	}
}
