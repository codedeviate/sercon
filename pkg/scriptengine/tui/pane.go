package tui

import "strings"

// PaneBuffer is a capped scrollback log: a ring of completed lines plus a
// single pending line being built up by streaming Writes. \r without a
// following \n is treated as "carriage return only" — it resets the
// pending line, matching how terminals display progress spinners
// (`Updating: 23%\r` → next line OVERWRITES the previous, not appends).
//
// PaneBuffer is not safe for concurrent use. The TUI Controller owns one
// per pane and serialises writes onto the application goroutine.
type PaneBuffer struct {
	cap     int      // maximum number of completed lines retained
	lines   []string // completed lines (no trailing \n); ring of size <= cap
	start   int      // index of oldest line in `lines` (only used when len(lines)==cap)
	pending strings.Builder
}

// NewPaneBuffer constructs a buffer with the given line cap. A non-
// positive cap is clamped to 1.
func NewPaneBuffer(cap int) *PaneBuffer {
	if cap < 1 {
		cap = 1
	}
	return &PaneBuffer{cap: cap, lines: make([]string, 0, cap)}
}

// Write appends p to the buffer, splitting on \n and applying \r
// overwrite. It always returns (len(p), nil) — the buffer never errors
// or short-writes.
func (b *PaneBuffer) Write(p []byte) (int, error) {
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch c {
		case '\n':
			b.commit()
		case '\r':
			// Bare \r resets the pending line. A following \n
			// commits the (now-empty) pending line normally.
			b.pending.Reset()
		default:
			b.pending.WriteByte(c)
		}
	}
	return len(p), nil
}

// commit moves the pending line into the ring as a completed line and
// evicts the oldest if at capacity.
func (b *PaneBuffer) commit() {
	line := b.pending.String()
	b.pending.Reset()
	if len(b.lines) < b.cap {
		b.lines = append(b.lines, line)
		return
	}
	// At capacity: overwrite the slot at start, then advance start.
	b.lines[b.start] = line
	b.start = (b.start + 1) % b.cap
}

// Snapshot returns the buffer's current text content: every committed
// line in order (oldest first), each followed by '\n', then the pending
// line (if any) without a trailing newline.
func (b *PaneBuffer) Snapshot() string {
	var out strings.Builder
	if len(b.lines) < b.cap {
		for _, l := range b.lines {
			out.WriteString(l)
			out.WriteByte('\n')
		}
	} else {
		for i := 0; i < b.cap; i++ {
			out.WriteString(b.lines[(b.start+i)%b.cap])
			out.WriteByte('\n')
		}
	}
	out.WriteString(b.pending.String())
	return out.String()
}

// Clear discards every completed line and the pending line.
func (b *PaneBuffer) Clear() {
	b.lines = b.lines[:0]
	b.start = 0
	b.pending.Reset()
}
