package tui_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

func TestFallback_PrefixesEachLine(t *testing.T) {
	var buf bytes.Buffer
	w := tui.NewFallbackPane(&buf, "brew")
	w.Write([]byte("downloading...\ninstalling...\n"))
	got := buf.String()
	want := "[brew] downloading...\n[brew] installing...\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestFallback_BuffersPartialLine(t *testing.T) {
	var buf bytes.Buffer
	w := tui.NewFallbackPane(&buf, "log")
	w.Write([]byte("hello "))
	w.Write([]byte("world\n"))
	got := buf.String()
	if got != "[log] hello world\n" {
		t.Errorf("got %q", got)
	}
}

func TestFallback_FlushUnterminatedLine(t *testing.T) {
	// Flush() forces the pending unterminated line to be emitted (with
	// a synthetic trailing \n). Called at TUI teardown so no output is
	// lost.
	var buf bytes.Buffer
	w := tui.NewFallbackPane(&buf, "x")
	w.Write([]byte("partial"))
	w.Flush()
	got := buf.String()
	if !strings.HasSuffix(got, "[x] partial\n") {
		t.Errorf("got %q", got)
	}
}

func TestFallback_StripsANSI(t *testing.T) {
	// Subprocess output in fallback mode shouldn't dump ANSI escape
	// bytes into the user's pipe. We strip them.
	var buf bytes.Buffer
	w := tui.NewFallbackPane(&buf, "brew")
	w.Write([]byte("\x1b[31mERR\x1b[0m boom\n"))
	got := buf.String()
	if got != "[brew] ERR boom\n" {
		t.Errorf("got %q", got)
	}
}

func TestFallback_OverwriteOnCarriageReturn(t *testing.T) {
	// Pending line is reset by bare \r, matching PaneBuffer's behaviour.
	var buf bytes.Buffer
	w := tui.NewFallbackPane(&buf, "x")
	w.Write([]byte("10%\r50%\r100%\n"))
	got := buf.String()
	if got != "[x] 100%\n" {
		t.Errorf("got %q", got)
	}
}
