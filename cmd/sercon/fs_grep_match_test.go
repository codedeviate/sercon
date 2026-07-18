package main

import "testing"

func TestGrepFile_LiteralAndRegex(t *testing.T) {
	data := []byte("alpha TODO one\nbeta\ngamma todo two\n")

	m, err := compileGrepMatcher("TODO", true /*fixed*/, false, false /*ignoreCase*/, false)
	if err != nil {
		t.Fatal(err)
	}
	got, count := grepFile("f.txt", data, m, 0, 0, 0)
	if count != 1 || len(got) != 1 || got[0].Line != 1 || got[0].Column != 7 || got[0].Text != "alpha TODO one" {
		t.Fatalf("literal: %+v count=%d", got, count)
	}

	// Smart-case-off regex, case-insensitive matches both TODO and todo.
	mi, err := compileGrepMatcher("todo", false, false, true /*ignoreCase*/, false)
	if err != nil {
		t.Fatal(err)
	}
	got, count = grepFile("f.txt", data, mi, 0, 0, 0)
	if count != 2 || len(got) != 2 {
		t.Fatalf("ci regex: %+v count=%d", got, count)
	}
}

func TestGrepFile_Context(t *testing.T) {
	data := []byte("l1\nl2\nHIT\nl4\nl5\n")
	m, _ := compileGrepMatcher("HIT", true, false, false, false)
	got, _ := grepFile("f.txt", data, m, 1 /*before*/, 1 /*after*/, 0)
	if len(got) != 1 || len(got[0].Before) != 1 || got[0].Before[0] != "l2" || got[0].After[0] != "l4" {
		t.Fatalf("context: %+v", got)
	}
}

func TestIsBinary(t *testing.T) {
	if !isBinary([]byte("abc\x00def")) {
		t.Fatal("NUL should be binary")
	}
	if isBinary([]byte("plain text")) {
		t.Fatal("text should not be binary")
	}
}
