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
	"time"

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
		"branch":   scriptengine.PromisifyAsyncLegacy(vm, loop, gitBranch),
		"isClean":  scriptengine.PromisifyAsyncLegacy(vm, loop, gitIsClean),
		"revParse": scriptengine.PromisifyAsyncLegacy(vm, loop, gitRevParse),
		"status":   scriptengine.PromisifyAsyncLegacy(vm, loop, gitStatus),
		"add":      scriptengine.PromisifyAsyncLegacy(vm, loop, gitAdd),
		"commit":   scriptengine.PromisifyAsyncLegacy(vm, loop, gitCommit),
		"log":      scriptengine.PromisifyAsyncLegacy(vm, loop, gitLog),
		"diffStat": scriptengine.PromisifyAsyncLegacy(vm, loop, gitDiffStat),
		"runText":  scriptengine.PromisifyAsyncLegacy(vm, loop, gitRunText),
	}
}

// gitTimeout bounds every `git` subprocess invocation so a hung git
// process can't hang the Run forever — this matters under `sercon serve`,
// where the Run's own timeout is disabled. Mirrors exec.go's subprocess
// default (optMillis(opts, "timeout", 30*time.Second)); git/gh bindings
// are positional with no opts surface, so this is a fixed default rather
// than a per-call override. Var (not const) so tests can shrink it
// without waiting out the real 30s.
var gitTimeout = 30 * time.Second

// gitRun spawns `git <args>` in cwd and returns the captured streams.
// Non-zero exit is reported via the integer return so callers can choose
// whether to throw or surface it (runText surfaces it; the typed
// bindings throw via gitRunChecked).
func gitRun(ctx context.Context, cwd string, args ...string) (string, string, int, error) {
	runCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "git", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	if err != nil {
		if ctxErr := runCtx.Err(); ctxErr != nil {
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

// gitBranchResult is a json-tagged struct (not map[string]any) so the JS
// object's key order is stable — see tcpProbeResult in probe.go for the
// rationale. Note: `detached` deliberately has no omitempty since a false
// value must still surface to scripts.
type gitBranchResult struct {
	Current  string   `json:"current"`
	Detached bool     `json:"detached"`
	All      []string `json:"all"`
}

// gitBranch reports the current branch (or "" when HEAD is detached)
// plus every local branch name.
func gitBranch(ctx context.Context, call goja.FunctionCall) (gitBranchResult, error) {
	cwd := readCwdOpt(call, 0)

	current := ""
	detached := false
	stdout, _, code, err := gitRun(ctx, cwd, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return gitBranchResult{}, err
	}
	if code == 0 {
		current = strings.TrimSpace(stdout)
	} else {
		detached = true
	}

	out, err := gitRunChecked(ctx, cwd, "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return gitBranchResult{}, err
	}
	all := []string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			all = append(all, line)
		}
	}

	return gitBranchResult{
		Current:  current,
		Detached: detached,
		All:      all,
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

// gitStatusEntry is a json-tagged struct (not map[string]any) so each JS
// status object has stable key order — see tcpProbeResult in probe.go.
type gitStatusEntry struct {
	Path          string `json:"path"`
	IndexStatus   string `json:"indexStatus"`
	WorkingStatus string `json:"workingStatus"`
}

// gitStatus parses `git status --porcelain` v1 output into a structured
// list: `XY <path>` where X is the index status and Y the working-tree
// status. Returns an empty array on a clean tree.
func gitStatus(ctx context.Context, call goja.FunctionCall) ([]gitStatusEntry, error) {
	cwd := readCwdOpt(call, 0)
	out, err := gitRunChecked(ctx, cwd, "status", "--porcelain")
	if err != nil {
		return nil, err
	}
	entries := []gitStatusEntry{}
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 3 {
			continue
		}
		entries = append(entries, gitStatusEntry{
			Path:          strings.TrimSpace(line[3:]),
			IndexStatus:   string(line[0]),
			WorkingStatus: string(line[1]),
		})
	}
	return entries, nil
}

// gitAddResult is a json-tagged struct (not map[string]any) for stable JS
// key order — see tcpProbeResult in probe.go.
type gitAddResult struct {
	Paths []string `json:"paths"`
}

// gitAdd stages one path or a list of paths. The `--` separator is
// inserted so paths that look like flags (`-foo`) work too.
func gitAdd(ctx context.Context, call goja.FunctionCall) (gitAddResult, error) {
	paths, err := pathsArg(call.Argument(0), "git.add")
	if err != nil {
		return gitAddResult{}, err
	}
	cwd := readCwdOpt(call, 1)
	args := append([]string{"add", "--"}, paths...)
	if _, err := gitRunChecked(ctx, cwd, args...); err != nil {
		return gitAddResult{}, err
	}
	return gitAddResult{Paths: paths}, nil
}

// gitCommit creates a new commit with the given message. The current
// HEAD SHA is returned so callers can chain. `allowEmpty:true` toggles
// `--allow-empty` for the rare case where an empty commit is the point
// (release markers, force-pushes after squash-rebases).
// gitCommitResult is a json-tagged struct (not map[string]any) for stable
// JS key order — see tcpProbeResult in probe.go.
type gitCommitResult struct {
	SHA string `json:"sha"`
}

func gitCommit(ctx context.Context, call goja.FunctionCall) (gitCommitResult, error) {
	msg := call.Argument(0).String()
	if strings.TrimSpace(msg) == "" {
		return gitCommitResult{}, errors.New("git.commit: message required")
	}
	opts := optAt(call, 1)
	cwd := optString(opts, "cwd", "")
	allowEmpty := optBool(opts, "allowEmpty", false)

	args := []string{"commit", "-m", msg}
	if allowEmpty {
		args = append(args, "--allow-empty")
	}
	if _, err := gitRunChecked(ctx, cwd, args...); err != nil {
		return gitCommitResult{}, err
	}
	out, err := gitRunChecked(ctx, cwd, "rev-parse", "HEAD")
	if err != nil {
		return gitCommitResult{}, err
	}
	return gitCommitResult{SHA: strings.TrimSpace(out)}, nil
}

// gitLog emits the most recent commits in the given rev range. The
// tab-delimited `--format` string is chosen because none of the fields
// we ask for can legitimately contain a tab — subject lines have
// newlines stripped by git already, and SHAs / timestamps are
// hex / digits.
// gitLogEntry is a json-tagged struct (not map[string]any) so each JS log
// entry has stable key order — see tcpProbeResult in probe.go.
type gitLogEntry struct {
	SHA       string `json:"sha"`
	ShortSHA  string `json:"shortSha"`
	Author    string `json:"author"`
	Email     string `json:"email"`
	Timestamp int64  `json:"timestamp"`
	Subject   string `json:"subject"`
}

func gitLog(ctx context.Context, call goja.FunctionCall) ([]gitLogEntry, error) {
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
	entries := []gitLogEntry{}
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 6)
		if len(parts) < 6 {
			continue
		}
		ts, _ := strconv.ParseInt(parts[4], 10, 64)
		entries = append(entries, gitLogEntry{
			SHA:       parts[0],
			ShortSHA:  parts[1],
			Author:    parts[2],
			Email:     parts[3],
			Timestamp: ts,
			Subject:   parts[5],
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
// gitDiffStatResult is a json-tagged struct (not map[string]any) for stable
// JS key order — see tcpProbeResult in probe.go.
type gitDiffStatResult struct {
	Files      int `json:"files"`
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

func gitDiffStat(ctx context.Context, call goja.FunctionCall) (gitDiffStatResult, error) {
	opts := optAt(call, 0)
	cwd := optString(opts, "cwd", "")
	revRange := optString(opts, "revRange", "HEAD~1..HEAD")

	out, err := gitRunChecked(ctx, cwd, "diff", "--shortstat", revRange)
	if err != nil {
		return gitDiffStatResult{}, err
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
	return gitDiffStatResult{
		Files:      files,
		Insertions: ins,
		Deletions:  del,
	}, nil
}

// gitRunText is the escape hatch for git invocations the typed bindings
// don't cover. Returns stdout / stderr / exitCode; non-zero exits do
// NOT throw, since the whole point is to give callers a structured way
// to react to git's various exit codes. Spawn failures and context
// cancellation still throw.
// gitRunTextResult is a json-tagged struct (not map[string]any) for stable
// JS key order — see tcpProbeResult in probe.go.
type gitRunTextResult struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
}

func gitRunText(ctx context.Context, call goja.FunctionCall) (gitRunTextResult, error) {
	args, err := pathsArg(call.Argument(0), "git.runText")
	if err != nil {
		return gitRunTextResult{}, err
	}
	if len(args) == 0 {
		return gitRunTextResult{}, errors.New("git.runText: args array is empty")
	}
	cwd := readCwdOpt(call, 1)
	stdout, stderr, exitCode, err := gitRun(ctx, cwd, args...)
	if err != nil {
		return gitRunTextResult{}, err
	}
	return gitRunTextResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
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
