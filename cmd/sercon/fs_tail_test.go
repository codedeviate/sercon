package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// collector accumulates lines delivered by followFile, safe for concurrent use.
type collector struct {
	mu    sync.Mutex
	lines []string
}

func (c *collector) add(s string) error {
	c.mu.Lock()
	c.lines = append(c.lines, s)
	c.mu.Unlock()
	return nil
}

func (c *collector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

// waitForLines polls until the collector holds at least n lines or the deadline
// passes (condition-based, not a fixed sleep).
func waitForLines(t *testing.T, c *collector, n int) []string {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got := c.snapshot(); len(got) >= n {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	return c.snapshot()
}

// appendLine appends one \n-terminated line to path (genuine append; O_APPEND).
func appendLine(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(s + "\n"); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
}

func TestFollowFile_FromEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("old-1\nold-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c collector
	go func() { _ = followFile(ctx, path, tailFrom{mode: "end"}, c.add) }()

	// Give the follower a moment to seek to EOF, then append.
	time.Sleep(100 * time.Millisecond)
	appendLine(t, path, "new-1")
	appendLine(t, path, "new-2")

	got := waitForLines(t, &c, 2)
	if len(got) != 2 || got[0] != "new-1" || got[1] != "new-2" {
		t.Fatalf("from:end delivered %v, want [new-1 new-2] (no old lines)", got)
	}
}

func TestFollowFile_FromStart(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("a\nb\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c collector
	go func() { _ = followFile(ctx, path, tailFrom{mode: "start"}, c.add) }()
	appendLine(t, path, "c")
	got := waitForLines(t, &c, 3)
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Fatalf("from:start delivered %v, want [a b c]", got)
	}
}

func TestFollowFile_FromLastNLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("l1\nl2\nl3\nl4\nl5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c collector
	go func() { _ = followFile(ctx, path, tailFrom{mode: "lines", n: 2}, c.add) }()
	got := waitForLines(t, &c, 2)
	if len(got) != 2 || got[0] != "l4" || got[1] != "l5" {
		t.Fatalf("from:2 delivered %v, want [l4 l5]", got)
	}
}

func TestFollowFile_PartialLineBuffered(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c collector
	go func() { _ = followFile(ctx, path, tailFrom{mode: "end"}, c.add) }()
	time.Sleep(100 * time.Millisecond)
	// Write a line in two fragments; only the completed line should surface.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("half ")
	_ = f.Sync()
	time.Sleep(50 * time.Millisecond)
	_, _ = f.WriteString("whole\n")
	_ = f.Close()
	got := waitForLines(t, &c, 1)
	if len(got) != 1 || got[0] != "half whole" {
		t.Fatalf("partial buffering delivered %v, want [half whole]", got)
	}
}

func TestFollowFile_Truncation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c collector
	go func() { _ = followFile(ctx, path, tailFrom{mode: "end"}, c.add) }()
	time.Sleep(100 * time.Millisecond)
	appendLine(t, path, "before")
	waitForLines(t, &c, 1)
	// Truncate in place (copytruncate) and write fresh content.
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	appendLine(t, path, "after")
	got := waitForLines(t, &c, 2)
	if len(got) != 2 || got[1] != "after" {
		t.Fatalf("after truncation delivered %v, want last line 'after'", got)
	}
}

func TestFollowFile_RotationRenameRecreate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var c collector
	go func() { _ = followFile(ctx, path, tailFrom{mode: "end"}, c.add) }()
	time.Sleep(100 * time.Millisecond)
	appendLine(t, path, "pre-rotate")
	waitForLines(t, &c, 1)
	// Rotate: rename current out of the way, create a fresh file at the path.
	if err := os.Rename(path, path+".1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	appendLine(t, path, "post-rotate")
	got := waitForLines(t, &c, 2)
	if len(got) < 2 || got[len(got)-1] != "post-rotate" {
		t.Fatalf("after rotation delivered %v, want last line 'post-rotate'", got)
	}
}

func TestFollowFile_ContextCancelStops(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- followFile(ctx, path, tailFrom{mode: "end"}, func(string) error { return nil }) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
		// returned promptly — good
	case <-time.After(2 * time.Second):
		t.Fatal("followFile did not return after ctx cancel")
	}
}

func TestFollowFile_OnLineErrorStops(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "log")
	if err := os.WriteFile(path, []byte("boom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	onLine := func(string) error { return fmt.Errorf("stop") }
	go func() { done <- followFile(ctx, path, tailFrom{mode: "start"}, onLine) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected followFile to return the onLine error, got nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("followFile did not return after onLine returned an error")
	}
}

func TestFollowFile_MissingFileErrors(t *testing.T) {
	err := followFile(context.Background(), filepath.Join(t.TempDir(), "nope"), tailFrom{mode: "end"}, func(string) error { return nil })
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
}

func TestParseTailFrom(t *testing.T) {
	cases := []struct {
		in   any
		mode string
		n    int
		err  bool
	}{
		{nil, "end", 0, false},
		{"end", "end", 0, false},
		{"start", "start", 0, false},
		{int64(5), "lines", 5, false},
		{float64(3), "lines", 3, false},
		{"bogus", "", 0, true},
		{int64(-1), "", 0, true},
	}
	for _, c := range cases {
		opts := map[string]any{}
		if c.in != nil {
			opts["from"] = c.in
		}
		got, err := parseTailFrom(opts)
		if c.err {
			if err == nil {
				t.Fatalf("from=%v: expected error", c.in)
			}
			continue
		}
		if err != nil || got.mode != c.mode || got.n != c.n {
			t.Fatalf("from=%v: got %+v err=%v, want mode=%q n=%d", c.in, got, err, c.mode, c.n)
		}
	}
}
