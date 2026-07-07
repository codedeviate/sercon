//go:build !windows

package main

import (
	"os/exec"
	"syscall"
	"time"
)

// configureProcessTermination makes the subprocess the leader of its own
// process group and arranges for a timeout/cancel to SIGKILL the entire
// group rather than just the direct child.
//
// Why this matters: `services.exec.shell` runs string commands as
// `/bin/sh -c "<cmd>"`. On platforms where the shell forks the command
// instead of exec-ing into it, killing the shell on a context timeout
// leaves the grandchild (the actual command) alive — and it still holds
// the stdout/stderr pipe open, so cmd.Wait blocks until that grandchild
// exits on its own. That turns a 150ms timeout into a full-duration hang
// (observed on Linux CI; macOS happens to exec and so didn't reproduce).
// Killing the whole process group reaps the grandchild too.
//
// WaitDelay is a backstop: if a process escapes the group (e.g. starts a
// new session), Wait still returns within the delay instead of blocking
// indefinitely on the lingering pipe.
func configureProcessTermination(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process != nil {
			// A negative PID targets the whole process group.
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
}

// killProcessGroup SIGKILLs the whole process group led by cmd's process.
// The PTY path needs this to enforce a deadline after cmd.Wait has already
// returned: by then os/exec's own context watcher has exited, so cmd.Cancel
// never fires, and a descendant still holding the pty slave would otherwise
// block the output copy indefinitely. startPTY makes the child a session
// leader (pgid == pid), so the negative-PID signal reaches the whole tree.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
