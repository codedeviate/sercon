//go:build !windows

package main

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/term"
)

// runInteractive runs cmd wired to sercon's own terminal.
//
// When stdin is a real TTY it allocates a pty, puts the terminal into raw mode,
// and copies bytes in both directions — forwarding SIGWINCH resizes — so
// full-screen and `-it` programs behave correctly and keystrokes (including
// Ctrl-C, which the child's tty line discipline turns into SIGINT for the
// child) pass straight through. Terminal state is restored on every exit path.
//
// When stdin is not a TTY (pipes, CI, `make demo`) it inherits the parent's
// stdio directly with no raw mode — enough for non-interactive invocations and
// safe to run headless.
//
// Cancellation/timeout is handled by exec.CommandContext (cmd was built with
// the run context): when it fires, Go kills the child, the output copy sees EOF,
// and Wait returns — the deferred restores below always run.
func runInteractive(cmd *exec.Cmd) error {
	stdinFd := int(os.Stdin.Fd())
	if !term.IsTerminal(stdinFd) {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = ptmx.Close() }()

	// Forward terminal-size changes to the pty (and set the initial size).
	winch := make(chan os.Signal, 1)
	signal.Notify(winch, syscall.SIGWINCH)
	defer func() {
		signal.Stop(winch)
		close(winch) // ends the goroutine below; Stop precedes it so no send races
	}()
	go func() {
		for range winch {
			_ = pty.InheritSize(os.Stdin, ptmx)
		}
	}()
	winch <- syscall.SIGWINCH // initial sizing

	// Raw mode so keystrokes reach the child's tty unbuffered and uninterpreted.
	if oldState, rerr := term.MakeRaw(stdinFd); rerr == nil {
		defer func() { _ = term.Restore(stdinFd, oldState) }()
	}

	go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	_, _ = io.Copy(os.Stdout, ptmx) // returns when the child exits / pty closes
	return cmd.Wait()
}
