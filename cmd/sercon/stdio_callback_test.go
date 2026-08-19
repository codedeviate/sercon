package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// queueLen reports how many complete lines are currently queued, under
// cb.mu. Test-only: several Go-level tests drive tryFeed directly with a
// nil loop (so no drain ever runs) and need to observe the queue; reading
// cb.queue straight from the test is safe only by accident of those tests
// being single-goroutine, and stops being safe the moment a live-loop
// variant is added.
func (c *lineCallback) queueLen() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.queue)
}

// queuedLine returns the i'th queued line (0 == oldest), under the same
// lock as queueLen. Test-only; see queueLen.
func (c *lineCallback) queuedLine(i int) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.queue[i]
}

// captureRealStderr redirects the process's real os.Stderr — not the
// redirectable stdioErrStream — to a pipe for the rest of the test.
// reportThrow (stdio_callback.go) writes there directly and deliberately
// bypasses the stream, so verifying it means capturing the OS-level file
// descriptor rather than swapping the package registry the way
// withCapturedStdio does. Call the returned finish() once, after the code
// under test has run, to restore os.Stderr and get everything written to it.
func captureRealStderr(t *testing.T) (finish func() string) {
	t.Helper()
	old := os.Stderr
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stderr = pw
	return func() string {
		os.Stderr = old
		_ = pw.Close()
		data, err := io.ReadAll(pr)
		_ = pr.Close()
		if err != nil {
			t.Fatalf("read captured stderr: %v", err)
		}
		return string(data)
	}
}

// A function target receives complete lines, in order, without the newline.
func TestCallback_ReceivesLinesInOrder(t *testing.T) {
	_, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		const seen = [];
		runtime.stdout.to(line => { seen.push(line); });
		console.log("one");
		console.log("two");
		console.log("three");
		// Delivery is scheduled on the loop, so yield once before reading.
		await runtime.time.sleep(10);
		export default seen.join("|");
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "one|two|three"; got != want {
		t.Fatalf("got %v want %v", got, want)
	}
}

// A line arrives exactly once, without its newline. Driven at the Go level so
// the test can hand the callback a partial write, which console.log never does.
func TestCallback_BuffersPartialLine(t *testing.T) {
	var base strings.Builder
	s := newStream("stdout", &base)

	cb := &lineCallback{queueCap: lineQueueCap}
	s.push(destination{kind: destCallback, cb: cb})

	// Two partial writes that together make one line: nothing may be queued
	// until the newline arrives, and nothing may fall through to the base.
	_, _ = s.Write([]byte("half-"))
	if got := cb.queueLen(); got != 0 {
		t.Fatalf("queued %d lines before the newline, want 0", got)
	}
	_, _ = s.Write([]byte("a-line\n"))
	if got, want := cb.queueLen(), 1; got != want {
		t.Fatalf("queued %d lines, want %d", got, want)
	}
	if got, want := cb.queuedLine(0), "half-a-line"; got != want {
		t.Fatalf("queued %q want %q", got, want)
	}
	if base.Len() != 0 {
		t.Fatalf("nothing should have fallen through, got %q", base.String())
	}
	s.reset()
}

// console.log INSIDE the handler must not recurse: it falls through to the
// destination beneath the callback.
func TestCallback_ReentrancyFallsThrough(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdout.to(line => { console.log("echo:" + line); });
		console.log("trigger");
		await runtime.time.sleep(20);
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	// The handler's own console.log bypassed the callback and reached the
	// capture buffer beneath it. Exactly once — no recursion.
	if got, want := out.String(), "echo:trigger\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// A trailing partial line is written to the destination beneath when the
// callback is popped, so nothing is lost when a script exits mid-line.
func TestCallback_PartialFlushedBeneathOnPop(t *testing.T) {
	var base strings.Builder
	s := newStream("stdout", &base)

	cb := &lineCallback{queueCap: lineQueueCap}
	restore := s.push(destination{kind: destCallback, cb: cb})
	_, _ = s.Write([]byte("no-newline-yet"))
	restore() // pops: partial goes beneath
	if got, want := base.String(), "no-newline-yet\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// Overflow falls through to the destination beneath rather than blocking or
// dropping.
func TestCallback_OverflowFallsThrough(t *testing.T) {
	var base strings.Builder
	s := newStream("stdout", &base)

	// A callback with no live loop never drains, so the queue fills.
	cb := &lineCallback{queueCap: 2}
	s.push(destination{kind: destCallback, cb: cb})
	for i := 0; i < 5; i++ {
		_, _ = s.Write([]byte("line\n"))
	}
	// 2 queued, 3 fell through.
	if got, want := strings.Count(base.String(), "line\n"), 3; got != want {
		t.Fatalf("fell through %d lines, want %d (base=%q)", got, want, base.String())
	}
	s.reset()
}

// With no live loop to drain it, whatever is still queued when the callback
// is popped is written to the destination beneath — in order, followed by
// any trailing partial line. This is the guarantee that replaced holding the
// run open for delivery: a line queued moments before the run ends reaches
// the destination beneath rather than the handler, but it is never dropped.
func TestCallback_QueuedLinesFlushedBeneathOnPop(t *testing.T) {
	var base strings.Builder
	s := newStream("stdout", &base)

	// A callback with no live loop never drains, so writes just accumulate.
	cb := &lineCallback{queueCap: lineQueueCap}
	restore := s.push(destination{kind: destCallback, cb: cb})
	_, _ = s.Write([]byte("one\ntwo\nthree\n"))
	_, _ = s.Write([]byte("trailing-partial"))
	restore() // pops: queued lines land first, then the partial

	if got, want := base.String(), "one\ntwo\nthree\ntrailing-partial\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// A tee'd callback already writes each write's raw bytes to the destination
// beneath as they arrive (stream.writeAt's tee branch). Whatever is still
// queued or buffered when the entry is popped must NOT be flushed beneath a
// second time — that would duplicate every undelivered line, up to
// queueCap of them.
func TestCallback_TeeDoesNotDuplicateOnPop(t *testing.T) {
	var base strings.Builder
	s := newStream("stdout", &base)

	// A callback with no live loop never drains, so these lines and the
	// trailing partial stay queued/buffered until pop — the exact window
	// the tee-and-flush-beneath overlap would double-write in.
	cb := &lineCallback{queueCap: lineQueueCap}
	restore := s.push(destination{kind: destCallback, cb: cb, tee: true})
	_, _ = s.Write([]byte("one\ntwo\n"))
	_, _ = s.Write([]byte("trailing-partial"))
	restore()

	// The tee already wrote both writes' raw bytes beneath as they
	// happened; popping must add nothing further.
	if got, want := base.String(), "one\ntwo\ntrailing-partial"; got != want {
		t.Fatalf("got %q want %q (a repeat of \"one\\ntwo\\n\" or \"trailing-partial\\n\" means a duplicate)", got, want)
	}
}

// A TEE'd callback whose queue overflows must not write the overflowing bytes
// beneath twice. writeDest's fall-through and writeAt's tee branch both target
// the destination beneath, so an unconditional fall-through doubled every
// overflowing line — breaking the delivered-once half of the contract.
func TestCallback_TeeOverflowFallsThroughBeneathOnce(t *testing.T) {
	var base strings.Builder
	s := newStream("stdout", &base)

	// No live loop, so nothing ever drains: the first 2 lines queue and the
	// remaining 3 overflow.
	cb := &lineCallback{queueCap: 2}
	s.push(destination{kind: destCallback, cb: cb, tee: true})
	for i := 0; i < 5; i++ {
		_, _ = s.Write([]byte("line\n"))
	}
	s.reset() // tee'd: the pop flush is skipped, so this adds nothing

	// The tee already wrote all 5 writes beneath as they arrived. The 3
	// overflowing writes must not appear a second time.
	if got, want := strings.Count(base.String(), "line\n"), 5; got != want {
		t.Fatalf("base saw %d lines, want %d (base=%q)", got, want, base.String())
	}
}

// A TEE'd handler that itself logs — the documented normal case for a tee'd
// logger — re-enters on every line. That re-entrant write falls through to the
// destination beneath, and the tee branch writes it beneath as well, so an
// unconditional fall-through double-printed every single line.
func TestCallback_TeeReentrantWriteReachesBeneathOnce(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdout.to(line => { console.log("echo:" + line); }, { tee: true });
		console.log("trigger");
		await runtime.time.sleep(20);
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	// "trigger" reaches the base once, via the tee, at write time. The
	// handler's own console.log is refused by the re-entrancy guard and reaches
	// the base once, via the tee again — not twice.
	if got, want := out.String(), "trigger\necho:trigger\n"; got != want {
		t.Fatalf("got %q want %q (a repeated \"echo:trigger\" is the double-write)", got, want)
	}
}

// A handler that throws on every line must not lose output silently: each
// thrown line has already left the queue by the time the throw is
// detected, so it can't also be flushed to the destination beneath the way
// a still-queued or partial line can. Exactly one diagnostic reaches the
// real process stderr — not the redirectable stream, and not once per
// thrown line — no matter how many lines throw.
func TestCallback_ThrowingHandlerReportsOnceOnRealStderr(t *testing.T) {
	_, _, restoreStdio := withCapturedStdio(t)
	defer restoreStdio()
	finish := captureRealStderr(t)

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdout.to(line => { throw new Error("boom: " + line); });
		console.log("one");
		console.log("two");
		console.log("three");
		await runtime.time.sleep(10);
	`); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := finish()
	const marker = "sercon: stdout callback handler threw:"
	if n := strings.Count(got, marker); n != 1 {
		t.Fatalf("got %d diagnostic lines for 3 throws, want exactly 1 (stderr=%q)", n, got)
	}
	if !strings.Contains(got, "boom: one") {
		t.Fatalf("diagnostic should report the first (oldest-delivered) throw, got %q", got)
	}
}
