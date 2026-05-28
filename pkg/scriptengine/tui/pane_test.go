package tui_test

import (
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

func TestPaneBuffer_LinesAccumulate(t *testing.T) {
	b := tui.NewPaneBuffer(100)
	b.Write([]byte("hello\nworld\n"))
	got := b.Snapshot()
	if got != "hello\nworld\n" {
		t.Errorf("got %q", got)
	}
}

func TestPaneBuffer_ChunkedAcrossNewlines(t *testing.T) {
	b := tui.NewPaneBuffer(100)
	b.Write([]byte("hel"))
	b.Write([]byte("lo\nwo"))
	b.Write([]byte("rld\n"))
	if got := b.Snapshot(); got != "hello\nworld\n" {
		t.Errorf("got %q", got)
	}
}

func TestPaneBuffer_CarriageReturnOverwrites(t *testing.T) {
	b := tui.NewPaneBuffer(100)
	b.Write([]byte("progress: 10%\rprogress: 50%\rprogress: 100%\n"))
	if got := b.Snapshot(); got != "progress: 100%\n" {
		t.Errorf("got %q", got)
	}
}

func TestPaneBuffer_CarriageReturnThenShorterLine(t *testing.T) {
	// "\rdone" must replace the *whole* current line, not partially
	// overwrite "longer line"'s tail.
	b := tui.NewPaneBuffer(100)
	b.Write([]byte("a much longer line\rdone\n"))
	if got := b.Snapshot(); got != "done\n" {
		t.Errorf("got %q", got)
	}
}

func TestPaneBuffer_CapEvictsOldest(t *testing.T) {
	b := tui.NewPaneBuffer(3) // cap of 3 lines
	for i := 0; i < 5; i++ {
		b.Write([]byte("line"))
		b.Write([]byte{'0' + byte(i)})
		b.Write([]byte{'\n'})
	}
	got := b.Snapshot()
	want := "line2\nline3\nline4\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPaneBuffer_UTF8AcrossChunkBoundary(t *testing.T) {
	// A 3-byte UTF-8 sequence split across two Write calls must still
	// render correctly. "好" is U+597D, encoded as E5 A5 BD.
	b := tui.NewPaneBuffer(100)
	b.Write([]byte{0xE5, 0xA5})
	b.Write([]byte{0xBD, '\n'})
	if got := b.Snapshot(); !strings.Contains(got, "好") {
		t.Errorf("got %q", got)
	}
}

func TestPaneBuffer_PartialFinalLine(t *testing.T) {
	// Final line without a trailing \n is still in the snapshot.
	b := tui.NewPaneBuffer(100)
	b.Write([]byte("a\nb"))
	if got := b.Snapshot(); got != "a\nb" {
		t.Errorf("got %q", got)
	}
}

func TestPaneBuffer_Clear(t *testing.T) {
	b := tui.NewPaneBuffer(100)
	b.Write([]byte("a\nb\n"))
	b.Clear()
	if got := b.Snapshot(); got != "" {
		t.Errorf("got %q", got)
	}
}
