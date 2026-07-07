package main

import (
	"strings"
	"testing"
)

// A script-controlled id or event value containing newlines must not be
// able to inject additional SSE fields/frames. Per the SSE spec these
// fields cannot contain newlines, so they must be stripped (data is already
// split into one `data:` line per line, so data cannot inject).
func TestFormatSSEEvent_IDAndEventCannotInject(t *testing.T) {
	out := string(formatSSEEvent(sseEvent{
		data:  "ok",
		id:    "1\ndata: injected\nevent: admin",
		event: "msg\ndata: sneaky",
	}))
	lines := strings.Split(out, "\n")
	countPrefix := func(p string) int {
		n := 0
		for _, l := range lines {
			if strings.HasPrefix(l, p) {
				n++
			}
		}
		return n
	}
	// Exactly one id: and one event: line — not three fields each.
	if got := countPrefix("id:"); got != 1 {
		t.Errorf("expected 1 id: line, got %d\n%s", got, out)
	}
	if got := countPrefix("event:"); got != 1 {
		t.Errorf("expected 1 event: line, got %d\n%s", got, out)
	}
	// The injected payloads must not appear as their own fields/lines.
	for _, bad := range []string{"data: injected", "event: admin", "data: sneaky"} {
		for _, l := range lines {
			if l == bad {
				t.Errorf("injected line %q leaked into the stream:\n%s", bad, out)
			}
		}
	}
	// The legitimate data line survives.
	if !strings.Contains(out, "data: ok\n") {
		t.Errorf("legitimate data line missing:\n%s", out)
	}
}
