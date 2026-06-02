package tui

import (
	"io"
	"strings"
)

// FallbackPane is the writer used when the TUI doesn't run (stdout is
// not a TTY). Every complete line is emitted to its underlying writer
// prefixed with "[paneName] "; pending unterminated text is buffered
// until a newline arrives or Flush() is called.
//
// ANSI escape sequences in the input are stripped — they'd be noise in
// a piped/redirected stream. Bare \r resets the pending line, matching
// the in-TUI buffer behaviour so spinners overwrite cleanly.
type FallbackPane struct {
	w       io.Writer
	prefix  string
	pending strings.Builder
}

// NewFallbackPane returns a writer that emits prefixed plain-text lines
// for the named pane to w.
func NewFallbackPane(w io.Writer, name string) *FallbackPane {
	return &FallbackPane{w: w, prefix: "[" + name + "] "}
}

// Write feeds p through the fallback. It always returns (len(p), nil)
// even when the underlying writer errors (the writer's error is swallowed
// because the call site is the synchronous script-side binding and the
// alternative is a JS exception on every log line during a broken pipe).
func (f *FallbackPane) Write(p []byte) (int, error) {
	clean := StripANSI(string(p))
	for i := 0; i < len(clean); i++ {
		switch clean[i] {
		case '\n':
			f.emit()
		case '\r':
			f.pending.Reset()
		default:
			f.pending.WriteByte(clean[i])
		}
	}
	return len(p), nil
}

func (f *FallbackPane) emit() {
	_, _ = io.WriteString(f.w, f.prefix)
	_, _ = io.WriteString(f.w, f.pending.String())
	_, _ = f.w.Write([]byte{'\n'})
	f.pending.Reset()
}

// Flush emits any pending unterminated line (with a synthetic trailing
// \n). Called at TUI teardown so the last partial line doesn't get lost.
func (f *FallbackPane) Flush() {
	if f.pending.Len() == 0 {
		return
	}
	f.emit()
}
