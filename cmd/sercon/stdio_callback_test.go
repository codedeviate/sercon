package main

import (
	"strings"
	"testing"
)

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
	if got := len(cb.queue); got != 0 {
		t.Fatalf("queued %d lines before the newline, want 0", got)
	}
	_, _ = s.Write([]byte("a-line\n"))
	if got, want := len(cb.queue), 1; got != want {
		t.Fatalf("queued %d lines, want %d", got, want)
	}
	if got, want := cb.queue[0], "half-a-line"; got != want {
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
