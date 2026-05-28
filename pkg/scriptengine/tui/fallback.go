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
	i := 0
	for i < len(p) {
		c := p[i]
		if c == 0x1b {
			// Skip an ANSI escape sequence.
			i = skipEscapeAt(p, i)
			continue
		}
		switch c {
		case '\n':
			f.emit()
		case '\r':
			f.pending.Reset()
		default:
			f.pending.WriteByte(c)
		}
		i++
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

// skipEscapeAt advances past an escape sequence starting at i. Returns
// the index of the first byte after the sequence. The fallback writer
// doesn't need to be as precise as the ANSI translator — anything that
// looks like ESC <byte>... is dropped. CSI sequences end at a byte in
// 0x40-0x7E; OSC sequences end at BEL or ST. Anything malformed: skip
// the ESC and continue.
func skipEscapeAt(p []byte, i int) int {
	if i+1 >= len(p) {
		return i + 1
	}
	next := p[i+1]
	switch next {
	case '[':
		// CSI: scan for final byte in 0x40-0x7E.
		j := i + 2
		for j < len(p) {
			c := p[j]
			if c >= 0x40 && c <= 0x7E {
				return j + 1
			}
			j++
		}
		return j
	case ']':
		// OSC: scan for BEL or ESC \.
		j := i + 2
		for j < len(p) {
			if p[j] == 0x07 {
				return j + 1
			}
			if p[j] == 0x1b && j+1 < len(p) && p[j+1] == '\\' {
				return j + 2
			}
			j++
		}
		return j
	default:
		return i + 2
	}
}
