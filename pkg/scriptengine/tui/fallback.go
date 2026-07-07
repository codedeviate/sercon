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
// a piped/redirected stream. A *bare* \r resets the pending line, matching
// the in-TUI buffer behaviour so spinners overwrite cleanly; the \r of a
// CRLF pair is treated as a plain line ending so CRLF-terminated streams
// (e.g. exec.shell with pty:true, whose ONLCR maps every \n to \r\n) keep
// their content instead of being wiped.
type FallbackPane struct {
	w       io.Writer
	prefix  string
	pending strings.Builder
	// pendingCR records a trailing '\r' whose successor byte has not been
	// seen yet (it may be the '\n' of a CRLF, arriving in this chunk or the
	// next). Resolved on the next byte: '\n' -> line ending, anything else
	// -> bare-CR overwrite.
	pendingCR bool
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
		c := clean[i]
		if f.pendingCR {
			f.pendingCR = false
			if c == '\n' {
				// CRLF: the \r was a line ending, not an overwrite.
				f.emit()
				continue
			}
			// Bare \r followed by more text: overwrite the pending line.
			f.pending.Reset()
		}
		switch c {
		case '\n':
			f.emit()
		case '\r':
			// Defer the decision until we see the next byte (possibly in
			// the next Write): \n makes it a CRLF line ending, otherwise a
			// bare-CR overwrite.
			f.pendingCR = true
		default:
			f.pending.WriteByte(c)
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
