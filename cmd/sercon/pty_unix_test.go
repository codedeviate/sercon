//go:build !windows

package main

import (
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// startPTY must make the child believe stdout is a terminal: `test -t 1`
// succeeds only on a tty, so the child prints TTY (not PIPE). The Cmd is
// created via CommandContext because startPTY sets cmd.Cancel.
func TestStartPTY_ChildSeesTTY(t *testing.T) {
	cmd := exec.CommandContext(context.Background(), "sh", "-c", "test -t 1 && echo TTY || echo PIPE")
	master, err := startPTY(cmd)
	if err != nil {
		t.Fatalf("startPTY: %v", err)
	}
	defer master.Close()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() { _, _ = io.Copy(&buf, master); close(done) }()

	_ = cmd.Wait() // child exits 0; ignore (no ExitError expected)
	<-done

	if !strings.Contains(buf.String(), "TTY") {
		t.Fatalf("child did not see a tty; got %q", buf.String())
	}
}
