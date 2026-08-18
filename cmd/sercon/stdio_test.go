package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// withCapturedStdio points both registry streams at buffers for the duration
// of a test. It replaces the old consoleOut/consoleErr var swap: because the
// stream objects are stable and only their destinations move, tests capture by
// pushing a destination rather than by reassigning a package var.
func withCapturedStdio(t *testing.T) (out, errb *bytes.Buffer, restore func()) {
	t.Helper()
	out, errb = &bytes.Buffer{}, &bytes.Buffer{}
	ro := stdioOutStream.push(destination{kind: destBuffer, w: out})
	re := stdioErrStream.push(destination{kind: destBuffer, w: errb})
	return out, errb, func() { ro(); re() }
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

	// Neither write may recurse; both land on a process stream exactly once.
	_, _ = stdioOutStream.Write([]byte("o\n"))
	_, _ = stdioErrStream.Write([]byte("e\n"))

	if got, want := errBase.String(), "o\n"; got != want {
		t.Fatalf("stderr base: got %q want %q", got, want)
	}
	if got, want := outBase.String(), "e\n"; got != want {
		t.Fatalf("stdout base: got %q want %q", got, want)
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

func newTestEngine(t *testing.T) *scriptengine.Engine {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	return eng
}

func testCtx() context.Context { return context.Background() }
