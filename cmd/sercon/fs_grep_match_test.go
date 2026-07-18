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

func TestGrepFile_LiteralIgnoreCase(t *testing.T) {
	// Line 1: preceding multibyte rune ("café " = 6 bytes, 5 runes) guards that
	// Column is a 1-based RUNE offset.
	// Line 3: a preceding U+212A KELVIN SIGN (3 bytes) whose lowercase is "k"
	// (1 byte) — the exact case that broke the old ToLower-then-Index literal
	// path (byte offset from the lowered line no longer aligned to the original
	// line). Routed through the regex path now, offsets stay aligned.
	data := []byte("café TODO here\nnope\nK todo there\n")
	m, err := compileGrepMatcher("todo", true /*fixed*/, false, true /*ignoreCase*/, false)
	if err != nil {
		t.Fatal(err)
	}
	got, count := grepFile("f.txt", data, m, 0, 0, 0)
	if count != 2 || len(got) != 2 {
		t.Fatalf("ci literal: %+v count=%d", got, count)
	}
	// "café " is 5 runes, so the first match starts at rune column 6.
	if got[0].Line != 1 || got[0].Column != 6 || got[0].Match != "TODO" {
		t.Fatalf("hit0: %+v", got[0])
	}
	// "K " is 2 runes, so the second match starts at rune column 3, and
	// Match must be the un-corrupted original bytes "todo".
	if got[1].Line != 3 || got[1].Column != 3 || got[1].Match != "todo" {
		t.Fatalf("hit1: %+v", got[1])
	}
}

func TestGrepFile_Multiline(t *testing.T) {
	// (?s) makes "." span the newline, so "foo.bar" matches "foo\nbar".
	data := []byte("x\nfoo\nbar\ny\n")
	m, err := compileGrepMatcher("foo.bar", false, false, false, true /*multiline*/)
	if err != nil {
		t.Fatal(err)
	}
	got, count := grepFile("f.txt", data, m, 0, 0, 0)
	if count != 1 || len(got) != 1 {
		t.Fatalf("multiline: %+v count=%d", got, count)
	}
	if got[0].Line != 2 {
		t.Fatalf("multiline start line: %+v", got[0])
	}
	if got[0].Match != "foo\nbar" || got[0].Text != "foo\nbar" {
		t.Fatalf("multiline match span: %q", got[0].Match)
	}
}

func TestGrepFile_MaxMatchesCount(t *testing.T) {
	// 3 matching lines, capped at 1: returned slice holds 1, but count is 3
	// (the grepCount contract — count may exceed len(matches) when capped).
	data := []byte("hit a\nhit b\nhit c\n")
	m, _ := compileGrepMatcher("hit", true, false, false, false)
	got, count := grepFile("f.txt", data, m, 0, 0, 1 /*maxMatches*/)
	if len(got) != 1 || count != 3 {
		t.Fatalf("maxMatches cap: len=%d count=%d", len(got), count)
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
