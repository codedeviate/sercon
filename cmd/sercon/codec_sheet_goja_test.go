// cmd/sercon/codec_sheet_goja_test.go
package main

import (
	"os"
	"path/filepath"
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

// A .tsv path with no explicit format must be read as tsv, not mis-parsed as
// csv (sniffSheetFormat returns csv for non-PK data; the .tsv ext must win).
func TestSheetGoja_TSVPathHonored(t *testing.T) {
	vm := sheetVM(t)
	dir := t.TempDir()
	tsvPath := filepath.Join(dir, "data.tsv")
	if err := os.WriteFile(tsvPath, []byte("a\tb\nc\td\n"), 0o644); err != nil {
		t.Fatalf("write temp tsv: %v", err)
	}
	if err := vm.Set("tsvPath", tsvPath); err != nil {
		t.Fatalf("set tsvPath: %v", err)
	}
	v, err := vm.RunString(`
		const back = sheet.read(tsvPath);   // no opts.format — must honor .tsv extension
		back.format + "|" + back.sheets[0].rows[0].length + "|" + back.sheets[0].rows[0][0] + "|" + back.sheets[0].rows[0][1];
	`)
	if err != nil {
		t.Fatalf("read .tsv path: %v", err)
	}
	// If mis-parsed as csv, the tab-delimited row would be a single cell "a\tb".
	if got := v.String(); got != "tsv|2|a|b" {
		t.Fatalf("got %q (want tsv|2|a|b — .tsv path was not honored)", got)
	}
}

// write(model) with no opts must reach the clean "format is required" error,
// not a goja "Cannot convert undefined to object" panic.
func TestSheetGoja_WriteNoOptsThrowsFormatRequired(t *testing.T) {
	vm := sheetVM(t)
	_, err := vm.RunString(`sheet.write([["a"]])`)
	if err == nil {
		t.Fatal("write with no opts should throw (format is required)")
	}
	if !strings.Contains(err.Error(), "format") {
		t.Fatalf("error should mention format, got: %v", err)
	}
}

// A non-primitive cell (object/array) must throw a clean TypeError rather than
// silently serializing Go-syntax garbage like "map[a:1]" or "[1 2]".
func TestSheetGoja_NonPrimitiveCellThrows(t *testing.T) {
	vm := sheetVM(t)
	if _, err := vm.RunString(`sheet.write([[ {a:1} ]], { format: "csv" })`); err == nil {
		t.Fatal("an object cell should throw")
	}
	if _, err := vm.RunString(`sheet.write([[ [1,2] ]], { format: "xlsx" })`); err == nil {
		t.Fatal("an array cell should throw")
	}
}

func TestSheetGoja_ODSRoundTrip(t *testing.T) {
	vm := sheetVM(t)
	v, err := vm.RunString(`
		const out = sheet.write({ sheets: [{ name: "S", rows: [["n","ok"],["a", true],["b", 3]] }] }, { format: "ods" });
		const back = sheet.read(out.bytes);   // sniffed as ods
		back.format + "|" + back.sheets[0].name + "|" + (typeof back.sheets[0].rows[1][1]) + "|" + (typeof back.sheets[0].rows[2][1]);
	`)
	if err != nil {
		t.Fatalf("ods round-trip: %v", err)
	}
	if got := v.String(); got != "ods|S|boolean|number" {
		t.Fatalf("got %q (want ods|S|boolean|number)", got)
	}
}
