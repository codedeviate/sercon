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
		"--input", "a=1", "--input", "b=2", "--format", "pdf", "--", "in.typ", "out.pdf"}
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
	want := []string{"query", "--root", "/r", "--input", "k=v", "--field", "value", "--one", "--", "in.typ", "<a>"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query args:\n got %v\nwant %v", got, want)
	}
}

// TestBuildCompileArgs_DashSeparator guards against a path/output starting
// with "-" being parsed as a flag: "--" must sit immediately before the
// positional inputPath/outputPath pair, regardless of which options precede it.
func TestBuildCompileArgs_DashSeparator(t *testing.T) {
	tests := []struct {
		name string
		spec compileSpec
	}{
		{"bare", compileSpec{inputPath: "-weird.typ", outputPath: "-out.pdf", format: "pdf"}},
		{"withOpts", compileSpec{
			inputPath: "-in.typ", outputPath: "-out.png", format: "png",
			root: "/r", inputs: map[string]string{"k": "v"}, ppi: 300, fontPaths: []string{"/f1"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := buildCompileArgs(tc.spec)
			idx := typstIndexOf(args, "--")
			if idx == -1 {
				t.Fatalf("buildCompileArgs(%+v): missing \"--\": %v", tc.spec, args)
			}
			if idx+2 != len(args)-1 && idx+1 >= len(args) {
				t.Fatalf("buildCompileArgs(%+v): malformed argv: %v", tc.spec, args)
			}
			if args[idx+1] != tc.spec.inputPath {
				t.Fatalf("buildCompileArgs(%+v): \"--\" not immediately before inputPath: %v", tc.spec, args)
			}
			if args[len(args)-1] != tc.spec.outputPath {
				t.Fatalf("buildCompileArgs(%+v): outputPath not last: %v", tc.spec, args)
			}
		})
	}
}

// TestBuildQueryArgs_DashSeparator mirrors TestBuildCompileArgs_DashSeparator
// for `typst query`'s positional inputPath/selector pair.
func TestBuildQueryArgs_DashSeparator(t *testing.T) {
	tests := []struct {
		name string
		spec querySpec
	}{
		{"bare", querySpec{inputPath: "-weird.typ", selector: "-suspicious"}},
		{"withOpts", querySpec{
			inputPath: "-in.typ", selector: "-<a>", field: "value", one: true,
			root: "/r", inputs: map[string]string{"k": "v"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := buildQueryArgs(tc.spec)
			idx := typstIndexOf(args, "--")
			if idx == -1 {
				t.Fatalf("buildQueryArgs(%+v): missing \"--\": %v", tc.spec, args)
			}
			if args[idx+1] != tc.spec.inputPath {
				t.Fatalf("buildQueryArgs(%+v): \"--\" not immediately before inputPath: %v", tc.spec, args)
			}
			if args[len(args)-1] != tc.spec.selector {
				t.Fatalf("buildQueryArgs(%+v): selector not last: %v", tc.spec, args)
			}
		})
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

func typstIndexOf(xs []string, s string) int {
	for i, x := range xs {
		if x == s {
			return i
		}
	}
	return -1
}
