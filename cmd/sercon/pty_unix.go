//go:build !windows

package main

import (
	"io"
	"os/exec"

	"github.com/creack/pty"
)

// startPTY starts cmd under a pseudo-terminal (darwin/linux) and returns the
// PTY master. pty.Start wires the child's stdin/stdout/stderr to the slave
// tty and starts the process, so callers must NOT set cmd.Std* or call
// cmd.Start/Run themselves. The child becomes a session leader (pty.Start
// sets Setsid), so cancellation must use Kill(-pid, SIGKILL) — callers are
// responsible for wiring cmd.Cancel accordingly (e.g. via
// configurePTYTermination) before calling startPTY.
func startPTY(cmd *exec.Cmd) (io.ReadCloser, error) {
	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})
	return ptmx, nil
}
