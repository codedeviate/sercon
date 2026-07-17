package main

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/dop251/goja"
)

// services.exec.interactive(cmd, opts?) runs a subprocess wired straight to
// sercon's own controlling terminal — the interactive counterpart to
// exec.shell. Where shell captures stdout/stderr (and only feeds a fixed
// string to stdin), interactive inherits the parent's stdin/stdout/stderr so
// genuinely interactive children work: `docker exec -it`, `ssh`, interactive
// `mysql`/`redis-cli` REPLs, pagers, and full-screen TUIs.
//
// Because the child owns the terminal, nothing is captured — the result is just
// { exitCode, success, durationMs }. This deliberately separate verb avoids the
// "why is stdout empty?" surprise a shell({stdio:"inherit"}) option would carry.
//
// On Unix with a real TTY on stdin, runInteractive allocates a pty, puts the
// terminal into raw mode, and copies both directions (forwarding SIGWINCH); on
// a non-TTY stdin (pipes, CI, `make demo`) or on Windows it inherits the raw
// handles. Terminal state is restored on every exit path.

// execInteractiveArgs is the plain-Go bundle handed from execInteractiveExtract
// (on the event loop) to execInteractive (the work goroutine).
type execInteractiveArgs struct {
	argv    []string
	cwd     string
	env     []string
	timeout time.Duration // 0 = no timeout (run until the child exits)
}

// execInteractiveExtract parses (cmd, opts?) into execInteractiveArgs. It runs
// on the event loop — the only place the goja call may be touched.
func execInteractiveExtract(call goja.FunctionCall) (execInteractiveArgs, error) {
	var args execInteractiveArgs
	cmdArg := call.Argument(0)
	if cmdArg == nil || goja.IsUndefined(cmdArg) || goja.IsNull(cmdArg) {
		return args, errors.New("interactive: cmd argument required")
	}

	opts := optsAsMap(call)
	// No default timeout: an interactive shell / REPL is expected to run until
	// the user exits it, unlike exec.shell's 30s capture default.
	args.timeout = optMillis(opts, "timeout", 0)
	args.cwd = optString(opts, "cwd", "")
	args.env = buildEnv(opts)

	argv, err := buildArgv(cmdArg)
	if err != nil {
		return args, err
	}
	args.argv = argv
	return args, nil
}

func execInteractive(ctx context.Context, args execInteractiveArgs) (map[string]any, error) {
	runCtx := ctx
	if args.timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, args.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(runCtx, args.argv[0], args.argv[1:]...) //nolint:gosec // user-supplied argv is intentional
	if args.cwd != "" {
		cmd.Dir = args.cwd
	}
	if args.env != nil {
		cmd.Env = args.env
	}

	start := time.Now()
	runErr := runInteractive(cmd) // platform-specific: pty+raw on a TTY, inherit otherwise
	durationMs := time.Since(start).Milliseconds()

	exitCode := 0
	success := true
	if runErr != nil {
		// A deadline/cancel is qualitatively different from a non-zero exit —
		// the child never chose its own code. Surface those as throws. Check
		// ctx first because CommandContext wraps the kill in an *exec.ExitError.
		if ctxErr := runCtx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("interactive: %w", ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			success = false
		} else {
			return nil, fmt.Errorf("interactive: %w", runErr)
		}
	}

	return map[string]any{
		"exitCode":   exitCode,
		"success":    success,
		"durationMs": durationMs,
	}, nil
}
