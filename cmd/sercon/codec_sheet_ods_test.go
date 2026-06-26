// cmd/sercon/codec_sheet_ods_test.go
package main

import (
	"testing"
)

// odsContent wraps a body of <table:table> XML into a full content.xml.
func odsContent(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>` +
		`<office:document-content ` +
		`xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" ` +
		`xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" ` +
		`xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">` +
		`<office:body><office:spreadsheet>` + body +
		`</office:spreadsheet></office:body></office:document-content>`
}

// odsBytes builds a minimal ODS package (mimetype + content.xml) from a content body.
func odsBytes(t *testing.T, body string) []byte {
	t.Helper()
	return makeZip(t, [][2]string{
		{"mimetype", "application/vnd.oasis.opendocument.spreadsheet"},
		{"content.xml", odsContent(body)},
	}, true)
}

func TestReadODS_TypedCells(t *testing.T) {
	body := `<table:table table:name="Data">` +
		`<table:table-row>` +
		`<table:table-cell office:value-type="string"><text:p>Name</text:p></table:table-cell>` +
		`<table:table-cell office:value-type="float" office:value="42"><text:p>42</text:p></table:table-cell>` +
		`<table:table-cell office:value-type="boolean" office:boolean-value="true"><text:p>TRUE</text:p></table:table-cell>` +
		`<table:table-cell office:value-type="date" office:date-value="2026-06-26"><text:p>06/26/26</text:p></table:table-cell>` +
		`<table:table-cell office:value-type="boolean" office:boolean-value="false"><text:p>FALSE</text:p></table:table-cell>` +
		`</table:table-row>` +
		`</table:table>`
	book, err := readODS(odsBytes(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if book.format != "ods" || len(book.tabs) != 1 || book.tabs[0].name != "Data" {
		t.Fatalf("book = %+v", book)
	}
	r := book.tabs[0].rows[0]
	if r[0] != "Name" {
		t.Errorf("cell0 = %#v, want string Name", r[0])
	}
	if r[1] != 42.0 {
		t.Errorf("cell1 = %#v, want number 42", r[1])
	}
	if r[2] != true {
		t.Errorf("cell2 = %#v, want bool true", r[2])
	}
	if r[3] != "2026-06-26" {
		t.Errorf("cell3 = %#v, want ISO date string 2026-06-26", r[3])
	}
	if r[4] != false {
		t.Errorf("cell4 = %#v, want bool false", r[4])
	}
}

func TestReadODS_TrailingBlankRowHugeRepeat(t *testing.T) {
	// One data row, then a LibreOffice-style trailing blank pad with a huge
	// number-rows-repeated. The pad is pure padding and must not error.
	body := `<table:table table:name="S">` +
		`<table:table-row>` +
		`<table:table-cell office:value-type="string"><text:p>a</text:p></table:table-cell>` +
		`</table:table-row>` +
		`<table:table-row table:number-rows-repeated="1048576"><table:table-cell/></table:table-row>` +
		`</table:table>`
	book, err := readODS(odsBytes(t, body))
	if err != nil {
		t.Fatalf("trailing blank pad must not error: %v", err)
	}
	rows := book.tabs[0].rows
	if len(rows) != 1 {
		t.Fatalf("rows = %d (%#v), want 1 (pad trimmed)", len(rows), rows)
	}
	if rows[0][0] != "a" {
		t.Errorf("rows[0][0] = %#v, want \"a\"", rows[0][0])
	}
}

func TestReadODS_RowRepeatExpands(t *testing.T) {
	body := `<table:table table:name="S">` +
		`<table:table-row table:number-rows-repeated="3">` +
		`<table:table-cell office:value-type="string"><text:p>x</text:p></table:table-cell>` +
		`</table:table-row>` +
		`</table:table>`
	book, err := readODS(odsBytes(t, body))
	if err != nil {
		t.Fatal(err)
	}
	rows := book.tabs[0].rows
	if len(rows) != 3 {
		t.Fatalf("rows = %d (%#v), want 3 identical rows", len(rows), rows)
	}
	for i, row := range rows {
		if len(row) != 1 || row[0] != "x" {
			t.Errorf("rows[%d] = %#v, want [\"x\"]", i, row)
		}
	}
}

func TestReadODS_RepeatTrimAndInterior(t *testing.T) {
	// "a", one interior blank, "b", then a huge trailing blank pad.
	body := `<table:table table:name="S"><table:table-row>` +
		`<table:table-cell office:value-type="string"><text:p>a</text:p></table:table-cell>` +
		`<table:table-cell/>` +
		`<table:table-cell office:value-type="string"><text:p>b</text:p></table:table-cell>` +
		`<table:table-cell table:number-columns-repeated="1014"/>` +
		`</table:table-row></table:table>`
	book, err := readODS(odsBytes(t, body))
	if err != nil {
		t.Fatal(err)
	}
	row := book.tabs[0].rows[0]
	want := []any{"a", nil, "b"}
	if len(row) != len(want) {
		t.Fatalf("row len = %d (%#v), want 3 (trailing pad trimmed)", len(row), row)
	}
	for i := range want {
		if row[i] != want[i] {
			t.Errorf("row[%d] = %#v, want %#v", i, row[i], want[i])
		}
	}
}

func TestReadODS_RepeatCapExceeded(t *testing.T) {
	body := `<table:table table:name="S"><table:table-row>` +
		`<table:table-cell office:value-type="string"><text:p>x</text:p></table:table-cell>` +
		`<table:table-cell office:value-type="string" table:number-columns-repeated="99999999"><text:p>y</text:p></table:table-cell>` +
		`</table:table-row></table:table>`
	if _, err := readODS(odsBytes(t, body)); err == nil {
		t.Fatal("expected error when a repeat count exceeds the cap")
	}
}

func TestReadODS_MissingContent(t *testing.T) {
	z := makeZip(t, [][2]string{{"mimetype", "application/vnd.oasis.opendocument.spreadsheet"}}, true)
	if _, err := readODS(z); err == nil {
		t.Fatal("expected error when content.xml is absent")
	}
}
