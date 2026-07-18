package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func streamEng(t *testing.T) *scriptengine.Engine {
	t.Helper()
	root := searchFixture(t)
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: root, DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	old, _ := os.Getwd()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return eng
}

func TestFsFind_Stream(t *testing.T) {
	eng := streamEng(t)
	_, err := eng.Run(context.Background(), "s.ts", `
		const got = [];
		for await (const p of fs.find({ glob: "**/*.go", stream: true })) got.push(p);
		if (got.slice().sort().join(",") !== "b.go,sub/c.go") throw new Error(got.join(","));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

func TestFsGrep_Stream(t *testing.T) {
	eng := streamEng(t)
	_, err := eng.Run(context.Background(), "s.ts", `
		const got = [];
		for await (const m of fs.grep({ pattern: "package", glob: "**/*.go", stream: true })) got.push(m.path);
		if (got.slice().sort().join(",") !== "b.go,sub/c.go") throw new Error(got.join(","));
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestFsFind_Stream_BreakEarly verifies that breaking out of a for-await loop
// early (before the producer drains) does not deadlock or leak the producer
// goroutine — the iterator's return() must cancel the walk and release the
// HoldRun sentinel.
//
// NOTE: this fixture only has 2 matching entries against the stream's
// 256-slot buffered channel, so it can never actually force the producer to
// block on a full channel — it is a smoke test only (regressing the
// cancellable `select` in streamFind to a bare `out <- item` would still
// pass this one). See TestFsFind_Stream_BreakEarly_Backpressure below for a
// test that genuinely forces backpressure and would catch that regression.
func TestFsFind_Stream_BreakEarly(t *testing.T) {
	eng := streamEng(t)
	_, err := eng.Run(context.Background(), "s.ts", `
		for await (const p of fs.find({ glob: "**/*.go", stream: true })) {
			break;
		}
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestStreamFind_CancelUnblocksBlockedProducer is the falsifiable version of
// TestFsFind_Stream_BreakEarly. It calls streamFind directly (the same
// function the real fs.find({stream:true}) binding runs on a background
// goroutine) with a deliberately *unbuffered* out channel, so the second
// entry can never be sent until the first is drained — deterministic
// backpressure, independent of fastwalk's actual concurrency/timing (an
// end-to-end version driven through the real 256-slot buffered channel was
// tried first and found unreliable: with a flat, single-directory fixture
// fastwalk uses a single worker, so only one goroutine ever blocks, and that
// signal was too easily masked by ambient goroutine-count noise/margin).
//
// The test drains exactly one item (mirroring "break after reading one"),
// cancels, and asserts streamFind returns promptly. If streamFind's per-item
// send were a bare `out <- item` instead of
//
//	select {
//	case out <- item:
//	case <-ctx.Done():
//	}
//
// the producer would block forever trying to send the second entry (nobody
// drains `out` again), and this test times out waiting on streamFind's
// return channel — a real, reproducible RED or a leaked goroutine, not a
// flaky proxy signal.
//
// Verified RED against a deliberately-reverted bare-send and GREEN against
// the current select-based code, under -race (see task-6-report.md for the
// exact diff and failure output).
func TestStreamFind_CancelUnblocksBlockedProducer(t *testing.T) {
	root := searchFixture(t) // a.txt, b.go, sub/c.go — at least 3 file entries
	a := fsFindArgs{
		walk: walkOptions{
			roots:     []string{root},
			types:     map[string]bool{"file": true},
			gitignore: true,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan any) // unbuffered: forces backpressure on the very next send

	done := make(chan error, 1)
	go func() { done <- streamFind(ctx, a, out) }()

	select {
	case <-out: // drain exactly one item; the producer now blocks sending #2
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive the first item within 2s")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("streamFind returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("streamFind did not return after ctx cancellation — producer likely blocked on a bare channel send")
	}
}

// TestStreamGrep_CancelUnblocksBlockedProducer is the streamGrep analogue of
// TestStreamFind_CancelUnblocksBlockedProducer — same bug shape
// (streamGrep's per-match send inside grepEachFile's callback), same
// deterministic unbuffered-channel forcing.
//
// Both matches live in the SAME file (2 occurrences of "package" in one
// file) rather than one-per-file across the walk. That matters:
// grepEachFile's handle() rechecks ctx.Err() once per FILE before reading —
// if the two matches were in different files, a cancellation landing between
// files would be caught by that per-file guard and the loop's send would
// never even be attempted, masking a regression. Keeping both matches in one
// file forces the second send to be attempted from *within* the same fn
// callback invocation, past that guard, so it genuinely exercises the
// select/bare-send in streamGrep's per-match loop.
func TestStreamGrep_CancelUnblocksBlockedProducer(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("package one\npackage two\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := compileGrepMatcher("package", false, false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	a := grepArgs{
		walk: walkOptions{roots: []string{root}, gitignore: true},
		extra: grepExtra{
			matcher: m,
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := make(chan any) // unbuffered

	done := make(chan error, 1)
	go func() { done <- streamGrep(ctx, a, out) }()

	select {
	case <-out:
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive the first match within 2s")
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("streamGrep returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("streamGrep did not return after ctx cancellation — producer likely blocked on a bare channel send")
	}
}
