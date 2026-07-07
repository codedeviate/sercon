package main

import (
	"context"
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
		// /bin/sh exits at once; the backgrounded subshell ignores SIGHUP
		// and keeps the pty slave open for 5s.
		argv:    []string{"/bin/sh", "-c", "(trap '' HUP; sleep 5) &"},
		timeout: 500 * time.Millisecond,
		usePTY:  true,
	}
	start := time.Now()
	_, err := execShell(context.Background(), args)
	elapsed := time.Since(start)

	if elapsed > 3*time.Second {
		t.Fatalf("execShell blocked %s — timeout not enforced on the PTY path", elapsed)
	}
	if err == nil {
		t.Fatalf("expected a deadline error, got success after %s", elapsed)
	}
}
