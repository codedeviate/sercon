package main

import (
	"testing"

	"github.com/dop251/goja"
)

// alwaysLook / neverLook are LookPath stubs for the platform table.
func alwaysLook(string) bool { return true }
func neverLook(string) bool  { return false }

func TestOpenerArgv_Platforms(t *testing.T) {
	cases := []struct {
		goos string
		want string
	}{
		{"darwin", "open"},
		{"linux", "xdg-open"},
		{"windows", "rundll32"},
	}
	for _, c := range cases {
		argv, ok := openerArgv(c.goos, alwaysLook)
		if !ok || len(argv) == 0 || argv[0] != c.want {
			t.Fatalf("%s: got %v ok=%v, want prefix[0]=%q", c.goos, argv, ok, c.want)
		}
	}
}

func TestOpenerArgv_LinuxFallback(t *testing.T) {
	// xdg-open absent → gnome-open.
	look := func(name string) bool { return name == "gnome-open" }
	argv, ok := openerArgv("linux", look)
	if !ok || argv[0] != "gnome-open" {
		t.Fatalf("got %v ok=%v, want gnome-open", argv, ok)
	}
}

func TestOpenerArgv_NoneAvailable(t *testing.T) {
	if _, ok := openerArgv("linux", neverLook); ok {
		t.Fatal("no opener on PATH should yield ok=false")
	}
}

// openExtract rejects empty/missing targets (no shell interpolation surface).
func TestOpenExtract_RejectsEmpty(t *testing.T) {
	vm := goja.New()
	mk := func(vals ...goja.Value) goja.FunctionCall { return goja.FunctionCall{Arguments: vals} }
	if _, err := openExtract(mk()); err == nil {
		t.Fatal("missing target should error")
	}
	if _, err := openExtract(mk(vm.ToValue(""))); err == nil {
		t.Fatal("empty target should error")
	}
	got, err := openExtract(mk(vm.ToValue("https://example.com")))
	if err != nil || got != "https://example.com" {
		t.Fatalf("valid target: got %q err=%v", got, err)
	}
}
