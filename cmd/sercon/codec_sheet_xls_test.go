// cmd/sercon/codec_sheet_xls_test.go
package main

import (
	"os"
	"testing"
)

// testdata/tiny.xls is a 2-sheet BIFF8 workbook generated once with xlwt
// (pure-Python, dev-time only — NOT a runtime dependency). The numeric cells
// use an explicit "general" number format because xlwt's DEFAULT integer XF
// makes extrame/xls misread them as dates; "general" matches what real Excel
// emits for plain numbers. Regenerate with:
//   python3 -m venv /tmp/xlsvenv && /tmp/xlsvenv/bin/pip install xlwt
//   /tmp/xlsvenv/bin/python - <<'PY'
//   import xlwt
//   wb = xlwt.Workbook(); g = xlwt.easyxf(num_format_str="general")
//   s1 = wb.add_sheet("Data")
//   for c,v in enumerate(["item","qty","active"]): s1.write(0,c,v)
//   s1.write(1,0,"apples"); s1.write(1,1,3,g); s1.write(1,2,"yes")
//   s1.write(2,0,"pears");  s1.write(2,1,5,g); s1.write(2,2,"no")
//   s2 = wb.add_sheet("Notes"); s2.write(0,0,"note"); s2.write(1,0,"second sheet")
//   wb.save("cmd/sercon/testdata/tiny.xls")
//   PY
func TestReadXLS(t *testing.T) {
	data, err := os.ReadFile("testdata/tiny.xls")
	if err != nil {
		t.Fatal(err)
	}
	book, err := readXLS(data)
	if err != nil {
		t.Fatal(err)
	}
	if book.format != "xls" {
		t.Fatalf("format = %q", book.format)
	}
	if len(book.tabs) != 2 || book.tabs[0].name != "Data" || book.tabs[1].name != "Notes" {
		t.Fatalf("tabs = %+v", book.tabs)
	}
	r := book.tabs[0].rows
	if r[0][0] != "item" || r[0][1] != "qty" || r[0][2] != "active" {
		t.Fatalf("Data header = %v", r[0])
	}
	if r[1][0] != "apples" || r[1][1] != "3" || r[1][2] != "yes" {
		t.Fatalf("Data row1 = %v", r[1])
	}
	if book.tabs[1].rows[1][0] != "second sheet" {
		t.Fatalf("Notes row1 = %v", book.tabs[1].rows[1])
	}
}

func TestReadXLS_NotXLS(t *testing.T) {
	if _, err := readXLS([]byte("not an OLE2 file")); err == nil {
		t.Fatal("expected error for non-XLS input")
	}
}
