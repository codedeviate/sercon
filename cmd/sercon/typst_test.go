package main

import (
	"reflect"
	"testing"
)

func TestInferTypstFormat(t *testing.T) {
	cases := map[string]string{"a.pdf": "pdf", "b.PNG": "png", "c.svg": "svg"}
	for p, want := range cases {
		got, err := inferTypstFormat(p)
		if err != nil || got != want {
			t.Fatalf("inferTypstFormat(%q)=%q,%v want %q", p, got, err, want)
		}
	}
	if _, err := inferTypstFormat("x.txt"); err == nil {
		t.Fatal("unknown ext should error")
	}
}

func TestValidateTypstInput(t *testing.T) {
	if err := validateTypstInput("", ""); err == nil {
		t.Fatal("neither should error")
	}
	if err := validateTypstInput("a.typ", "= x"); err == nil {
		t.Fatal("both should error")
	}
	if err := validateTypstInput("a.typ", ""); err != nil {
		t.Fatalf("input-only ok: %v", err)
	}
	if err := validateTypstInput("", "= x"); err != nil {
		t.Fatalf("source-only ok: %v", err)
	}
}

func TestBuildCompileArgs(t *testing.T) {
	got := buildCompileArgs(compileSpec{
		inputPath: "in.typ", outputPath: "out.pdf", format: "pdf",
		root: "/r", inputs: map[string]string{"b": "2", "a": "1"},
		ppi: 0, fontPaths: []string{"/f1", "/f2"},
	})
	want := []string{"compile", "--root", "/r", "--font-path", "/f1", "--font-path", "/f2",
		"--input", "a=1", "--input", "b=2", "--format", "pdf", "in.typ", "out.pdf"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("compile args:\n got %v\nwant %v", got, want)
	}
	// png includes --ppi
	g2 := buildCompileArgs(compileSpec{inputPath: "i", outputPath: "o.png", format: "png", ppi: 300})
	joined := ""
	for _, a := range g2 {
		joined += a + " "
	}
	if !typstContains(g2, "--ppi") || !typstContains(g2, "300") || !typstContains(g2, "--format") {
		t.Fatalf("png args missing ppi/format: %v", g2)
	}
}

func TestBuildQueryArgs(t *testing.T) {
	got := buildQueryArgs(querySpec{inputPath: "in.typ", selector: "<a>", field: "value", one: true,
		root: "/r", inputs: map[string]string{"k": "v"}})
	want := []string{"query", "--root", "/r", "--input", "k=v", "--field", "value", "--one", "in.typ", "<a>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query args:\n got %v\nwant %v", got, want)
	}
}

func typstContains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
