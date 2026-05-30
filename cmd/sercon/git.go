package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// gitNamespace wires `services.git.*`. The wrapper shells out to the host
// `git` binary (no go-git dep) so behaviour matches whatever the user
// has installed — that parity matters more than a self-contained pure-Go
// implementation here. Every binding accepts an `opts.cwd` so a single
// engine can work across multiple checkouts.
func gitNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"branch":   scriptengine.PromisifyAsync(vm, loop, gitBranch),
		"isClean":  scriptengine.PromisifyAsync(vm, loop, gitIsClean),
		"revParse": scriptengine.PromisifyAsync(vm, loop, gitRevParse),
		"status":   scriptengine.PromisifyAsync(vm, loop, gitStatus),
		"add":      scriptengine.PromisifyAsync(vm, loop, gitAdd),
		"commit":   scriptengine.PromisifyAsync(vm, loop, gitCommit),
		"log":      scriptengine.PromisifyAsync(vm, loop, gitLog),
		"diffStat": scriptengine.PromisifyAsync(vm, loop, gitDiffStat),
		"runText":  scriptengine.PromisifyAsync(vm, loop, gitRunText),
	}
}

// gitRun spawns `git <args>` in cwd and returns the captured streams.
// Non-zero exit is reported via the integer return so callers can choose
// whether to throw or surface it (runText surfaces it; the typed
// bindings throw via gitRunChecked).
func gitRun(ctx context.Context, cwd string, args ...string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return stdoutBuf.String(), stderrBuf.String(), 0, fmt.Errorf("git: %w", ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		return stdoutBuf.String(), stderrBuf.String(), 0, fmt.Errorf("git: %w", err)
	}
	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}

// gitRunChecked is the strict variant used by typed bindings: any
// non-zero exit becomes a JS throw with stderr in the message. Use
// gitRun + manual handling when zero-vs-nonzero is part of the contract
// (e.g. `git symbolic-ref --short HEAD` exits non-zero on detached
// HEAD, which is a useful signal rather than a failure).
func gitRunChecked(ctx context.Context, cwd string, args ...string) (string, error) {
	stdout, stderr, code, err := gitRun(ctx, cwd, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		if msg != "" {
			return "", fmt.Errorf("git %s: exited %d: %s", args[0], code, msg)
		}
		return "", fmt.Errorf("git %s: exited %d", args[0], code)
	}
	return stdout, nil
}

// readCwdOpt pulls `opts.cwd` from the argument at position `idx`. When
// the arg is missing / undefined / not an object, returns "" so the
// subprocess inherits the engine's working directory.
func readCwdOpt(call goja.FunctionCall, idx int) string {
	if len(call.Arguments) <= idx {
		return ""
	}
	arg := call.Argument(idx)
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return ""
	}
	m, ok := arg.Export().(map[string]any)
	if !ok {
		return ""
	}
	if s, ok := m["cwd"].(string); ok {
		return s
	}
	return ""
}

// optAt is optsAsMap parametrised by argument position.
func optAt(call goja.FunctionCall, idx int) map[string]any {
	if len(call.Arguments) <= idx {
		return nil
	}
	arg := call.Argument(idx)
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return nil
	}
	if m, ok := arg.Export().(map[string]any); ok {
		return m
	}
	return nil
}

// gitBranch reports the current branch (or "" when HEAD is detached)
// plus every local branch name.
func gitBranch(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	cwd := readCwdOpt(call, 0)

	current := ""
	detached := false
	stdout, _, code, err := gitRun(ctx, cwd, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return nil, err
	}
	if code == 0 {
		current = strings.TrimSpace(stdout)
	} else {
		detached = true
	}

	out, err := gitRunChecked(ctx, cwd, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	all := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			all = append(all, line)
		}
	}

	return map[string]any{
		"current":  current,
		"detached": detached,
		"all":      all,
	}, nil
}

// gitIsClean returns true when the working tree has no uncommitted or
// untracked changes (i.e. `git status --porcelain` is empty).
func gitIsClean(ctx context.Context, call goja.FunctionCall) (bool, error) {
	cwd := readCwdOpt(call, 0)
	out, err := gitRunChecked(ctx, cwd, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// gitRevParse returns the full 40-char SHA for the given rev. Invalid
// revs throw (git's own error message is included).
func gitRevParse(ctx context.Context, call goja.FunctionCall) (string, error) {
	rev := strings.TrimSpace(call.Argument(0).String())
	if rev == "" {
		return "", errors.New("git.revParse: rev argument required")
	}
	cwd := readCwdOpt(call, 1)
	out, err := gitRunChecked(ctx, cwd, "rev-parse", rev)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// gitStatus parses `git status --porcelain` v1 output into a structured
// list: `XY <path>` where X is the index status and Y the working-tree
// status. Returns an empty array on a clean tree.
func gitStatus(ctx context.Context, call goja.FunctionCall) ([]map[string]any, error) {
	cwd := readCwdOpt(call, 0)
	out, err := gitRunChecked(ctx, cwd, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	entries := []map[string]any{}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 {
			continue
		}
		entries = append(entries, map[string]any{
			"path":          strings.TrimSpace(line[3:]),
			"indexStatus":   string(line[0]),
			"workingStatus": string(line[1]),
		})
	}
	return entries, nil
}

// gitAdd stages one path or a list of paths. The `--` separator is
// inserted so paths that look like flags (`-foo`) work too.
func gitAdd(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	paths, err := pathsArg(call.Argument(0), "git.add")
	if err != nil {
		return nil, err
	}
	cwd := readCwdOpt(call, 1)
	args := append([]string{"add", "--"}, paths...)
	if _, err := gitRunChecked(ctx, cwd, args...); err != nil {
		return nil, err
	}
	return map[string]any{"paths": paths}, nil
}

// gitCommit creates a new commit with the given message. The current
// HEAD SHA is returned so callers can chain. `allowEmpty:true` toggles
// `--allow-empty` for the rare case where an empty commit is the point
// (release markers, force-pushes after squash-rebases).
func gitCommit(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	msg := call.Argument(0).String()
	if strings.TrimSpace(msg) == "" {
		return nil, errors.New("git.commit: message required")
	}
	opts := optAt(call, 1)
	cwd := optString(opts, "cwd", "")
	allowEmpty := optBool(opts, "allowEmpty", false)

	args := []string{"commit", "-m", msg}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	if _, err := gitRunChecked(ctx, cwd, args...); err != nil {
		return nil, err
	}
	out, err := gitRunChecked(ctx, cwd, "rev-parse", "HEAD")
	if err != nil {
		return nil, err
	}
	return map[string]any{"sha": strings.TrimSpace(out)}, nil
}

// gitLog emits the most recent commits in the given rev range. The
// tab-delimited `--format` string is chosen because none of the fields
// we ask for can legitimately contain a tab — subject lines have
// newlines stripped by git already, and SHAs / timestamps are
// hex / digits.
func gitLog(ctx context.Context, call goja.FunctionCall) ([]map[string]any, error) {
	opts := optAt(call, 0)
	cwd := optString(opts, "cwd", "")
	limit := optInt(opts, "limit", 50)
	revRange := optString(opts, "revRange", "HEAD")
	if limit <= 0 {
		return nil, errors.New("git.log: limit must be positive")
	}

	args := []string{
		"log",
		"-n", strconv.Itoa(limit),
		"--format=%H%x09%h%x09%an%x09%ae%x09%at%x09%s",
		revRange,
	}
	out, err := gitRunChecked(ctx, cwd, args...)
	if err != nil {
		return nil, err
	}
	entries := []map[string]any{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 6)
		if len(parts) < 6 {
			continue
		}
		ts, _ := strconv.ParseInt(parts[4], 10, 64)
		entries = append(entries, map[string]any{
			"sha":       parts[0],
			"shortSha":  parts[1],
			"author":    parts[2],
			"email":     parts[3],
			"timestamp": ts,
			"subject":   parts[5],
		})
	}
	return entries, nil
}

// diffStatRe captures the three counters in `git diff --shortstat`'s
// human-readable summary: " 3 files changed, 17 insertions(+), 5
// deletions(-)". Any of the three may be absent (a pure-add diff has no
// deletions and vice-versa), which is why we sweep the line and pull
// each counter out independently.
var diffStatRe = regexp.MustCompile(`(\d+) (file|insertion|deletion)`)

// gitDiffStat aggregates `git diff --shortstat`'s counters. Default
// revRange is `HEAD~1..HEAD` (the last commit). An empty diff returns
// zero across the board rather than throwing.
func gitDiffStat(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	opts := optAt(call, 0)
	cwd := optString(opts, "cwd", "")
	revRange := optString(opts, "revRange", "HEAD~1..HEAD")

	out, err := gitRunChecked(ctx, cwd, "diff", "--shortstat", revRange)
	if err != nil {
		return nil, err
	}

	files, ins, del := 0, 0, 0
	for _, m := range diffStatRe.FindAllStringSubmatch(out, -1) {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "file":
			files = n
		case "insertion":
			ins = n
		case "deletion":
			del = n
		}
	}
	return map[string]any{
		"files":      files,
		"insertions": ins,
		"deletions":  del,
	}, nil
}

// gitRunText is the escape hatch for git invocations the typed bindings
// don't cover. Returns stdout / stderr / exitCode; non-zero exits do
// NOT throw, since the whole point is to give callers a structured way
// to react to git's various exit codes. Spawn failures and context
// cancellation still throw.
func gitRunText(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	args, err := pathsArg(call.Argument(0), "git.runText")
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		return nil, errors.New("git.runText: args array is empty")
	}
	cwd := readCwdOpt(call, 1)
	stdout, stderr, exitCode, err := gitRun(ctx, cwd, args...)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"stdout":   stdout,
		"stderr":   stderr,
		"exitCode": exitCode,
	}, nil
}

// pathsArg converts a JS string-or-string[] into a Go []string. Used by
// git.add (path list) and git.runText (argv). The label parameter is
// folded into error messages so the caller's name reaches the script.
func pathsArg(arg goja.Value, label string) ([]string, error) {
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return nil, fmt.Errorf("%s: argument required", label)
	}
	switch v := arg.Export().(type) {
	case string:
		if v == "" {
			return nil, fmt.Errorf("%s: string is empty", label)
		}
		return []string{v}, nil
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s: array element %v not a string", label, item)
			}
			out = append(out, s)
		}
		return out, nil
	case []string:
		return v, nil
	default:
		return nil, fmt.Errorf("%s: argument must be string or string[], got %T", label, v)
	}
}
