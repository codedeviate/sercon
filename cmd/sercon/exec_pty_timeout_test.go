package main

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

// A PTY-mode shell whose direct child exits immediately but leaves a
// SIGHUP-surviving descendant holding the pty slave must still honour the
// timeout: execShell must return a deadline error promptly, not block until
// the descendant exits on its own.
//
// This is Linux-specific. On darwin the kernel revokes the slave tty when
// the session leader exits, so the copy loop unblocks on its own and the
// hang never occurs (the bug is structurally unreachable there); on Windows
// there is no PTY. The test therefore runs only on Linux — where the
// pre-fix code blocked for the full descendant lifetime and returned
// success:true instead of the deadline error.
func TestExecShell_PTYTimeoutKillsLingeringDescendant(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("PTY-slave-lingering hang is Linux-specific; %s revokes the slave on session-leader exit", runtime.GOOS)
	}
	args := execShellArgs{
		// /bin/sh exits at once; the backgrounded subshell ignores SIGHUP and
		// tries to hold the pty slave open for 30s. Whether that descendant
		// actually lingers (vs. being reaped / the slave revoked) is
		// kernel/runner-dependent, so the deterministic invariant we assert is
		// "execShell does not hang past the timeout" — NOT that a specific
		// deadline error is returned. Pre-fix, a lingering descendant blocked
		// the call until it exited (~30s); the fix bounds it to ~the timeout.
		argv:    []string{"/bin/sh", "-c", "(trap '' HUP; sleep 30) &"},
		timeout: 500 * time.Millisecond,
		usePTY:  true,
	}
	start := time.Now()
	_, err := execShell(context.Background(), args)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("execShell blocked %s on the PTY path — timeout not enforced (a descendant held the slave)", elapsed)
	}
	// err may be the deadline (descendant lingered) or nil (it didn't) — both
	// are bounded, which is the guarantee. If an error came back, it must be
	// the deadline, not some unrelated failure.
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unexpected error from bounded PTY exec: %v", err)
	}
}
