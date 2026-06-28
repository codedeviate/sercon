// cmd/sercon/codec_sheet_slk_test.go
package main

import "testing"

const sampleSYLK = "ID;PWXL;N;E\n" +
	"B;Y3;X2\n" +
	"C;Y1;X1;K\"Name\"\n" +
	"C;Y1;X2;K\"Qty\"\n" +
	"C;Y2;X1;K\"apples\"\n" +
	"C;Y2;X2;K3\n" +
	"C;Y3;X1;K\"pears\"\n" +
	"C;Y3;X2;K5\n" +
	"E\n"

func TestReadSYLK(t *testing.T) {
	book, err := readSYLK([]byte(sampleSYLK), "Sheet1")
	if err != nil {
		t.Fatal(err)
	}
	if book.format != "slk" || len(book.tabs) != 1 || book.tabs[0].name != "Sheet1" {
		t.Fatalf("book = %+v", book)
	}
	rows := book.tabs[0].rows
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0][0] != "Name" || rows[0][1] != "Qty" {
		t.Fatalf("header = %v", rows[0])
	}
	if rows[1][0] != "apples" || rows[1][1] != float64(3) {
		t.Fatalf("row1 = %v (types %T,%T)", rows[1], rows[1][0], rows[1][1])
	}
	if rows[2][0] != "pears" || rows[2][1] != float64(5) {
		t.Fatalf("row2 = %v", rows[2])
	}
}

func TestReadSYLK_DoubledQuote(t *testing.T) {
	// SYLK escapes a literal '"' inside a quoted value by doubling it.
	src := "ID;P\nC;Y1;X1;K\"say \"\"hi\"\"\"\nE\n"
	book, err := readSYLK([]byte(src), "S")
	if err != nil {
		t.Fatal(err)
	}
	if book.tabs[0].rows[0][0] != `say "hi"` {
		t.Fatalf("unescaped = %q, want %q", book.tabs[0].rows[0][0], `say "hi"`)
	}
}

func TestReadSYLK_NotSylk(t *testing.T) {
	if _, err := readSYLK([]byte("just,a,csv\n1,2,3\n"), "x"); err == nil {
		t.Fatal("expected error for non-SYLK input (no ID record)")
	}
}

func TestReadSYLK_EscapedSemicolon(t *testing.T) {
	// ;; is an escaped literal ';' inside a field.
	src := "ID;P\nC;Y1;X1;K\"a;;b\"\nE\n"
	book, err := readSYLK([]byte(src), "S")
	if err != nil {
		t.Fatal(err)
	}
	if book.tabs[0].rows[0][0] != "a;b" {
		t.Fatalf("unescaped = %q, want %q", book.tabs[0].rows[0][0], "a;b")
	}
}
