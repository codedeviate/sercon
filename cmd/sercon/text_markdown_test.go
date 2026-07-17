// cmd/sercon/text_markdown_test.go
package main

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func markdownVM(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	obj := vm.NewObject()
	for k, v := range textMarkdownNamespace(vm) {
		_ = obj.Set(k, v)
	}
	_ = vm.Set("markdown", obj)
	return vm
}

func TestMarkdownToHtmlBasic(t *testing.T) {
	vm := markdownVM(t)
	v, err := vm.RunString("markdown.toHtml('# Hi\\n\\n- a\\n- b');")
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if !strings.Contains(got, "<h1>Hi</h1>") {
		t.Fatalf("missing <h1>: %q", got)
	}
	if !strings.Contains(got, "<ul>") || !strings.Contains(got, "<li>a</li>") {
		t.Fatalf("missing list: %q", got)
	}
}

// GFM is on by default: a pipe table renders to <table>.
func TestMarkdownToHtmlGFMTable(t *testing.T) {
	vm := markdownVM(t)
	src := "| h1 | h2 |\\n| -- | -- |\\n| a | b |\\n"
	v, err := vm.RunString("markdown.toHtml('" + src + "');")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); !strings.Contains(got, "<table>") {
		t.Fatalf("GFM table not rendered: %q", got)
	}
}

// gfm:false disables the table extension — a pipe table stays a paragraph.
func TestMarkdownToHtmlGFMDisabled(t *testing.T) {
	vm := markdownVM(t)
	src := "| h1 | h2 |\\n| -- | -- |\\n| a | b |\\n"
	v, err := vm.RunString("markdown.toHtml('" + src + "', { gfm: false });")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); strings.Contains(got, "<table>") {
		t.Fatalf("gfm:false should not render a table: %q", got)
	}
}

// hardBreaks maps a single newline to <br>.
func TestMarkdownToHtmlHardBreaks(t *testing.T) {
	vm := markdownVM(t)
	v, err := vm.RunString("markdown.toHtml('a\\nb', { hardBreaks: true });")
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); !strings.Contains(got, "<br>") {
		t.Fatalf("hardBreaks should emit <br>: %q", got)
	}
}
