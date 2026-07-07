//go:build windows

package main

import (
	"os/exec"
	"time"
)

// configureProcessTermination sets a WaitDelay backstop on Windows.
//
// Tree-killing a process and all its descendants on Windows requires Job
// Objects, which is out of scope here (and the exec.shell timeout tests
// are skipped on Windows). The WaitDelay still bounds how long cmd.Wait
// blocks on lingering pipes after the context kills the direct process.
func configureProcessTermination(cmd *exec.Cmd) {
	cmd.WaitDelay = 2 * time.Second
}

// killProcessGroup is a no-op on Windows: the PTY path is unix-only
// (startPTY returns errPTYUnsupported there), so this is never reached.
func killProcessGroup(_ *exec.Cmd) {}
