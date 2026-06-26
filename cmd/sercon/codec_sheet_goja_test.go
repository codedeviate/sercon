// cmd/sercon/codec_sheet_goja_test.go
package main

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func sheetVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	ns := sheetNamespace(vm)
	obj := vm.NewObject()
	for k, v := range ns {
		_ = obj.Set(k, v)
	}
	_ = vm.Set("sheet", obj)
	return vm
}

func TestSheetGoja_XLSXRoundTrip(t *testing.T) {
	vm := sheetVM(t)
	v, err := vm.RunString(`
		const out = sheet.write({ sheets: [{ name: "S", rows: [["n", "qty"], ["a", 5]] }] }, { format: "xlsx" });
		const back = sheet.read(out.bytes);   // sniffed as xlsx
		back.format + "|" + back.sheets.length + "|" + back.sheets[0].name + "|" + (typeof back.sheets[0].rows[1][1]);
	`)
	if err != nil {
		t.Fatalf("xlsx round-trip: %v", err)
	}
	if got := v.String(); got != "xlsx|1|S|number" {
		t.Fatalf("got %q (want xlsx|1|S|number)", got)
	}
}

func TestSheetGoja_BareArrayCSV(t *testing.T) {
	vm := sheetVM(t)
	// Write the CSV and get back format + bytes.
	v, err := vm.RunString(`sheet.write([["a","b"],["c","d"]], { format: "csv" })`)
	if err != nil {
		t.Fatalf("bare array: %v", err)
	}
	obj := v.ToObject(vm)
	if obj.Get("format").String() != "csv" {
		t.Fatalf("expected format=csv, got %q", obj.Get("format").String())
	}
	rawBytes, ok := obj.Get("bytes").Export().([]byte)
	if !ok {
		t.Fatalf("bytes field is not []byte: %T", obj.Get("bytes").Export())
	}
	csvStr := strings.TrimSpace(string(rawBytes))
	if !strings.HasPrefix(csvStr, "a,b") {
		t.Fatalf("csv content unexpected: %q", csvStr)
	}
}

func TestSheetGoja_CSVMultiSheetThrows(t *testing.T) {
	vm := sheetVM(t)
	_, err := vm.RunString(`sheet.write({ sheets: [{ rows: [["a"]] }, { rows: [["b"]] }] }, { format: "csv" })`)
	if err == nil {
		t.Fatal("csv write with 2 sheets should throw")
	}
}

func TestSheetGoja_EmptySheetModelThrows(t *testing.T) {
	vm := sheetVM(t)
	// object with empty sheets array
	_, err := vm.RunString(`sheet.write({ sheets: [] }, { format: "csv" })`)
	if err == nil {
		t.Fatal("write with empty sheets array should throw")
	}
	// bare empty array
	_, err = vm.RunString(`sheet.write([], { format: "csv" })`)
	if err == nil {
		t.Fatal("write with bare empty array should throw")
	}
}
