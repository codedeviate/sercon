//go:build !windows

package main

import (
	"io"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
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

	// Pump stdin → pty with an INTERRUPTIBLE reader that we join before
	// returning. A plain `go io.Copy(ptmx, os.Stdin)` would park in a blocking
	// read(os.Stdin) and outlive this call; on the next interactive() session a
	// second reader parks on the same fd, and the kernel may hand the user's
	// first keystroke to the stale reader — which writes it to the now-closed
	// old pty and drops it (brief 09). interruptibleCopy guarantees exactly one
	// reader at a time by stopping this one before we return.
	stopStdin, perr := interruptibleCopy(stdinFd, ptmx)
	if perr != nil {
		// Self-pipe creation failed (near-impossible). Fall back to the
		// fire-and-forget copy; the cross-session first-byte race is
		// best-effort in this degraded case rather than losing input entirely.
		go func() { _, _ = io.Copy(ptmx, os.Stdin) }()
	} else {
		defer stopStdin()
	}

	_, _ = io.Copy(os.Stdout, ptmx) // returns when the child exits / pty closes
	return cmd.Wait()
}

// interruptibleCopy copies bytes from the raw file descriptor srcFd to dst in a
// goroutine, returning stop(). stop() interrupts a parked read and blocks until
// the goroutine has exited, so no reader outlives the call — the invariant that
// keeps a finished session's stdin reader from stealing the next session's
// first keystroke.
//
// It reads srcFd only after unix.Poll reports data ready, so srcFd stays in
// blocking mode and its flags are never changed — a later non-interactive child
// that inherits os.Stdin sees a normal (blocking) fd. Cancellation uses the
// self-pipe trick: stop() writes to a pipe that Poll also watches, waking the
// parked Poll immediately.
func interruptibleCopy(srcFd int, dst io.Writer) (stop func(), err error) {
	var p [2]int
	if err := unix.Pipe(p[:]); err != nil {
		return nil, err
	}
	stopR, stopW := p[0], p[1]

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		fds := []unix.PollFd{
			{Fd: int32(srcFd), Events: unix.POLLIN},
			{Fd: int32(stopR), Events: unix.POLLIN},
		}
		for {
			fds[0].Revents, fds[1].Revents = 0, 0
			if _, perr := unix.Poll(fds, -1); perr != nil {
				if perr == unix.EINTR {
					continue
				}
				return
			}
			if fds[1].Revents != 0 { // stop() requested
				return
			}
			if fds[0].Revents&(unix.POLLIN|unix.POLLHUP|unix.POLLERR) == 0 {
				continue
			}
			n, rerr := unix.Read(srcFd, buf)
			if n > 0 {
				if _, werr := dst.Write(buf[:n]); werr != nil {
					return
				}
			}
			if n == 0 { // EOF
				return
			}
			if rerr != nil && rerr != unix.EINTR && rerr != unix.EAGAIN {
				return
			}
		}
	}()

	var once sync.Once
	stop = func() {
		once.Do(func() {
			_, _ = unix.Write(stopW, []byte{0})
			<-done
			_ = unix.Close(stopW)
			_ = unix.Close(stopR)
		})
	}
	return stop, nil
}
