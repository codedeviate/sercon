// cmd/sercon/codec_sheet_test.go
package main

import (
	"reflect"
	"testing"
)

func TestReadDelimited_CSV(t *testing.T) {
	book, err := readDelimited([]byte("a,b,c\n1,2,3\n"), ',', "data")
	if err != nil {
		t.Fatal(err)
	}
	if book.format != "csv" || len(book.tabs) != 1 || book.tabs[0].name != "data" {
		t.Fatalf("book = %+v", book)
	}
	want := [][]any{{"a", "b", "c"}, {"1", "2", "3"}}
	if !reflect.DeepEqual(book.tabs[0].rows, want) {
		t.Fatalf("rows = %#v", book.tabs[0].rows)
	}
}

func TestReadDelimited_TSV(t *testing.T) {
	book, err := readDelimited([]byte("x\ty\n4\t5\n"), '\t', "s")
	if err != nil {
		t.Fatal(err)
	}
	if book.format != "tsv" || book.tabs[0].rows[1][1] != "5" {
		t.Fatalf("tsv parse: %+v", book)
	}
}

func TestWriteDelimited_RoundTrip(t *testing.T) {
	book := sheetBook{format: "csv", tabs: []sheetTab{{name: "s", rows: [][]any{{"a", 1.0, true}, {"b", nil, false}}}}}
	data, err := writeDelimited(book, ',')
	if err != nil {
		t.Fatal(err)
	}
	// number/bool/nil stringified; empty cell is blank.
	if got := string(data); got != "a,1,true\nb,,false\n" {
		t.Fatalf("csv = %q", got)
	}
}

func TestWriteDelimited_MultiSheetThrows(t *testing.T) {
	book := sheetBook{tabs: []sheetTab{{name: "a"}, {name: "b"}}}
	if _, err := writeDelimited(book, ','); err == nil {
		t.Fatal("csv write with >1 sheet must error")
	}
}

func TestCellToStr(t *testing.T) {
	cases := map[any]string{"hi": "hi", true: "true", false: "false", nil: "", 42.0: "42", 3.5: "3.5", int64(7): "7"}
	for in, want := range cases {
		if got := cellToStr(in); got != want {
			t.Fatalf("cellToStr(%v) = %q want %q", in, got, want)
		}
	}
}

func TestXLSX_RoundTrip_Typed(t *testing.T) {
	book := sheetBook{format: "xlsx", tabs: []sheetTab{{name: "Data", rows: [][]any{
		{"Name", "Qty", "InStock"},
		{"Widget", 42.0, true},
		{"Gadget", 7.0, false},
	}}}}
	data, err := writeXLSX(book)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	back, err := readXLSX(data)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(back.tabs) != 1 || back.tabs[0].name != "Data" {
		t.Fatalf("tabs = %+v", back.tabs)
	}
	r := back.tabs[0].rows
	if r[1][1] != 42.0 {
		t.Fatalf("Qty cell = %#v (want number 42)", r[1][1])
	}
	if r[1][2] != true || r[2][2] != false {
		t.Fatalf("InStock cells = %#v / %#v (want bools)", r[1][2], r[2][2])
	}
	if r[0][0] != "Name" {
		t.Fatalf("header = %#v (want string)", r[0][0])
	}
}

func TestXLSX_MultiSheet(t *testing.T) {
	book := sheetBook{format: "xlsx", tabs: []sheetTab{
		{name: "First", rows: [][]any{{"a"}}},
		{name: "Second", rows: [][]any{{"b"}}},
	}}
	data, _ := writeXLSX(book)
	back, err := readXLSX(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.tabs) != 2 || back.tabs[0].name != "First" || back.tabs[1].name != "Second" {
		t.Fatalf("sheets = %+v", back.tabs)
	}
}
