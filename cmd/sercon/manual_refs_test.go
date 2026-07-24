package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestManualSectionRefs guards against stale §17 cross-references in MANUAL.md.
//
// The §17 "Binding reference" chapter is GENERATED (`make reference`), and its
// hierarchical section numbers shift whenever a namespace is added or reordered
// (adding `cloud`/`mcp` bumped codec/net/services/etc. by a chapter). The
// hand-written "Signatures: §17.x (`ns.member`)" links scattered through the
// prose guides do NOT move with them, so they silently rot — which is exactly
// how ~two dozen of them drifted before this guard existed.
//
// This test builds the authoritative member→number map from the generated §17
// headings and asserts every namespaced §17 reference in the prose (before the
// generated chapter) resolves to the current number, failing with the exact
// corrections needed. Regenerate/fix the prose numbers when it trips.
func TestManualSectionRefs(t *testing.T) {
	path := findManual(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read MANUAL.md: %v", err)
	}
	lines := strings.Split(string(data), "\n")

	gen := -1
	for i, l := range lines {
		if strings.HasPrefix(l, "## 17. Binding reference") {
			gen = i
			break
		}
	}
	if gen < 0 {
		t.Fatal("could not locate the generated '## 17. Binding reference' section in MANUAL.md")
	}

	// Authoritative map from the generated headings: "#### 17.3.9.1 codec.toml.parse".
	heading := regexp.MustCompile(`^#{3,5} (17(?:\.\d+)+) (\S+)$`)
	member := map[string]string{}  // ns.member -> "17.a.b.c"
	chapter := map[string]string{} // ns -> "17.a"
	for _, l := range lines[gen:] {
		if m := heading.FindStringSubmatch(l); m != nil {
			num, name := m[1], m[2]
			if strings.Contains(name, ".") {
				member[name] = num
			} else {
				chapter[name] = num
			}
		}
	}
	if len(member) == 0 {
		t.Fatal("parsed no §17 member headings — heading format may have changed")
	}

	// Namespaced prose refs: §17.x (`ns.member`) — the checkable, recurring class.
	ref := regexp.MustCompile("§(17(?:\\.\\d+)+)\\s*\\(`([a-z][a-zA-Z0-9]*(?:\\.[a-zA-Z0-9*]+)+)`\\)")
	prose := strings.Join(lines[:gen], "\n")

	var bad []string
	for _, m := range ref.FindAllStringSubmatch(prose, -1) {
		cur, name := m[1], m[2]
		want, ok := member[name]
		if !ok && strings.HasSuffix(name, ".*") { // e.g. `services.git.*` → the chapter
			want, ok = chapter[strings.SplitN(name, ".", 2)[0]]
		}
		if !ok {
			continue // references something outside the generated reference; not our concern
		}
		if cur != want {
			bad = append(bad, "§"+cur+" (`"+name+"`) should be §"+want)
		}
	}
	if len(bad) > 0 {
		t.Fatalf("stale §17 cross-references in MANUAL.md (%d) — update the prose numbers to match the generated §17:\n  %s",
			len(bad), strings.Join(bad, "\n  "))
	}
}

// findManual walks up from the test's working directory to the repo root to
// locate MANUAL.md (tests run in cmd/sercon; the manual is two levels up).
func findManual(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		p := filepath.Join(dir, "MANUAL.md")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("MANUAL.md not found walking up from CWD; skipping cross-ref guard")
		}
		dir = parent
	}
}
