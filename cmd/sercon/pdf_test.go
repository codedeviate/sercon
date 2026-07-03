// cmd/sercon/pdf_test.go
package main

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestValidatePDFFormat(t *testing.T) {
	cases := map[string]string{"": "-png", "png": "-png", "PNG": "-png", "jpeg": "-jpeg", "tiff": "-tiff"}
	for in, want := range cases {
		got, err := validatePDFFormat(in)
		if err != nil || got != want {
			t.Fatalf("validatePDFFormat(%q)=%q,%v want %q", in, got, err, want)
		}
	}
	if _, err := validatePDFFormat("gif"); err == nil {
		t.Fatal("gif must be rejected")
	}
}

func TestParsePDFPages(t *testing.T) {
	type r struct{ f, l int }
	ok := map[string]r{"": {0, 0}, "3": {3, 3}, "1-5": {1, 5}}
	for in, want := range ok {
		f, l, err := parsePDFPages(in)
		if err != nil || f != want.f || l != want.l {
			t.Fatalf("parsePDFPages(%q)=%d,%d,%v want %d,%d", in, f, l, err, want.f, want.l)
		}
	}
	for _, bad := range []string{"0", "-1", "5-1", "a", "1-b", "1-2-3"} {
		if _, _, err := parsePDFPages(bad); err == nil {
			t.Fatalf("parsePDFPages(%q) should error", bad)
		}
	}
}

func TestBuildPdfImageArgs(t *testing.T) {
	got := buildPdfImageArgs(pdfImageSpec{src: "in.pdf", prefix: "out", format: "-png", firstPage: 1, lastPage: 1, dpi: 150})
	want := []string{"-png", "-f", "1", "-l", "1", "-r", "150", "--", "in.pdf", "out"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildPdfImageArgs:\n got %v\nwant %v", got, want)
	}
	// No page bounds, no dpi → just format + paths after "--".
	g2 := buildPdfImageArgs(pdfImageSpec{src: "-weird.pdf", prefix: "p", format: "-jpeg"})
	want2 := []string{"-jpeg", "--", "-weird.pdf", "p"}
	if !reflect.DeepEqual(g2, want2) {
		t.Fatalf("buildPdfImageArgs(no bounds):\n got %v\nwant %v", g2, want2)
	}
}

func TestBuildPdfTextArgs(t *testing.T) {
	got := buildPdfTextArgs(pdfTextSpec{src: "in.pdf", dest: "", firstPage: 1, lastPage: 3, layout: true})
	want := []string{"-f", "1", "-l", "3", "-layout", "--", "in.pdf", "-"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildPdfTextArgs:\n got %v\nwant %v", got, want)
	}
	g2 := buildPdfTextArgs(pdfTextSpec{src: "in.pdf", dest: "out.txt"})
	want2 := []string{"--", "in.pdf", "out.txt"}
	if !reflect.DeepEqual(g2, want2) {
		t.Fatalf("buildPdfTextArgs(dest):\n got %v\nwant %v", g2, want2)
	}
}

func TestBuildPdfHTMLArgs(t *testing.T) {
	got := buildPdfHTMLArgs(pdfHTMLSpec{src: "in.pdf", dest: "out.html", firstPage: 2, lastPage: 2})
	want := []string{"-i", "-noframes", "-f", "2", "-l", "2", "--", "in.pdf", "out.html"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildPdfHTMLArgs:\n got %v\nwant %v", got, want)
	}
}

// TestGlobGenerated_MetacharacterPrefix guards against a `dest` prefix that
// contains glob metacharacters (*, ?, [) corrupting the match: pdftoppm writes
// files named after the literal prefix on disk, but filepath.Glob would
// interpret those characters as patterns instead of literal text.
func TestGlobGenerated_MetacharacterPrefix(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "re[po]rt")
	want := []string{
		filepath.Join(dir, "re[po]rt-1.png"),
		filepath.Join(dir, "re[po]rt-2.png"),
	}
	decoys := []string{
		filepath.Join(dir, "re[po]rtX.png"), // no numeric suffix after "-"
		filepath.Join(dir, "re[po]rt-1.jpeg"), // wrong extension
		filepath.Join(dir, "other-1.png"),     // unrelated prefix
	}
	for _, p := range append(append([]string{}, want...), decoys...) {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", p, err)
		}
	}
	got, err := globGenerated(prefix, "png")
	if err != nil {
		t.Fatalf("globGenerated: %v", err)
	}
	wantSorted := append([]string{}, want...)
	sort.Strings(wantSorted)
	if !reflect.DeepEqual(got, wantSorted) {
		t.Fatalf("globGenerated(%q) = %v, want %v", prefix, got, wantSorted)
	}
}

// TestGlobGenerated_SingletonMetacharacterPrefix covers the single-page case
// (no "-N" suffix, just "<prefix>.<ext>") with a "?" metacharacter in prefix.
func TestGlobGenerated_SingletonMetacharacterPrefix(t *testing.T) {
	dir := t.TempDir()
	prefix := filepath.Join(dir, "p?age")
	singleton := filepath.Join(dir, "p?age.png")
	if err := os.WriteFile(singleton, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed %s: %v", singleton, err)
	}
	got, err := globGenerated(prefix, "png")
	if err != nil {
		t.Fatalf("globGenerated: %v", err)
	}
	want := []string{singleton}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("globGenerated(%q) = %v, want %v", prefix, got, want)
	}
}

func TestParsePdfInfo(t *testing.T) {
	out := "Title:          Demo\nPages:          3\nEncrypted:      no\nTagged:         yes\nPage size:      300 x 144 pts\nPDF version:    1.4\n"
	info := parsePdfInfo(out)
	if info["pages"] != 3 {
		t.Fatalf("pages = %v, want 3", info["pages"])
	}
	if info["title"] != "Demo" {
		t.Fatalf("title = %v, want Demo", info["title"])
	}
	if info["encrypted"] != false || info["tagged"] != true {
		t.Fatalf("encrypted/tagged = %v/%v, want false/true", info["encrypted"], info["tagged"])
	}
	if info["pageSize"] != "300 x 144 pts" || info["pdfVersion"] != "1.4" {
		t.Fatalf("pageSize/pdfVersion = %v/%v", info["pageSize"], info["pdfVersion"])
	}

	dated := parsePdfInfo("CreationDate:   Mon Jan  1 00:00:00 2024\nModDate:        Tue Jan  2 00:00:00 2024\n")
	if dated["creationDate"] != "Mon Jan  1 00:00:00 2024" {
		t.Fatalf("creationDate = %v", dated["creationDate"])
	}
	if dated["modDate"] != "Tue Jan  2 00:00:00 2024" {
		t.Fatalf("modDate = %v", dated["modDate"])
	}
}
