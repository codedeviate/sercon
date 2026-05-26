package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// execNamespace wires `api.exec.*`. v0.4.17 ships `shell` only; `http` (the
// recon-with-curl-fallback path) lands in v0.4.18. Lives under its own
// namespace because subprocess work has different operational concerns
// than the pure-Go bindings — host binary on PATH, environment, working
// directory.
func execNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"shell": scriptengine.PromisifyAsync(vm, loop, execShell),
	}
}

// execShell runs a subprocess and waits for it to exit. The contract:
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
func execShell(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	cmdArg := call.Argument(0)
	if cmdArg == nil || goja.IsUndefined(cmdArg) || goja.IsNull(cmdArg) {
		return nil, errors.New("shell: cmd argument required")
	}

	opts := optsAsMap(call)
	timeout := optMillis(opts, "timeout", 30*time.Second)
	cwd := optString(opts, "cwd", "")
	stdin := optString(opts, "stdin", "")

	argv, err := buildArgv(cmdArg)
	if err != nil {
		return nil, err
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...) //nolint:gosec // user-supplied argv is intentional
	if cwd != "" {
		cmd.Dir = cwd
	}
	if env := buildEnv(opts); env != nil {
		cmd.Env = env
	}
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	runErr := cmd.Run()
	durationMs := time.Since(start).Milliseconds()

	exitCode := 0
	success := true
	if runErr != nil {
		// Context deadline / cancel are qualitatively different from a
		// non-zero exit — the subprocess never got to choose its own
		// exit code. Surface those as Go errors / JS throws so scripts
		// can react accordingly. exec.CommandContext wraps the kill in
		// an *exec.ExitError on some platforms, so check ctx.Err first.
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
