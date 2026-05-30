package main

import (
	"bytes"
	"io"
	"testing"
)

// When paging is suppressed (--no-pager) or stdout isn't a terminal, output
// must render directly to stdout rather than spawning a pager.
func TestPageOutput_DirectWhenNoPager(t *testing.T) {
	var buf bytes.Buffer
	oldOut, oldTTY := pagerStdout, pagerIsTTYFn
	pagerStdout = &buf
	pagerIsTTYFn = func() bool { return true } // even on a TTY, --no-pager forces direct
	defer func() { pagerStdout, pagerIsTTYFn = oldOut, oldTTY }()

	pageOutput(true, func(w io.Writer) { _, _ = w.Write([]byte("hello")) })
	if buf.String() != "hello" {
		t.Fatalf("--no-pager should render directly; got %q", buf.String())
	}
}

func TestPageOutput_DirectWhenNotTTY(t *testing.T) {
	var buf bytes.Buffer
	oldOut, oldTTY := pagerStdout, pagerIsTTYFn
	pagerStdout = &buf
	pagerIsTTYFn = func() bool { return false } // piped / redirected
	defer func() { pagerStdout, pagerIsTTYFn = oldOut, oldTTY }()

	pageOutput(false, func(w io.Writer) { _, _ = w.Write([]byte("piped")) })
	if buf.String() != "piped" {
		t.Fatalf("non-TTY should render directly; got %q", buf.String())
	}
}
