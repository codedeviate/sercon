// cmd/sercon/codec_sheet_delimited_test.go
package main

import (
	"reflect"
	"testing"
)

// MySQL tab dumps are unquoted, so a bare double-quote in a field (an inch
// mark like 2", common in a hardware catalog) must not abort the parse.
// encoding/csv without LazyQuotes throws "bare \" in non-quoted-field" on
// the whole file.
func TestReadDelimited_TSVBareQuotePreserved(t *testing.T) {
	data := []byte("art\tname\n7311\tStikk 2\" rund\n7312\tplain\n")
	book, err := readDelimited(data, '\t', "t")
	if err != nil {
		t.Fatalf("bare quote in TSV must not error, got: %v", err)
	}
	rows := book.tabs[0].rows
	want := [][]any{
		{"art", "name"},
		{"7311", `Stikk 2" rund`},
		{"7312", "plain"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Fatalf("rows = %#v, want %#v", rows, want)
	}
}

// A bare quote in a CSV field must likewise be tolerated rather than
// aborting the file.
func TestReadDelimited_CSVBareQuotePreserved(t *testing.T) {
	data := []byte("a,b\n12\" pipe,x\n")
	book, err := readDelimited(data, ',', "c")
	if err != nil {
		t.Fatalf("bare quote in CSV must not error, got: %v", err)
	}
	got := book.tabs[0].rows[1]
	want := []any{`12" pipe`, "x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("row = %#v, want %#v", got, want)
	}
}

// Well-formed RFC 4180 quoting must still parse correctly (quotes stripped,
// embedded comma preserved, doubled quote unescaped) — LazyQuotes only adds
// tolerance, it must not change valid-CSV semantics.
func TestReadDelimited_CSVQuotedFieldsStillParse(t *testing.T) {
	data := []byte("a,b\n\"hello, world\",\"she said \"\"hi\"\"\"\n")
	book, err := readDelimited(data, ',', "c")
	if err != nil {
		t.Fatalf("well-formed CSV must parse, got: %v", err)
	}
	got := book.tabs[0].rows[1]
	want := []any{"hello, world", `she said "hi"`}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("quoted row = %#v, want %#v", got, want)
	}
}
