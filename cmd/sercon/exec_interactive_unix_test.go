//go:build !windows

package main

import (
	"bytes"
	"os"
	"sync"
	"testing"
	"time"
)

// syncBuf is a mutex-guarded io.Writer so the copy goroutine and the test can
// touch it without a race.
type syncBuf struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// waitFor polls until cond() is true or the deadline passes.
func waitFor(cond func() bool, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// interruptibleCopy must forward bytes from the source fd to dst, and its
// returned stop() must interrupt a parked read and block until the copy
// goroutine has exited.
func TestInterruptibleCopy_ForwardsAndStops(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()

	var dst syncBuf
	stop, err := interruptibleCopy(int(pr.Fd()), &dst)
	if err != nil {
		t.Fatalf("interruptibleCopy: %v", err)
	}

	if _, err := pw.WriteString("hello"); err != nil {
		t.Fatal(err)
	}
	if !waitFor(func() bool { return dst.String() == "hello" }, 2*time.Second) {
		t.Fatalf("dst = %q, want %q", dst.String(), "hello")
	}

	// stop() must return promptly even though the reader is parked (nothing more
	// to read) — i.e. the parked read was interrupted, not left blocked.
	done := make(chan struct{})
	go func() { stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stop() did not return — reader was not interrupted")
	}

	// Idempotent: a second stop() is safe and also returns promptly.
	stop()
}

// The single-owner invariant: after stop(), a fresh interruptibleCopy on the
// same source receives a byte written AFTER the switch — the leftover reader
// from the first session must not steal it (the first-keystroke-dropped bug).
func TestInterruptibleCopy_NoLeftoverReaderStealsNextByte(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()

	// Session 1: start and immediately stop, with no byte in flight — mirrors an
	// interactive child that exited while its stdin reader sat parked.
	var dst1 syncBuf
	stop1, err := interruptibleCopy(int(pr.Fd()), &dst1)
	if err != nil {
		t.Fatal(err)
	}
	stop1() // must fully join session 1's reader before session 2 starts

	// Session 2: a fresh reader, then the user's first keystroke.
	var dst2 syncBuf
	stop2, err := interruptibleCopy(int(pr.Fd()), &dst2)
	if err != nil {
		t.Fatal(err)
	}
	defer stop2()

	if _, err := pw.WriteString("l"); err != nil {
		t.Fatal(err)
	}
	if !waitFor(func() bool { return dst2.String() == "l" }, 2*time.Second) {
		t.Fatalf("first byte after session switch was lost: dst2 = %q, want %q", dst2.String(), "l")
	}
	if dst1.String() != "" {
		t.Fatalf("stopped session 1 reader stole a byte: dst1 = %q", dst1.String())
	}
}
