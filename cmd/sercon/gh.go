package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// ghNamespace wires `services.gh.*` — a thin wrapper around the GitHub CLI
// (`gh`). Everything goes through `gh --json` so we get structured
// output instead of parsing human-readable text. The wrapper respects
// whatever authentication state `gh auth` is already in; we don't try
// to swap accounts or manage tokens.
func ghNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"authStatus": scriptengine.PromisifyAsync(vm, loop, ghAuthStatus),
		"prList":     scriptengine.PromisifyAsync(vm, loop, ghPrList),
		"repoView":   scriptengine.PromisifyAsync(vm, loop, ghRepoView),
	}
}

// ghRun spawns `gh <args>` in cwd and returns the captured streams. The
// caller decides whether non-zero exit is a throw or a signal (authStatus
// treats it as a signal; the others throw).
func ghRun(ctx context.Context, cwd string, args ...string) (string, string, int, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", "", 0, fmt.Errorf("gh not on PATH: %w", err)
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	if err := cmd.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return stdoutBuf.String(), stderrBuf.String(), 0, fmt.Errorf("gh: %w", ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		return stdoutBuf.String(), stderrBuf.String(), 0, fmt.Errorf("gh: %w", err)
	}
	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}

// ghAuthStatusResult is the resolved value of services.gh.authStatus. It's a
// json-tagged struct (not a map[string]any) so the JS object's key order is
// stable — goja enumerates struct fields in declaration order, whereas a Go
// map shuffles JSON.stringify output run-to-run. See tcpProbeResult in
// probe.go for the rationale. Note `authenticated` deliberately omits
// `omitempty`: a false value must still appear in the object.
type ghAuthStatusResult struct {
	Authenticated bool   `json:"authenticated"`
	User          string `json:"user"`
	Raw           string `json:"raw"`
}

// ghAuthStatus probes whether `gh` is installed *and* authenticated.
// We exercise `gh api user --jq .login` rather than `gh auth status`
// because the former emits a machine-friendly result (just the login on
// success, a clear error on failure) instead of the multi-line human
// report the latter produces.
//
// Failure modes are data, not throws: a missing gh binary or an
// unauthenticated session both resolve to `{ authenticated: false, …}`
// so scripts can branch without try/catch. Context cancellation still
// throws.
func ghAuthStatus(ctx context.Context, call goja.FunctionCall) (ghAuthStatusResult, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return ghAuthStatusResult{
			Authenticated: false,
			User:          "",
			Raw:           "gh not on PATH",
		}, nil
	}
	stdout, stderr, code, err := ghRun(ctx, "", "api", "user", "--jq", ".login")
	if err != nil {
		// Only context cancellation reaches here; exec.ExitError was
		// already absorbed into `code` by ghRun.
		return ghAuthStatusResult{}, err
	}
	login := strings.TrimSpace(stdout)
	if code == 0 && login != "" {
		return ghAuthStatusResult{
			Authenticated: true,
			User:          login,
			Raw:           login,
		}, nil
	}
	raw := strings.TrimSpace(stderr)
	if raw == "" {
		raw = strings.TrimSpace(stdout)
	}
	return ghAuthStatusResult{
		Authenticated: false,
		User:          "",
		Raw:           raw,
	}, nil
}

// prListFields is the column set we ask `gh pr list --json` for. Kept as
// a package-level string so the test that validates field naming can
// pin it.
const prListFields = "number,title,state,author,headRefName,baseRefName,url,createdAt,updatedAt"

// ghPrList returns recent pull requests on the repo identified by the
// process's working directory (or `opts.cwd`). `gh` does the auth and
// repo-detection; we just shape the JSON.
func ghPrList(ctx context.Context, call goja.FunctionCall) ([]map[string]any, error) {
	opts := optAt(call, 0)
	cwd := optString(opts, "cwd", "")
	state := optString(opts, "state", "open")
	limit := optInt(opts, "limit", 30)
	author := optString(opts, "author", "")
	if limit <= 0 {
		return nil, errors.New("gh.prList: limit must be positive")
	}

	args := []string{
		"pr", "list",
		"--state", state,
		"--limit", strconv.Itoa(limit),
		"--json", prListFields,
	}
	if author != "" {
		args = append(args, "--author", author)
	}

	stdout, stderr, code, err := ghRun(ctx, cwd, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		return nil, fmt.Errorf("gh.prList: gh exited %d: %s", code, msg)
	}
	return parsePRListJSON([]byte(stdout))
}

// parsePRListJSON unmarshals the `--json` blob and flattens the
// `author: { login, ... }` wrapper into a bare login string. Pulled out
// of ghPrList so it's testable without spawning gh.
func parsePRListJSON(raw []byte) ([]map[string]any, error) {
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("gh.prList: parse JSON: %w", err)
	}
	for _, pr := range out {
		flattenLogin(pr, "author")
	}
	return out, nil
}

// repoViewFields covers the columns we expose. defaultBranchRef is
// flattened to `defaultBranch` post-parse; the wire shape needs the
// `Ref` suffix because that's the GraphQL field name `gh` uses.
const repoViewFields = "name,owner,description,url,defaultBranchRef,visibility"

// ghRepoView returns metadata about a repo. With no argument it asks
// gh about the cwd's repo (so it works from inside a checkout); pass a
// "owner/name" string to look up any repo `gh` has access to.
func ghRepoView(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	var repo string
	var opts map[string]any
	if len(call.Arguments) > 0 {
		arg := call.Argument(0)
		if arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			switch v := arg.Export().(type) {
			case string:
				repo = v
				opts = optAt(call, 1)
			case map[string]any:
				// First arg is opts directly — no repo name supplied.
				opts = v
			}
		}
	}
	cwd := optString(opts, "cwd", "")

	args := []string{"repo", "view"}
	if repo != "" {
		args = append(args, repo)
	}
	args = append(args, "--json", repoViewFields)

	stdout, stderr, code, err := ghRun(ctx, cwd, args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		return nil, fmt.Errorf("gh.repoView: gh exited %d: %s", code, msg)
	}
	return parseRepoViewJSON([]byte(stdout))
}

// parseRepoViewJSON unmarshals `gh repo view --json` output and
// flattens the two object-wrapper fields scripts almost always want
// scalar versions of: `owner.login` → `owner`, and `defaultBranchRef.name`
// → `defaultBranch`. Pulled out for testability.
func parseRepoViewJSON(raw []byte) (map[string]any, error) {
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("gh.repoView: parse JSON: %w", err)
	}
	flattenLogin(data, "owner")

	// defaultBranchRef may be present-and-populated, present-as-null
	// (empty repo), or absent. Always set `defaultBranch` so callers
	// don't have to undefined-check.
	if v, ok := data["defaultBranchRef"]; ok {
		if m, ok := v.(map[string]any); ok {
			if name, ok := m["name"].(string); ok {
				data["defaultBranch"] = name
			}
		}
		delete(data, "defaultBranchRef")
	}
	if _, ok := data["defaultBranch"]; !ok {
		data["defaultBranch"] = ""
	}
	return data, nil
}

// flattenLogin rewrites `m[key] = { login, … }` to `m[key] = "<login>"`.
// gh wraps every user-shaped field this way; scripts almost always want
// just the login.
func flattenLogin(m map[string]any, key string) {
	v, ok := m[key].(map[string]any)
	if !ok {
		return
	}
	login, ok := v["login"].(string)
	if !ok {
		return
	}
	m[key] = login
}
