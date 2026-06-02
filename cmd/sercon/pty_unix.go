//go:build !windows

package main

import (
	"io"
	"os/exec"
	"syscall"
	"time"

	"github.com/creack/pty"
)

// startPTY starts cmd under a pseudo-terminal (darwin/linux) and returns the
// PTY master. pty.Start wires the child's stdin/stdout/stderr to the slave
// tty and starts the process, so callers must NOT set cmd.Std* or call
// cmd.Start/Run themselves. The child becomes a session leader (pty.Start
// sets Setsid), so cancellation kills the whole session via the negative-PID
// group signal — mirroring configureProcessTermination but without Setpgid,
// which would conflict with Setsid.
//
// cmd MUST have been created with exec.CommandContext: startPTY sets
// cmd.Cancel, which Go's exec package only permits on a context-backed Cmd.
func startPTY(cmd *exec.Cmd) (io.ReadWriteCloser, error) {
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	_ = pty.Setsize(ptmx, &pty.Winsize{Rows: 24, Cols: 80})
	return ptmx, nil
}
