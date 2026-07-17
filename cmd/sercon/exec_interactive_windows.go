//go:build windows

package main

import (
	"os"
	"os/exec"
)

// runInteractive on Windows inherits the parent's standard handles directly.
// There is no pty allocation (sercon is pty-free on Windows, mirroring
// exec.shell's pipe fallback), so full-screen ConPTY programs are not driven
// here — but ordinary interactive console programs that read stdin and write
// stdout/stderr work. Cancellation/timeout is handled by exec.CommandContext.
func runInteractive(cmd *exec.Cmd) error {
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
