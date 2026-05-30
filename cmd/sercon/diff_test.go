package main

import "testing"

// countAddedRemoved must ignore the `--- a` / `+++ b` file headers and
// only count body lines. Mirrors what `git diff --shortstat` reports.
func TestDiff_CountAddedRemoved(t *testing.T) {
	diff := `--- a
+++ b
@@ -1,3 +1,3 @@
 same
-old1
-old2
+new1
+new2
+extra
 same
`
	added, removed := countAddedRemoved(diff)
	if added != 3 {
		t.Errorf("added: got %d, want 3", added)
	}
	if removed != 2 {
		t.Errorf("removed: got %d, want 2", removed)
	}
}

// looksBinary fires on a NUL byte anywhere in the first 8 KB. Text inputs
// without NULs must be classified as text even when they contain non-ASCII
// bytes (UTF-8, etc.) — chardet handles charset detection, this only
// gates whether we bother running difflib.
func TestDiff_LooksBinary(t *testing.T) {
	if !looksBinary([]byte{0x89, 'P', 'N', 'G', 0x00, 'c'}) {
		t.Error("PNG header should be classified binary")
	}
	if !looksBinary([]byte("hello\x00world")) {
		t.Error("embedded NUL should be classified binary")
	}
	if looksBinary([]byte("café crème — UTF-8 prose")) {
		t.Error("UTF-8 prose without NUL should not be binary")
	}
	if looksBinary([]byte("")) {
		t.Error("empty input should not be binary")
	}
	// NUL past 8 KB doesn't count (the look is bounded to keep the call cheap).
	big := make([]byte, 9000)
	for i := range big {
		big[i] = 'a'
	}
	big[8500] = 0
	if looksBinary(big) {
		t.Error("NUL past the 8KB sample window should not flag binary")
	}
}
