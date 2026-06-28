// cmd/sercon/codec_sheet_dif_test.go
package main

import "testing"

const sampleDIF = "TABLE\n0,1\n\"sercon\"\n" +
	"VECTORS\n0,2\n\"\"\n" +
	"TUPLES\n0,3\n\"\"\n" +
	"DATA\n0,0\n\"\"\n" +
	"-1,0\nBOT\n1,0\n\"Name\"\n1,0\n\"Qty\"\n" +
	"-1,0\nBOT\n1,0\n\"apples\"\n0,3\nV\n" +
	"-1,0\nBOT\n1,0\n\"pears\"\n0,5\nV\n" +
	"-1,0\nEOD\n"

func TestReadDIF(t *testing.T) {
	book, err := readDIF([]byte(sampleDIF), "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if book.format != "dif" || len(book.tabs) != 1 {
		t.Fatalf("book = %+v", book)
	}
	rows := book.tabs[0].rows
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3 (%v)", len(rows), rows)
	}
	if rows[0][0] != "Name" || rows[0][1] != "Qty" {
		t.Fatalf("header = %v", rows[0])
	}
	if rows[1][0] != "apples" || rows[1][1] != float64(3) {
		t.Fatalf("row1 = %v", rows[1])
	}
	if rows[2][0] != "pears" {
		t.Fatalf("row2 name = %v", rows[2][0])
	}
	if rows[2][1] != float64(5) {
		t.Fatalf("row2 qty = %v", rows[2][1])
	}
}

func TestReadDIF_BoolAndNA(t *testing.T) {
	src := "TABLE\n0,1\n\"\"\nDATA\n0,0\n\"\"\n" +
		"-1,0\nBOT\n0,0\nTRUE\n0,0\nNA\n-1,0\nEOD\n"
	book, err := readDIF([]byte(src), "S")
	if err != nil {
		t.Fatal(err)
	}
	r := book.tabs[0].rows[0]
	if r[0] != true || r[1] != nil {
		t.Fatalf("row = %v (want [true <nil>])", r)
	}
}

func TestReadDIF_NotDif(t *testing.T) {
	if _, err := readDIF([]byte("a,b,c\n"), "x"); err == nil {
		t.Fatal("expected error for non-DIF input (no DATA section)")
	}
}
