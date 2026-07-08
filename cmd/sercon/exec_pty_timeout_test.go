package main

import (
	"context"
	"errors"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// A PTY-mode shell whose direct child exits immediately but leaves a
// descendant holding the pty slave open must still honour the timeout:
// execShell must return the deadline error promptly, not block until the
// descendant exits on its own.
//
// Linux-only. On darwin the kernel revokes the slave tty when the session
// leader exits, so the copy loop unblocks on its own and the hang can't
// occur (the bug is structurally unreachable); on Windows there is no PTY.
//
// The descendant is `setsid sleep 10`, run in a NEW session: sh's
// session-leader exit therefore does not SIGHUP it and the kernel does not
// revoke its slave, so it deterministically holds the pty slave open — the
// exact lingering condition the fix addresses (a plain backgrounded sleep is
// runner-dependent). Pre-fix, execShell blocked until the sleeper exited
// (~10s); the fix's deadline path (kill the group + close the master to
// unblock the copy) returns in ~the timeout. The sleeper is in its own
// session, so the group-kill misses it — master.Close() is what unblocks the
// copy — and it exits on its own in ~10s.
func TestExecShell_PTYTimeoutKillsLingeringDescendant(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("PTY-slave-lingering hang is Linux-specific; %s revokes the slave on session-leader exit", runtime.GOOS)
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not on PATH; needed to hold the pty slave deterministically")
	}
	args := execShellArgs{
		argv:    []string{"/bin/sh", "-c", "setsid sleep 10 &"},
		timeout: 500 * time.Millisecond,
		usePTY:  true,
	}
	start := time.Now()
	_, err := execShell(context.Background(), args)
	elapsed := time.Since(start)

	if elapsed > 4*time.Second {
		t.Fatalf("execShell blocked %s on the PTY path — timeout not enforced against a lingering descendant", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a deadline error from the bounded PTY exec, got %v", err)
	}
}
