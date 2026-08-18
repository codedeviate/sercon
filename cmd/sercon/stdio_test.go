package main

import (
	"bytes"
	"context"
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

func newTestEngine(t *testing.T) *scriptengine.Engine {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	return eng
}

func testCtx() context.Context { return context.Background() }
