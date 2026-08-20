package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// withCapturedStdio points both registry streams at buffers for the duration
// of a test by swapping the *stream package vars themselves (same technique
// as TestDest_CrossStreamFoldHasNoCycle), rather than pushing a destination
// onto the existing stream's stack. Two things need that:
//
//   - runtime.stdout / runtime.stderr binding tests exercise a one-shot
//     redirect (silence/to/toFile) and never call the restore their script
//     received — checking that one behaviour is the whole point of the test.
//     Pushing onto the shared package-level stream and popping only the
//     capture entry (by id) would leave that redirect on the stream forever,
//     for every later test in the package to trip over.
//   - runOne now calls resetStdio() at the start of every run, which drops
//     every entry on the stream's STACK. A pushed capture sits on that stack
//     and would be wiped before the script under test ever wrote a byte. A
//     swapped-in stream starts with an empty stack and the capture buffer as
//     its BASE — reset never touches base, so it survives.
func withCapturedStdio(t *testing.T) (out, errb *bytes.Buffer, restore func()) {
	t.Helper()
	out, errb = &bytes.Buffer{}, &bytes.Buffer{}
	oldOut, oldErr := stdioOutStream, stdioErrStream
	stdioOutStream = newStream("stdout", out)
	stdioErrStream = newStream("stderr", errb)
	var once sync.Once
	restore = func() {
		once.Do(func() {
			// Release whatever the script pushed and never popped (an open
			// file from toFile, a live line callback) before dropping the
			// swapped-in stream — closeDest only runs via reset()/pop(),
			// never by falling out of scope.
			stdioOutStream.reset()
			stdioErrStream.reset()
			stdioOutStream, stdioErrStream = oldOut, oldErr
		})
	}
	// Also registered as a Cleanup, so the package vars are restored even if a
	// test forgets its `defer restore()` or a helper t.Fatals before reaching
	// it. sync.Once makes the second call a no-op rather than resetting the
	// real streams a second time.
	t.Cleanup(restore)
	return out, errb, restore
}

// A bare stream writes to the process stream it was constructed with.
func TestStream_BaseWrite(t *testing.T) {
	var base bytes.Buffer
	s := newStream("stdout", &base)
	if _, err := s.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if got, want := base.String(), "hello\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// push redirects; the returned restore pops back to the base.
func TestStream_PushRestore(t *testing.T) {
	var base, redir bytes.Buffer
	s := newStream("stdout", &base)
	restore := s.push(destination{kind: destBuffer, w: &redir})
	_, _ = s.Write([]byte("a\n"))
	restore()
	_, _ = s.Write([]byte("b\n"))
	if got, want := redir.String(), "a\n"; got != want {
		t.Fatalf("redirected: got %q want %q", got, want)
	}
	if got, want := base.String(), "b\n"; got != want {
		t.Fatalf("base: got %q want %q", got, want)
	}
}

// restore is idempotent: calling it twice must not pop an unrelated entry.
func TestStream_RestoreIdempotent(t *testing.T) {
	var base, first, second bytes.Buffer
	s := newStream("stdout", &base)
	r1 := s.push(destination{kind: destBuffer, w: &first})
	r1()
	r1() // second call must be a no-op
	r2 := s.push(destination{kind: destBuffer, w: &second})
	_, _ = s.Write([]byte("x\n"))
	r2()
	if got, want := second.String(), "x\n"; got != want {
		t.Fatalf("second: got %q want %q", got, want)
	}
	if base.Len() != 0 {
		t.Fatalf("base should be empty, got %q", base.String())
	}
}

// Out-of-order restore removes the right entry and leaves the rest intact.
func TestStream_RestoreOutOfOrder(t *testing.T) {
	var base, mid, top bytes.Buffer
	s := newStream("stdout", &base)
	rMid := s.push(destination{kind: destBuffer, w: &mid})
	rTop := s.push(destination{kind: destBuffer, w: &top})
	rMid() // pop the middle entry while `top` is still effective
	_, _ = s.Write([]byte("t\n"))
	rTop()
	_, _ = s.Write([]byte("b\n"))
	if got, want := top.String(), "t\n"; got != want {
		t.Fatalf("top: got %q want %q", got, want)
	}
	if mid.Len() != 0 {
		t.Fatalf("mid should be empty, got %q", mid.String())
	}
	if got, want := base.String(), "b\n"; got != want {
		t.Fatalf("base: got %q want %q", got, want)
	}
}

// reset drops every redirect at once.
func TestStream_Reset(t *testing.T) {
	var base, a, b bytes.Buffer
	s := newStream("stdout", &base)
	s.push(destination{kind: destBuffer, w: &a})
	s.push(destination{kind: destBuffer, w: &b})
	s.reset()
	_, _ = s.Write([]byte("z\n"))
	if got, want := base.String(), "z\n"; got != want {
		t.Fatalf("base: got %q want %q", got, want)
	}
}

// The console shim, runtime.log and printRunResult all go through the registry.
func TestRegistry_ConsoleAndRuntimeLogRouted(t *testing.T) {
	out, errb, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		console.log("out-line");
		console.error("err-line");
		runtime.log("rt-line");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := out.String(); !strings.Contains(got, "out-line") || !strings.Contains(got, "rt-line") {
		t.Fatalf("stdout missing routed lines: %q", got)
	}
	if got, want := errb.String(), "err-line\n"; got != want {
		t.Fatalf("stderr: got %q want %q", got, want)
	}
}

// text.printf is script-facing output exactly like console.log / runtime.log
// and must go through the same registry.
func TestRegistry_TextPrintfRouted(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `text.str.printf("%s=%d\n", "x", 42);`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "x=42\n"; got != want {
		t.Fatalf("printf: got %q want %q", got, want)
	}
}

// silence: a null destination swallows output without touching the base.
func TestDest_Null(t *testing.T) {
	var base bytes.Buffer
	s := newStream("stdout", &base)
	restore := s.push(nullDest(false))
	_, _ = s.Write([]byte("swallowed\n"))
	restore()
	_, _ = s.Write([]byte("kept\n"))
	if got, want := base.String(), "kept\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// A file destination truncates by default and appends on request.
func TestDest_FileTruncateAndAppend(t *testing.T) {
	var base bytes.Buffer
	path := filepath.Join(t.TempDir(), "log.txt")

	s := newStream("stdout", &base)
	d, err := fileDest(path, false, false)
	if err != nil {
		t.Fatalf("fileDest: %v", err)
	}
	restore := s.push(d)
	_, _ = s.Write([]byte("first\n"))
	restore() // closes the file

	d2, err := fileDest(path, true, false) // append
	if err != nil {
		t.Fatalf("fileDest append: %v", err)
	}
	restore2 := s.push(d2)
	_, _ = s.Write([]byte("second\n"))
	restore2()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "first\nsecond\n"; string(got) != want {
		t.Fatalf("append: got %q want %q", got, want)
	}

	d3, err := fileDest(path, false, false) // truncate
	if err != nil {
		t.Fatalf("fileDest truncate: %v", err)
	}
	restore3 := s.push(d3)
	_, _ = s.Write([]byte("third\n"))
	restore3()

	got, err = os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "third\n"; string(got) != want {
		t.Fatalf("truncate: got %q want %q", got, want)
	}
}

// fileDest reports an unopenable path at the call site, not at write time.
func TestDest_FileOpenError(t *testing.T) {
	if _, err := fileDest(filepath.Join(t.TempDir(), "no-such-dir", "x.log"), false, false); err == nil {
		t.Fatal("expected an error for a missing parent directory")
	}
}

// tee writes to the new destination AND to the destination beneath it.
func TestDest_Tee(t *testing.T) {
	var base, sink bytes.Buffer
	s := newStream("stdout", &base)
	restore := s.push(destination{kind: destBuffer, w: &sink, tee: true})
	_, _ = s.Write([]byte("both\n"))
	restore()
	if got, want := sink.String(), "both\n"; got != want {
		t.Fatalf("sink: got %q want %q", got, want)
	}
	if got, want := base.String(), "both\n"; got != want {
		t.Fatalf("base: got %q want %q", got, want)
	}
}

// tee resolves to the destination beneath, not unconditionally to the base:
// teeing on top of a silence() writes only to the new destination.
func TestDest_TeeOntoSilence(t *testing.T) {
	var base, sink bytes.Buffer
	s := newStream("stdout", &base)
	s.push(nullDest(false))
	s.push(destination{kind: destBuffer, w: &sink, tee: true})
	_, _ = s.Write([]byte("only-sink\n"))
	if got, want := sink.String(), "only-sink\n"; got != want {
		t.Fatalf("sink: got %q want %q", got, want)
	}
	if base.Len() != 0 {
		t.Fatalf("base should be empty, got %q", base.String())
	}
	s.reset()
}

// A cross-stream fold resolves to the PROCESS stream, so two streams folded at
// each other cannot ping-pong.
func TestDest_CrossStreamFoldHasNoCycle(t *testing.T) {
	outBase, errBase := &bytes.Buffer{}, &bytes.Buffer{}
	oldOut, oldErr := stdioOutStream, stdioErrStream
	stdioOutStream = newStream("stdout", outBase)
	stdioErrStream = newStream("stderr", errBase)
	defer func() { stdioOutStream, stdioErrStream = oldOut, oldErr }()

	// Push a decoy onto each stream's OWN stack before resolving the fold
	// that targets it. processStreamDest must resolve to the PROCESS stream
	// (the base) and bypass these decoys entirely; an implementation that
	// instead resolved to "whatever the target stream's current effective
	// destination is" would land here instead — the discriminating case a
	// prior version of this test missed (it resolved both folds while both
	// stacks were still empty, so either implementation agreed).
	var errDecoy, outDecoy bytes.Buffer
	stdioErrStream.push(destination{kind: destBuffer, w: &errDecoy}) // target of stdout's fold
	stdioOutStream.push(destination{kind: destBuffer, w: &outDecoy}) // target of stderr's fold

	dOut, err := processStreamDest("stderr", false)
	if err != nil {
		t.Fatalf("processStreamDest: %v", err)
	}
	dErr, err := processStreamDest("stdout", false)
	if err != nil {
		t.Fatalf("processStreamDest: %v", err)
	}
	stdioOutStream.push(dOut) // stdout -> stderr
	stdioErrStream.push(dErr) // stderr -> stdout

	// Neither write may recurse; both must land on the process stream exactly
	// once, bypassing the decoy sitting on the target stream's own stack.
	_, _ = stdioOutStream.Write([]byte("o\n"))
	_, _ = stdioErrStream.Write([]byte("e\n"))

	if got, want := errBase.String(), "o\n"; got != want {
		t.Fatalf("stderr base: got %q want %q", got, want)
	}
	if got, want := outBase.String(), "e\n"; got != want {
		t.Fatalf("stdout base: got %q want %q", got, want)
	}
	if errDecoy.Len() != 0 {
		t.Fatalf("fold must bypass stderr's own stack, got %q", errDecoy.String())
	}
	if outDecoy.Len() != 0 {
		t.Fatalf("fold must bypass stdout's own stack, got %q", outDecoy.String())
	}
}

// failingWriter fails its first `failures` writes with a fixed error, then
// records everything handed to it afterwards. Lets a test drive stream.failover
// deterministically without needing a real full disk.
type failingWriter struct {
	failures int
	got      strings.Builder
}

func (w *failingWriter) Write(p []byte) (int, error) {
	if w.failures > 0 {
		w.failures--
		return 0, errors.New("simulated write failure")
	}
	return w.got.Write(p)
}

// A failed write on the BASE destination must not silence the process stream.
// There is nothing beneath the base to fall over to, and reset() is
// truncateTo(0), which never touches base — so demoting it to destNull (as an
// earlier version did unconditionally) killed stdout for the life of the
// process, not even recoverable by runtime.stdout.reset() or the between-Run
// resetStdio(). One transient error would take out a whole --watch session.
func TestFailover_BaseKeepsWritingAfterATransientError(t *testing.T) {
	finish := captureRealStderr(t)

	w := &failingWriter{failures: 1}
	s := newStream("stdout", w)
	_, _ = s.Write([]byte("during-the-failure\n")) // this one is lost, unavoidably
	_, _ = s.Write([]byte("kept-1\n"))
	_, _ = s.Write([]byte("kept-2\n"))
	s.reset() // the recovery must not depend on a reset — nor be undone by one
	_, _ = s.Write([]byte("kept-3\n"))
	diag := finish()

	if got, want := w.got.String(), "kept-1\nkept-2\nkept-3\n"; got != want {
		t.Fatalf("later writes must still reach the underlying writer: got %q want %q", got, want)
	}
	if s.base.kind != destStream {
		t.Fatalf("base kind is %v; the process stream must never be demoted to destNull", s.base.kind)
	}
	// Reported once (the d.failed flag), and worded as a write failure — at
	// level 0 there is no redirect to name, so "redirect to stdout failed"
	// would describe something that does not exist.
	if n := strings.Count(diag, "sercon: stdout write failed:"); n != 1 {
		t.Fatalf("got %d base-write diagnostics, want exactly 1 (stderr=%q)", n, diag)
	}
	if strings.Contains(diag, "redirect to") {
		t.Fatalf("base failure must not be described as a redirect: %q", diag)
	}
}

// A failed write on a STACKED destination falls through to the destination
// beneath it, and reports once — not once per line.
func TestFailover_StackedFallsThroughBeneathAndReportsOnce(t *testing.T) {
	finish := captureRealStderr(t)

	var base strings.Builder
	s := newStream("stdout", &base)
	bad := &failingWriter{failures: 100} // never recovers
	// Shaped like a toFile redirect onto a full disk (file left nil: this entry
	// owns no *os.File for closeDest to close).
	s.push(destination{kind: destFile, w: bad, path: "/tmp/full-disk.log"})

	_, _ = s.Write([]byte("a\n"))
	_, _ = s.Write([]byte("b\n"))
	_, _ = s.Write([]byte("c\n"))
	s.reset()
	diag := finish()

	// Every write reaches the destination beneath — here the base, since this is
	// the only redirect. The failing one falls through from failover; the entry
	// is then marked dead, and writeDest routes each later write beneath as
	// well, so a destination going bad costs at most the write in flight when it
	// failed. Nothing after it is dropped.
	if got, want := base.String(), "a\nb\nc\n"; got != want {
		t.Fatalf("base: got %q want %q", got, want)
	}
	// Attempted exactly once: a dead entry is never written to again, so the
	// second and third writes never reached the broken writer (and so there is
	// only ever one error to report).
	if got := 100 - bad.failures; got != 1 {
		t.Fatalf("the broken writer saw %d write attempts, want 1", got)
	}
	if n := strings.Count(diag, "failed:"); n != 1 {
		t.Fatalf("got %d diagnostics for 3 writes, want exactly 1 (stderr=%q)", n, diag)
	}
	if !strings.Contains(diag, "redirect to /tmp/full-disk.log failed:") {
		t.Fatalf("a stacked failure should name the redirect: %q", diag)
	}
}

// A TEE'd destination whose own write fails must write each line beneath
// exactly once. Two overlapping fall-through routes make this easy to get
// wrong, and both must stay conditional on !d.tee:
//
//   - the failing write ("a"): failover used to fall through to level-1 and
//     then writeAt's tee branch wrote the same bytes beneath again, giving
//     "a\na\nb\n";
//   - every write AFTER the failure ("b"): writeDest's dead branch routes it
//     beneath, and writeAt's tee branch would write it beneath again.
func TestFailover_TeeWritesEachLineBeneathOnce(t *testing.T) {
	finish := captureRealStderr(t)

	var base strings.Builder
	s := newStream("stdout", &base)
	bad := &failingWriter{failures: 100}
	s.push(destination{kind: destBuffer, w: bad, tee: true})

	_, _ = s.Write([]byte("a\n")) // fails, marks the entry dead
	_, _ = s.Write([]byte("b\n")) // takes the dead route
	_, _ = s.Write([]byte("c\n"))
	s.reset()
	diag := finish()

	if got, want := base.String(), "a\nb\nc\n"; got != want {
		t.Fatalf("base: got %q want %q (a repeated line is the double-write)", got, want)
	}
	if got := 100 - bad.failures; got != 1 {
		t.Fatalf("the broken writer saw %d write attempts, want 1", got)
	}
	if n := strings.Count(diag, "failed:"); n != 1 {
		t.Fatalf("got %d diagnostics, want exactly 1 (stderr=%q)", n, diag)
	}
}

// targetInfo describes the effective destination.
func TestStream_TargetInfo(t *testing.T) {
	var base bytes.Buffer
	s := newStream("stdout", &base)
	if got := s.targetInfo(); got["kind"] != "stream" || got["name"] != "stdout" || got["depth"] != 0 {
		t.Fatalf("base: %#v", got)
	}
	path := filepath.Join(t.TempDir(), "t.log")
	d, err := fileDest(path, true, true)
	if err != nil {
		t.Fatalf("fileDest: %v", err)
	}
	restore := s.push(d)
	got := s.targetInfo()
	if got["kind"] != "file" || got["path"] != path || got["append"] != true || got["tee"] != true || got["depth"] != 1 {
		t.Fatalf("file: %#v", got)
	}
	restore()
}

// silence() from a script swallows output; the returned restore brings it back.
func TestBinding_Silence(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		const r = runtime.stdout.silence();
		console.log("hidden");
		r();
		console.log("visible");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "visible\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// toFile writes to disk; append is honoured.
func TestBinding_ToFile(t *testing.T) {
	_, _, restore := withCapturedStdio(t)
	defer restore()

	path := filepath.Join(t.TempDir(), "s.log")
	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		const p = `+strconv.Quote(path)+`;
		let r = runtime.stdout.toFile(p);
		console.log("one");
		r();
		r = runtime.stdout.toFile(p, { append: true });
		console.log("two");
		r();
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "one\ntwo\n"; string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// A cross-stream fold moves console.log onto stderr.
func TestBinding_FoldToStderr(t *testing.T) {
	out, errb, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdout.to("stderr");
		console.log("folded");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout should be empty, got %q", out.String())
	}
	if got := errb.String(); !strings.Contains(got, "folded") {
		t.Fatalf("stderr missing folded line: %q", got)
	}
}

// tee writes both places.
func TestBinding_Tee(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	path := filepath.Join(t.TempDir(), "tee.log")
	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		runtime.stdout.toFile(`+strconv.Quote(path)+`, { tee: true });
		console.log("seen-twice");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "seen-twice\n"; string(got) != want {
		t.Fatalf("file: got %q want %q", got, want)
	}
	if want := "seen-twice\n"; out.String() != want {
		t.Fatalf("stdout: got %q want %q", out.String(), want)
	}
}

// target() reports the effective destination.
func TestBinding_Target(t *testing.T) {
	_, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		runtime.stderr.silence();
		export default runtime.stderr.target().kind;
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got := val.Export(); got != "null" {
		t.Fatalf("target kind: got %v want \"null\"", got)
	}
}

// tee onto the void is a script bug, caught at the call site.
func TestBinding_TeeNullThrows(t *testing.T) {
	_, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	_, err := eng.Run(testCtx(), "s.ts", `runtime.stdout.to("null", { tee: true });`)
	if err == nil {
		t.Fatal("expected a throw for to(\"null\", { tee: true })")
	}
	if !strings.Contains(err.Error(), "tee") {
		t.Fatalf("error should mention tee: %v", err)
	}
}

// An unopenable file throws at the call site rather than at write time.
func TestBinding_ToFileOpenErrorThrows(t *testing.T) {
	_, _, restore := withCapturedStdio(t)
	defer restore()

	bad := filepath.Join(t.TempDir(), "missing-dir", "x.log")
	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `runtime.stdout.toFile(`+strconv.Quote(bad)+`);`); err == nil {
		t.Fatal("expected a throw for an unopenable path")
	}
}

// resetStdio between runs gives the second script clean streams.
func TestRegistry_ResetBetweenRuns(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "a.ts", `runtime.stdout.silence(); console.log("a-hidden");`); err != nil {
		t.Fatalf("run a: %v", err)
	}
	// runOne calls resetStdio() before each engine call; call the real thing
	// directly rather than through a test-only alias — with withCapturedStdio
	// now swapping the *stream package vars, the capture buffer is the
	// swapped stream's base, which resetStdio() (truncateTo(0) on both
	// streams) never touches, so it's already safe to call here unmodified.
	resetStdio()
	if _, err := eng.Run(testCtx(), "b.ts", `console.log("b-visible");`); err != nil {
		t.Fatalf("run b: %v", err)
	}
	if got, want := out.String(), "b-visible\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// scoped applies the redirect for the callback's duration and restores after.
func TestBinding_ScopedRestores(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		await runtime.stdout.scoped("null", async () => {
			console.log("hidden");
			await runtime.time.sleep(5);
			console.log("also-hidden");
		});
		console.log("visible");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "visible\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// scoped restores even when the callback throws, and the throw propagates.
func TestBinding_ScopedRestoresOnThrow(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		let threw = false;
		try {
			await runtime.stdout.scoped("null", () => { throw new Error("boom"); });
		} catch (e) {
			threw = true;
			// The original Error must round-trip through the rejection, not an
			// opaque wrapped Go value: pins exceptionValue against a regression
			// back to vm.ToValue(err) / vm.NewGoError(err).
			runtime.assert.ok(e instanceof Error, "caught value must be an Error");
			runtime.assert.ok(e.message === "boom", "message must round-trip, got " + e.message);
		}
		runtime.assert.ok(threw, "the throw must propagate");
		console.log("visible");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "visible\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// scoped restores even when reading the callback's result's `then` property
// panics. goja's (*Object).Get panics with a *goja.Exception when a getter
// throws (a JS getter, or a revoked Proxy) — settleAfter calls Get("then")
// unguarded, and by then the redirect has already been pushed. callScopedFn
// must still restore before that panic continues on its way to becoming the
// script's catchable throw.
func TestBinding_ScopedRestoresOnThenGetterThrow(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		let threw = false;
		try {
			await runtime.stdout.scoped("null", () => ({ get then() { throw new Error("boom"); } }));
		} catch (e) {
			threw = true;
		}
		runtime.assert.ok(threw, "the throw must propagate");
		console.log("visible");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "visible\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// scoped restores when an async callback's returned promise REJECTS (as
// opposed to TestBinding_ScopedRestoresOnThrow, which covers a synchronous
// throw) — exercising settleAfter's onErr branch, the other cleanup() call
// site, with the same fidelity check as the synchronous-throw test.
func TestBinding_ScopedRestoresOnAsyncReject(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		let threw = false;
		try {
			await runtime.stdout.scoped("null", async () => {
				await runtime.time.sleep(5);
				throw new Error("boom");
			});
		} catch (e) {
			threw = true;
			runtime.assert.ok(e instanceof Error, "caught value must be an Error");
			runtime.assert.ok(e.message === "boom", "message must round-trip, got " + e.message);
		}
		runtime.assert.ok(threw, "the throw must propagate");
		console.log("visible");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "visible\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// scoped preserves the thrown Error's identity when calling result.then
// itself throws synchronously — distinct from TestBinding_ScopedRestoresOnAsyncReject
// (a normal rejection delivered through the onErr callback, which already
// carried the real value through call.Argument(0)): this hits settleAfter's
// OTHER cleanup+reject site, `if _, err := then(obj, onOK, onErr); err !=
// nil`, which used to wrap err in vm.NewGoError directly instead of going
// through exceptionValue.
func TestBinding_ScopedRestoresOnThenCallThrow(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	if _, err := eng.Run(testCtx(), "s.ts", `
		let threw = false;
		try {
			await runtime.stdout.scoped("null", () => ({
				then() { throw new Error("boom"); },
			}));
		} catch (e) {
			threw = true;
			runtime.assert.ok(e instanceof Error, "caught value must be an Error");
			runtime.assert.ok(e.message === "boom", "message must round-trip, got " + e.message);
		}
		runtime.assert.ok(threw, "the throw must propagate");
		console.log("visible");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := out.String(), "visible\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// scoped accepts an optional opts object between the target and the callback.
func TestBinding_ScopedWithOpts(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	path := filepath.Join(t.TempDir(), "scoped.log")
	eng := newTestEngine(t)
	// The trailing console.log AFTER the scope closes is the discriminating
	// part: if the sync-success restore leaked, "after" would still be teed
	// into the file (proving the redirect was never popped) instead of only
	// reaching stdout's base.
	if _, err := eng.Run(testCtx(), "s.ts", `
		await runtime.stdout.scoped({ file: `+strconv.Quote(path)+` }, { tee: true }, () => {
			console.log("teed");
		});
		console.log("after");
	`); err != nil {
		t.Fatalf("run: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if want := "teed\n"; string(got) != want {
		t.Fatalf("file: got %q want %q — restore must have leaked", got, want)
	}
	if want := "teed\nafter\n"; out.String() != want {
		t.Fatalf("stdout: got %q want %q", out.String(), want)
	}
}

// capture returns everything written during the callback, and nothing leaks.
func TestBinding_Capture(t *testing.T) {
	out, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		const text = await runtime.stdout.capture(async () => {
			console.log("captured-1");
			await runtime.time.sleep(5);
			console.log("captured-2");
		});
		export default text;
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "captured-1\ncaptured-2\n"; got != want {
		t.Fatalf("captured: got %q want %q", got, want)
	}
	if out.Len() != 0 {
		t.Fatalf("capture must be exclusive; stdout got %q", out.String())
	}
}

// capture works with a synchronous callback too.
func TestBinding_CaptureSyncFn(t *testing.T) {
	_, _, restore := withCapturedStdio(t)
	defer restore()

	eng := newTestEngine(t)
	val, err := eng.Run(testCtx(), "s.ts", `
		export default await runtime.stderr.capture(() => { console.error("sync"); });
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if got, want := val.Export(), "sync\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func newTestEngine(t *testing.T) *scriptengine.Engine {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	return eng
}

func testCtx() context.Context { return context.Background() }
