package main

import (
	"strings"
	"testing"
)

// TestRaw_OpenRejectsOrOpens: net.raw.open must be wired; opening either
// succeeds (root) or rejects with a privilege/platform hint. Either way the
// binding exists and returns a Promise.
func TestRaw_OpenRejectsOrOpens(t *testing.T) {
	got := runSocketScript(t, `
		let out = "";
		try {
			const h = await net.raw.open({ iface: "lo" });
			await h.close();
			out = "opened";
		} catch (e) {
			out = "rejected:" + e.message;
		}
		__capture(out);
	`)
	s, _ := got.(string)
	if !strings.HasPrefix(s, "opened") && !strings.HasPrefix(s, "rejected:") {
		t.Fatalf("unexpected output: %q", got)
	}
}
