package main

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// A PTY-mode shell whose direct child exits immediately but leaves a
// backgrounded descendant behind must return as soon as the child exits — it
// must NOT block until the descendant finishes on its own.
//
// The descendant is `setsid sleep 10`, launched in a NEW session so it is not
// in sh's process group (the deadline-path group-kill would otherwise reap it)
// and inherits sh's pty-slave fds. One might expect that to keep io.Copy(master)
// blocked until the sleeper exits (~10s). It does not: creack/pty makes sh the
// pty's session leader with the pty as its *controlling terminal*, so sh's exit
// hangs up the terminal and the master read unblocks immediately with EIO —
// independent of the sleeper still holding a slave fd. (This is the same
// session-leader-exit hangup darwin performs; with a controlling terminal Linux
// does it too, which is why the lingering-descendant *hang* is structurally
// unreachable in this code path and execShell reports clean success rather than
// a deadline.) The regression this guards against is the copy/Wait logic
// blocking for the descendant's full lifetime instead of returning at child
// exit — hence a large timeout (so a slow return can only mean "waited for the
// descendant", never "hit the timeout") and a tight elapsed bound.
//
// Linux-only: `setsid` is a util-linux command (absent on darwin) and the path
// needs a real pty + /bin/sh; Windows has no PTY.
func TestExecShell_PTYReturnsAtChildExitNotDescendant(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skipf("needs setsid + a real pty + /bin/sh; not exercised on %s", runtime.GOOS)
	}
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not on PATH; needed to background the descendant in its own session")
	}
	args := execShellArgs{
		argv: []string{"/bin/sh", "-c", "setsid sleep 10 &"},
		// Large on purpose: execShell must return when sh exits (~instantly),
		// so this timeout should never be reached. If a regression makes the
		// copy wait for the descendant, the return would be ~10s (still under
		// this timeout, so no deadline error) and the elapsed bound below fires.
		timeout: 30 * time.Second,
		usePTY:  true,
	}
	start := time.Now()
	out, err := execShell(context.Background(), args)
	elapsed := time.Since(start)

	if elapsed > 5*time.Second {
		t.Fatalf("execShell blocked %s on the PTY path — it waited for the lingering descendant instead of returning when the child exited", elapsed)
	}
	// The child exited 0 and the pty drained cleanly on hangup, so this is a
	// success, not a deadline: the descendant-holds-the-slave hang cannot occur
	// here, so the deadline branch never fires.
	if err != nil {
		t.Fatalf("expected clean success from the bounded PTY exec, got %v", err)
	}
	if out["success"] != true || out["exitCode"] != 0 {
		t.Fatalf("expected success=true / exitCode=0, got success=%v exitCode=%v", out["success"], out["exitCode"])
	}
}
