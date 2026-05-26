package main

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestStrNamespace exercises every `api.str.*` member against known vectors
// driven through a real Engine + Run.
func TestStrNamespace(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterNamespaceFactory("str", func(vm *goja.Runtime, _ *eventloop.EventLoop) map[string]any {
		return strNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}

	script := `
function eq(label, got, want) {
  if (got !== want) throw new Error(label + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want));
}

// trim / ltrim / rtrim with default mask + custom mask
eq("trim default",  str.trim("  abc  "),       "abc");
eq("trim custom",   str.trim("///abc///", "/"), "abc");
eq("ltrim default", str.ltrim("  abc"),         "abc");
eq("rtrim default", str.rtrim("abc  "),         "abc");
eq("ltrim mask",    str.ltrim("___ab", "_"),    "ab");
eq("rtrim mask",    str.rtrim("ab___", "_"),    "ab");

// reverse: rune-aware
eq("reverse ascii",   str.reverse("hello"), "olleh");
eq("reverse unicode", str.reverse("café"),  "éfac");

// stripHtml
eq("stripHtml",       str.stripHtml("<p>hi <b>there</b></p>"), "hi there");

// nl2br / br2nl
eq("nl2br",       str.nl2br("a\nb\nc"),               "a<br>\nb<br>\nc");
eq("nl2br xhtml", str.nl2br("a\nb", true),            "a<br/>\nb");
eq("br2nl",       str.br2nl("a<br>b<br/>c<BR />d"),   "a\nb\nc\nd");

// base64 round trip
eq("b64 encode", str.base64Encode("hello"),    "aGVsbG8=");
eq("b64 decode", str.base64Decode("aGVsbG8="), "hello");

// url round trip (form-style: '+' for space)
eq("url encode",      str.urlEncode("a b/c"),  "a+b%2Fc");
eq("url decode plus", str.urlDecode("a+b"),    "a b");
eq("url decode pct",  str.urlDecode("a%20b"),  "a b");

// html entities
eq("html entity",   str.htmlEntityDecode("&lt;p&gt;&amp;"), "<p>&");

// padding
eq("pad default",   str.pad("ab", 5),               "ab   ");
eq("pad char",      str.pad("ab", 5, "."),          "ab...");
eq("pad left",      str.pad("ab", 5, ".", "left"),  "...ab");
eq("pad both",      str.pad("ab", 6, ".", "both"),  "..ab..");
eq("lpad",          str.lpad("ab", 5, "."),         "...ab");
eq("rpad",          str.rpad("ab", 5, "."),         "ab...");
eq("pad too short", str.pad("abc", 2),              "abc"); // no truncation

// sprintf
eq("sprintf basic", str.sprintf("%s=%d", "x", 42), "x=42");
eq("sprintf hex",   str.sprintf("%08x", 255),      "000000ff");
eq("sprintf float", str.sprintf("%.2f", 1.5),      "1.50");

// normalizeNewlines
eq("nl lf",   str.normalizeNewlines("a\r\nb\rc\nd", "lf"),   "a\nb\nc\nd");
eq("nl crlf", str.normalizeNewlines("a\nb\rc",       "crlf"), "a\r\nb\r\nc");
eq("nl cr",   str.normalizeNewlines("a\r\nb\nc",     "cr"),   "a\rb\rc");
`
	if _, err := eng.Run(context.Background(), "str_test.ts", script); err != nil {
		t.Fatalf("str test script: %v", err)
	}
}

// TestPathNamespace covers dirname/basename including the suffix-stripping
// form of basename.
func TestPathNamespace(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterNamespaceFactory("pth", func(vm *goja.Runtime, _ *eventloop.EventLoop) map[string]any {
		return pathNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	script := `
function eq(label, got, want) {
  if (got !== want) throw new Error(label + ": got " + JSON.stringify(got) + ", want " + JSON.stringify(want));
}
eq("dirname abs",     pth.dirname("/a/b/c.txt"),         "/a/b");
eq("dirname rel",     pth.dirname("a/b/c.txt"),          "a/b");
eq("dirname leaf",    pth.dirname("c.txt"),              ".");
eq("basename",        pth.basename("/a/b/c.txt"),        "c.txt");
eq("basename suffix", pth.basename("/a/b/c.txt", ".txt"), "c");
eq("basename equal",  pth.basename("/a/b/.txt", ".txt"), ".txt"); // do not strip when name == suffix
`
	if _, err := eng.Run(context.Background(), "path_test.ts", script); err != nil {
		t.Fatalf("path test script: %v", err)
	}
}

// TestStrftime covers the supported token subset.
func TestStrftime(t *testing.T) {
	// 2024-01-15 10:30:45 UTC == 1705314645000 ms
	loc, err := time.LoadLocation("UTC")
	if err != nil {
		t.Fatal(err)
	}
	ts := time.UnixMilli(1705314645000).In(loc)

	cases := []struct {
		layout, want string
	}{
		{"%Y-%m-%d", "2024-01-15"},
		{"%H:%M:%S", "10:30:45"},
		{"%F %T", "2024-01-15 10:30:45"},
		{"%A", "Monday"},
		{"%a", "Mon"},
		{"%B", "January"},
		{"%b", "Jan"},
		{"%j", "015"},
		{"%y", "24"},
		{"100%% literal", "100% literal"},
		{"%Q", "%Q"}, // unknown token preserved
		{"%Z", "UTC"},
		{"%z", "+0000"},
	}
	for _, c := range cases {
		got := strftime(ts, c.layout)
		if got != c.want {
			t.Errorf("strftime(%q) = %q, want %q", c.layout, got, c.want)
		}
	}
}
