package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

// execShellExtract + execShell run a subprocess and wait for it to exit.
// The contract:
//
//   - `cmd: string` is passed verbatim to the host's command shell
//     (`/bin/sh -c "<cmd>"` on Unix, `cmd /C "<cmd>"` on Windows) so
//     quoting, pipes, and redirects work as a user would type them at
//     the prompt.
//   - `cmd: string[]` is treated as `argv`: no shell is involved and
//     the host runs argv[0] with the rest as parameters. Use this
//     form when arguments contain whitespace or shell metacharacters
//     you don't want re-interpreted.
//
// Result shape:
//
//	{
//	  stdout:    string,   // captured stdout
//	  stderr:    string,   // captured stderr
//	  exitCode:  number,   // 0 on success
//	  success:   boolean,  // exitCode === 0
//	  durationMs: number,  // wall-clock ms from spawn to exit
//	}
//
// Process-start failures (host binary not on PATH, permission denied)
// surface as Go errors → JS throws. Non-zero exits do **not** throw —
// they're a normal subprocess outcome, reported via `exitCode` /
// `success` so scripts can react without try/catch.
// execShellArgs is the plain-Go argument bundle handed from execShellExtract
// (on the event loop) to execShell (the work goroutine). pane is a Go-side
// tui.Pane handle resolved on-loop; the worker only writes bytes to it.
type execShellArgs struct {
	argv    []string
	timeout time.Duration
	cwd     string
	stdin   string
	usePTY  bool
	env     []string
	pane    tui.Pane
}

// execShellExtract parses the (cmd, opts?) goja arguments into execShellArgs.
// It runs synchronously on the event loop — the only place the goja call may
// be touched; a validation error here rejects the Promise exactly like a
// work error.
func execShellExtract(call goja.FunctionCall) (execShellArgs, error) {
	var args execShellArgs
	cmdArg := call.Argument(0)
	if cmdArg == nil || goja.IsUndefined(cmdArg) || goja.IsNull(cmdArg) {
		return args, errors.New("shell: cmd argument required")
	}

	opts := optsAsMap(call)
	args.timeout = optMillis(opts, "timeout", 30*time.Second)
	args.cwd = optString(opts, "cwd", "")
	args.stdin = optString(opts, "stdin", "")
	args.usePTY = optBool(opts, "pty", false)
	args.env = buildEnv(opts)

	argv, err := buildArgv(cmdArg)
	if err != nil {
		return args, err
	}
	args.argv = argv

	pane, err := resolvePane(call.Argument(1))
	if err != nil {
		return args, err
	}
	args.pane = pane
	return args, nil
}

func execShell(ctx context.Context, args execShellArgs) (map[string]any, error) {
	stdin, usePTY, pane := args.stdin, args.usePTY, args.pane

	runCtx, cancel := context.WithTimeout(ctx, args.timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, args.argv[0], args.argv[1:]...) //nolint:gosec // user-supplied argv is intentional
	if args.cwd != "" {
		cmd.Dir = args.cwd
	}
	if args.env != nil {
		cmd.Env = args.env
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	start := time.Now()
	var runErr error

	if usePTY {
		// PTY path: the child runs under a pseudo-terminal so it believes it
		// is a terminal and emits color/progress. startPTY starts the process
		// (it owns the child's stdio), so we must not set cmd.Std* or Run().
		master, perr := startPTY(cmd)
		switch {
		case errors.Is(perr, errPTYUnsupported):
			usePTY = false // Windows: fall through to the pipe path below.
		case perr != nil:
			return nil, fmt.Errorf("shell: pty: %w", perr)
		default:
			// Destination: a pane (renders ANSI) or the capture buffer. A PTY
			// merges stdout+stderr onto one stream, so stderrBuf stays empty.
			var dst io.Writer = &stdoutBuf
			if pane != nil {
				dst = pane.AsWriter()
			}
			if stdin != "" {
				// stdin is written to the master (the child reads its tty);
				// the master is not closed, so stdin-EOF is not signalled.
				go func() { _, _ = io.WriteString(master, stdin) }()
			}
			done := make(chan struct{})
			go func() {
				_, _ = io.Copy(dst, master) // ends on EOF/EIO when the child exits
				close(done)
			}()
			runErr = cmd.Wait()
			<-done
			_ = master.Close()
		}
	}

	if !usePTY {
		// Pipe path (also the Windows pty fallback): original behaviour.
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}
		if pane != nil {
			// Stream stdout+stderr live into the pane. The result's
			// stdout/stderr strings stay empty in that mode (data was
			// streamed, not captured) — documented in MANUAL.md.
			w := pane.AsWriter()
			cmd.Stdout = w
			cmd.Stderr = w
		} else {
			cmd.Stdout = &stdoutBuf
			cmd.Stderr = &stderrBuf
		}
		configureProcessTermination(cmd)
		runErr = cmd.Run()
	}

	durationMs := time.Since(start).Milliseconds()

	exitCode := 0
	success := true
	if runErr != nil {
		// Context deadline / cancel are qualitatively different from a
		// non-zero exit — the subprocess never got to choose its own exit
		// code. Surface those as Go errors / JS throws. Check ctx first
		// because CommandContext wraps the kill in an *exec.ExitError.
		if ctxErr := runCtx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("shell: %w", ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
			success = false
		} else {
			return nil, fmt.Errorf("shell: %w", runErr)
		}
	}

	return map[string]any{
		"stdout":     stdoutBuf.String(),
		"stderr":     stderrBuf.String(),
		"exitCode":   exitCode,
		"success":    success,
		"durationMs": durationMs,
	}, nil
}

// resolvePane extracts the pane handle from the raw opts goja.Value. The
// "pane" key may hold:
//   - A Pane JS object (returned by tui.pane(name)): carries the Go-side
//     tui.Pane under the non-enumerable "__sercon_pane__" property, accessed
//     via goja.Object.Get so non-enumerable entries are reachable.
//   - A plain string: interpreted as a pane name to look up on the active TUI
//     controller (set by tui.layout in the current Run).
//
// Returns (nil, nil) when no "pane" key is present or the opts arg is absent.
func resolvePane(optsArg goja.Value) (tui.Pane, error) {
	if optsArg == nil || goja.IsUndefined(optsArg) || goja.IsNull(optsArg) {
		return nil, nil
	}
	optsObj, ok := optsArg.(*goja.Object)
	if !ok {
		return nil, nil
	}
	paneVal := optsObj.Get("pane")
	if paneVal == nil || goja.IsUndefined(paneVal) || goja.IsNull(paneVal) {
		return nil, nil
	}
	// String name: look up via the active TUI controller.
	if name, ok := paneVal.Export().(string); ok {
		c := activeTUIController()
		if c == nil {
			return nil, errors.New("shell: pane option set but no tui.layout has been declared")
		}
		h := c.Pane(name)
		if h == nil {
			return nil, fmt.Errorf("shell: unknown pane %q (declared: %v)", name, c.PaneNames())
		}
		return h, nil
	}
	// Pane JS object: the Go handle lives under the non-enumerable property
	// "__sercon_pane__". Access via goja.Object.Get (not Export) so the
	// non-enumerable entry is visible.
	paneObj, ok := paneVal.(*goja.Object)
	if !ok {
		return nil, fmt.Errorf("shell: pane option must be a Pane or string name, got %T", paneVal.Export())
	}
	raw := paneObj.Get("__sercon_pane__")
	if raw == nil || goja.IsUndefined(raw) {
		return nil, errors.New("shell: pane option is not a Pane handle")
	}
	p, ok := raw.Export().(tui.Pane)
	if !ok {
		return nil, errors.New("shell: pane handle is malformed")
	}
	return p, nil
}

// buildArgv converts the JS cmd argument into a Go []string. String input
// gets wrapped in the host's command shell; array input passes through.
func buildArgv(cmdArg goja.Value) ([]string, error) {
	switch v := cmdArg.Export().(type) {
	case string:
		if v == "" {
			return nil, errors.New("shell: cmd string is empty")
		}
		if runtime.GOOS == "windows" {
			return []string{"cmd", "/C", v}, nil
		}
		return []string{"/bin/sh", "-c", v}, nil
	case []any:
		if len(v) == 0 {
			return nil, errors.New("shell: argv array is empty")
		}
		argv := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("shell: argv element %v is not a string", item)
			}
			argv = append(argv, s)
		}
		return argv, nil
	case []string:
		if len(v) == 0 {
			return nil, errors.New("shell: argv array is empty")
		}
		return v, nil
	default:
		return nil, fmt.Errorf("shell: cmd must be string or string[], got %T", v)
	}
}

// buildEnv merges opts.env (if present) on top of the parent process
// environment. Returns nil when opts.env is absent — that signals
// exec.Cmd to inherit os.Environ() implicitly. Returning a slice
// containing only the overrides would *replace* the parent env, which
// is rarely what callers want.
func buildEnv(opts map[string]any) []string {
	if opts == nil {
		return nil
	}
	rawEnv, ok := opts["env"]
	if !ok {
		return nil
	}
	envMap, ok := rawEnv.(map[string]any)
	if !ok || len(envMap) == 0 {
		return nil
	}
	env := os.Environ()
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%v", k, v))
	}
	return env
}
