package main

import "testing"

func TestClipboardBackend(t *testing.T) {
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	only := func(names ...string) func(string) bool {
		set := map[string]bool{}
		for _, n := range names {
			set[n] = true
		}
		return func(s string) bool { return set[s] }
	}

	cases := []struct {
		name      string
		goos      string
		wayland   bool
		look      func(string) bool
		wantOK    bool
		wantRead0 string // expected readArgv[0] when ok
		wantWrite string // expected writeArgv[0] when ok
	}{
		{"darwin", "darwin", false, all, true, "pbpaste", "pbcopy"},
		{"darwin missing", "darwin", false, none, false, "", ""},
		{"linux wayland", "linux", true, only("wl-paste", "wl-copy"), true, "wl-paste", "wl-copy"},
		{"linux wayland flag but no tool falls back", "linux", true, only("xclip"), true, "xclip", "xclip"},
		{"linux xclip", "linux", false, only("xclip"), true, "xclip", "xclip"},
		{"linux xsel", "linux", false, only("xsel"), true, "xsel", "xsel"},
		{"linux none", "linux", false, none, false, "", ""},
		{"windows", "windows", false, all, true, "powershell", "clip"},
		{"windows missing", "windows", false, none, false, "", ""},
		{"unsupported", "plan9", false, all, false, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, w, ok, reason := clipboardBackend(tc.goos, tc.wayland, tc.look)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v (reason %q)", ok, tc.wantOK, reason)
			}
			if !ok {
				if reason == "" {
					t.Fatalf("expected a non-empty reason when unavailable")
				}
				return
			}
			if r[0] != tc.wantRead0 {
				t.Fatalf("readArgv[0]=%q want %q", r[0], tc.wantRead0)
			}
			if w[0] != tc.wantWrite {
				t.Fatalf("writeArgv[0]=%q want %q", w[0], tc.wantWrite)
			}
		})
	}
}

func TestClipboardImageBackend(t *testing.T) {
	all := func(string) bool { return true }
	none := func(string) bool { return false }
	only := func(names ...string) func(string) bool {
		set := map[string]bool{}
		for _, n := range names {
			set[n] = true
		}
		return func(s string) bool { return set[s] }
	}
	cases := []struct {
		name     string
		goos     string
		wayland  bool
		look     func(string) bool
		wantOK   bool
		wantKind string
	}{
		{"darwin with pngpaste", "darwin", false, all, true, "darwin"},
		{"darwin no pngpaste", "darwin", false, only("osascript"), false, ""},
		{"linux wayland", "linux", true, only("wl-copy", "wl-paste"), true, "wl"},
		{"linux xclip", "linux", false, only("xclip"), true, "xclip"},
		{"linux xsel only -> no image", "linux", false, only("xsel"), false, ""},
		{"windows", "windows", false, all, true, "windows"},
		{"unsupported", "plan9", false, all, false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			strat, ok, reason := clipboardImageBackend(tc.goos, tc.wayland, tc.look)
			if ok != tc.wantOK {
				t.Fatalf("ok=%v want %v (reason %q)", ok, tc.wantOK, reason)
			}
			if !ok {
				if reason == "" {
					t.Fatal("expected non-empty reason")
				}
				return
			}
			if strat.kind != tc.wantKind {
				t.Fatalf("kind=%q want %q", strat.kind, tc.wantKind)
			}
		})
	}
	_ = none
}

func TestIsPNG(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00}
	if !isPNG(png) {
		t.Fatal("valid PNG magic rejected")
	}
	for _, bad := range [][]byte{nil, []byte("not png"), {0x89, 'P'}, []byte("\xFF\xD8\xFF")} {
		if isPNG(bad) {
			t.Fatalf("non-PNG accepted: %v", bad)
		}
	}
}

func TestTrimWindowsClipboardNewline(t *testing.T) {
	cases := map[string]string{
		"hello\r\n":  "hello",
		"hello\n":    "hello",
		"hello":      "hello",
		"a\r\nb\r\n": "a\r\nb", // only one trailing newline stripped
	}
	for in, want := range cases {
		if got := trimWindowsClipboardNewline(in); got != want {
			t.Fatalf("trim(%q)=%q want %q", in, got, want)
		}
	}
}
