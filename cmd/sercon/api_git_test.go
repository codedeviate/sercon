package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

// makeRepo initialises an empty git repo in t.TempDir with a stable
// identity so commits hash deterministically across CI runs. Returns
// the absolute path so tests can pass it as `opts.cwd`.
func makeRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	runs := [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Sercon Test"},
		{"config", "commit.gpgsign", "false"},
	}
	for _, args := range runs {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func writeFile(t *testing.T, dir, name, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runGit drives a goja-flavoured FunctionCall into one of the api.tools.git
// bindings. Positional args go through vm.ToValue so the receiver sees
// the same JS-shaped types it would in production.
func runGit[T any](t *testing.T, fn func(context.Context, goja.FunctionCall) (T, error), args ...any) (T, error) {
	t.Helper()
	vm := goja.New()
	vals := make([]goja.Value, len(args))
	for i, a := range args {
		vals[i] = vm.ToValue(a)
	}
	return fn(context.Background(), goja.FunctionCall{Arguments: vals})
}

// Initial repo state: a single commit on `main`, working tree clean,
// HEAD resolvable, isClean true.
func TestGitBranchAndIsClean_FreshRepo(t *testing.T) {
	dir := makeRepo(t)
	writeFile(t, dir, "README.md", "hi\n")
	mustRun(t, dir, "add", "README.md")
	mustRun(t, dir, "commit", "-m", "initial")

	b, err := runGit(t, gitBranch, map[string]any{"cwd": dir})
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if b["current"].(string) != "main" {
		t.Errorf("current branch: %q", b["current"])
	}
	if b["detached"].(bool) {
		t.Errorf("detached: true on fresh repo")
	}
	if all := b["all"].([]string); len(all) != 1 || all[0] != "main" {
		t.Errorf("all branches: %v", all)
	}

	clean, err := runGit(t, gitIsClean, map[string]any{"cwd": dir})
	if err != nil {
		t.Fatalf("isClean: %v", err)
	}
	if !clean {
		t.Error("isClean: false (expected true on fresh commit)")
	}
}

// revParse returns the 40-char SHA. An invalid rev throws with git's
// own error message in the chain.
func TestGitRevParse(t *testing.T) {
	dir := makeRepo(t)
	writeFile(t, dir, "a.txt", "x\n")
	mustRun(t, dir, "add", "a.txt")
	mustRun(t, dir, "commit", "-m", "a")

	sha, err := runGit(t, gitRevParse, "HEAD", map[string]any{"cwd": dir})
	if err != nil {
		t.Fatalf("revParse HEAD: %v", err)
	}
	if len(sha) != 40 {
		t.Errorf("HEAD sha: %q (want 40 hex chars)", sha)
	}

	if _, err := runGit(t, gitRevParse, "no-such-ref", map[string]any{"cwd": dir}); err == nil {
		t.Error("revParse no-such-ref should throw")
	}
}

// Dirty tree: one modification + one untracked file. isClean must
// flip; status must enumerate both entries.
func TestGitStatus_DirtyTree(t *testing.T) {
	dir := makeRepo(t)
	writeFile(t, dir, "tracked.txt", "v1\n")
	mustRun(t, dir, "add", "tracked.txt")
	mustRun(t, dir, "commit", "-m", "start")
	writeFile(t, dir, "tracked.txt", "v2\n")
	writeFile(t, dir, "new.txt", "fresh\n")

	clean, err := runGit(t, gitIsClean, map[string]any{"cwd": dir})
	if err != nil || clean {
		t.Errorf("isClean: %v / clean=%v", err, clean)
	}

	entries, err := runGit(t, gitStatus, map[string]any{"cwd": dir})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	paths := map[string]map[string]any{}
	for _, e := range entries {
		paths[e["path"].(string)] = e
	}
	if mod := paths["tracked.txt"]; mod == nil || mod["workingStatus"].(string) != "M" {
		t.Errorf("tracked modified entry: %v", mod)
	}
	if neu := paths["new.txt"]; neu == nil {
		t.Error("missing new.txt in status")
	}
}

// add + commit round-trip. Verifies the returned SHA matches the
// current HEAD and that the post-commit tree is clean again.
func TestGitAddAndCommit(t *testing.T) {
	dir := makeRepo(t)
	writeFile(t, dir, "x.txt", "1\n")

	if _, err := runGit(t, gitAdd, []any{"x.txt"}, map[string]any{"cwd": dir}); err != nil {
		t.Fatalf("add: %v", err)
	}
	out, err := runGit(t, gitCommit, "first commit", map[string]any{"cwd": dir})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	sha := out["sha"].(string)
	if len(sha) != 40 {
		t.Errorf("returned sha: %q", sha)
	}
	headSha, err := runGit(t, gitRevParse, "HEAD", map[string]any{"cwd": dir})
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	if sha != headSha {
		t.Errorf("commit sha %s != HEAD %s", sha, headSha)
	}
	clean, _ := runGit(t, gitIsClean, map[string]any{"cwd": dir})
	if !clean {
		t.Error("isClean false after commit")
	}
}

// commit with an empty message throws before spawning git.
func TestGitCommit_EmptyMessageThrows(t *testing.T) {
	dir := makeRepo(t)
	if _, err := runGit(t, gitCommit, "", map[string]any{"cwd": dir}); err == nil {
		t.Fatal("expected error for empty commit message")
	}
}

// log returns the requested number of commits, newest first, with all
// fields populated.
func TestGitLog(t *testing.T) {
	dir := makeRepo(t)
	subjects := []string{"first", "second", "third"}
	for i, s := range subjects {
		writeFile(t, dir, "f.txt", strings.Repeat("x", i+1))
		mustRun(t, dir, "add", "f.txt")
		mustRun(t, dir, "commit", "-m", s)
	}

	out, err := runGit(t, gitLog, map[string]any{"cwd": dir, "limit": 2})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("log len: %d (want 2)", len(out))
	}
	// newest is "third"
	if out[0]["subject"].(string) != "third" {
		t.Errorf("first subject: %q", out[0]["subject"])
	}
	if out[1]["subject"].(string) != "second" {
		t.Errorf("second subject: %q", out[1]["subject"])
	}
	if a := out[0]["author"].(string); a != "Sercon Test" {
		t.Errorf("author: %q", a)
	}
	if ts := out[0]["timestamp"].(int64); ts <= 0 {
		t.Errorf("timestamp: %d", ts)
	}
}

// diffStat against a known two-commit history. We control the file
// content so the counters are predictable.
func TestGitDiffStat(t *testing.T) {
	dir := makeRepo(t)
	writeFile(t, dir, "f.txt", "a\nb\nc\n")
	mustRun(t, dir, "add", "f.txt")
	mustRun(t, dir, "commit", "-m", "base")
	writeFile(t, dir, "f.txt", "a\nb\nc\nd\ne\n")
	mustRun(t, dir, "add", "f.txt")
	mustRun(t, dir, "commit", "-m", "extend")

	out, err := runGit(t, gitDiffStat, map[string]any{"cwd": dir, "revRange": "HEAD~1..HEAD"})
	if err != nil {
		t.Fatalf("diffStat: %v", err)
	}
	if files := out["files"].(int); files != 1 {
		t.Errorf("files: %d", files)
	}
	if ins := out["insertions"].(int); ins != 2 {
		t.Errorf("insertions: %d (want 2)", ins)
	}
	if del := out["deletions"].(int); del != 0 {
		t.Errorf("deletions: %d", del)
	}
}

// runText surfaces non-zero exits as data — does NOT throw. Spawn
// failures and context cancellation still throw.
func TestGitRunText_NonZeroDoesNotThrow(t *testing.T) {
	dir := makeRepo(t)
	out, err := runGit(t, gitRunText, []any{"rev-parse", "no-such-ref"}, map[string]any{"cwd": dir})
	if err != nil {
		t.Fatalf("runText: %v", err)
	}
	if code := out["exitCode"].(int); code == 0 {
		t.Errorf("expected non-zero exit, got %d", code)
	}
	if strings.TrimSpace(out["stderr"].(string)) == "" {
		t.Error("expected stderr to be non-empty")
	}
}

// pathsArg / runText input validation. Empty array, wrong type.
func TestGitRunText_InputValidation(t *testing.T) {
	dir := makeRepo(t)
	if _, err := runGit(t, gitRunText, []any{}, map[string]any{"cwd": dir}); err == nil {
		t.Error("empty argv should error")
	}
	// gojaRunGitTest helper: int isn't a valid argv element
	if _, err := runGit(t, gitRunText, []any{42}, map[string]any{"cwd": dir}); err == nil {
		t.Error("non-string argv element should error")
	}
}

// Detached-HEAD reporting via `branch`. We check out the commit SHA
// directly so symbolic-ref fails and the wrapper reports detached:true.
func TestGitBranch_DetachedHEAD(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("detached-head test relies on POSIX-style git")
	}
	dir := makeRepo(t)
	writeFile(t, dir, "x.txt", "1\n")
	mustRun(t, dir, "add", "x.txt")
	mustRun(t, dir, "commit", "-m", "one")
	sha, err := runGit(t, gitRevParse, "HEAD", map[string]any{"cwd": dir})
	if err != nil {
		t.Fatal(err)
	}
	mustRun(t, dir, "checkout", "--detach", sha)

	b, err := runGit(t, gitBranch, map[string]any{"cwd": dir})
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if !b["detached"].(bool) {
		t.Error("detached: false on detached HEAD")
	}
	if b["current"].(string) != "" {
		t.Errorf("current: %q (want empty)", b["current"])
	}
}

// mustRun runs `git <args>` in dir, t.Fatal on non-zero. Used to set up
// fixtures; not part of the binding surface.
func mustRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}
