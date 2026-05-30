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

// decodeOrderedObject walks a JSON object from a *json.Decoder, preserving key
// order, and lets the caller rewrite each entry as it is emitted. This is the
// `gh.go` analogue of scriptengine.DecodeOrderedJSON, but with a per-key hook
// so the post-processing the legacy code did via in-place map mutation
// (flattening `author`/`owner` login wrappers, renaming `defaultBranchRef`)
// can be applied while still producing a stable, source-ordered
// *scriptengine.Ordered. goja takes a JS object's key order from Go map
// iteration (randomized per process), so returning a map here would shuffle
// JSON.stringify output run-to-run; an Ordered fixes the order.
//
// `rewrite` is consulted for each key, in source order. It receives the key
// and a `consume` interface exposing two mutually-exclusive ways to read the
// value:
//
//   - consume.Ordered() decodes it as a stable *Ordered (the normal path,
//     preserving nested key order);
//   - consume.Plain() decodes it as a throwaway plain value via json.Decode
//     (objects → map[string]any) — used for wrapper fields we only peek a
//     scalar out of, whose own key order is irrelevant since we discard them.
//
// Exactly one must be called per key; the hook then returns the key→value pair
// to emit plus an `emit` flag (emit=false drops the entry). If the hook calls
// neither, the value is drained with the ordered decoder to keep the token
// stream balanced. A nil rewrite emits every key with its ordered value
// unchanged. The decoder must be positioned just past the opening `{`.
func decodeOrderedObject(dec *json.Decoder, rewrite func(key string, c *valueConsumer) (string, any, bool, error)) (*scriptengine.Ordered, error) {
	o := scriptengine.NewOrdered()
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := keyTok.(string)

		if rewrite == nil {
			val, err := decodeOrderedAny(dec)
			if err != nil {
				return nil, err
			}
			o.Set(key, val)
			continue
		}

		c := &valueConsumer{dec: dec}
		outKey, outVal, emit, err := rewrite(key, c)
		if err != nil {
			return nil, err
		}
		if cErr := c.drain(); cErr != nil {
			return nil, cErr
		}
		if !emit {
			continue
		}
		o.Set(outKey, outVal)
	}
	if _, err := dec.Token(); err != nil { // consume '}'
		return nil, err
	}
	return o, nil
}

// valueConsumer reads the value following a JSON object key exactly once,
// either as a stable *Ordered or as a throwaway plain value. It tracks whether
// it was used so decodeOrderedObject can drain an unconsumed value and keep the
// token stream balanced.
type valueConsumer struct {
	dec  *json.Decoder
	used bool
}

// Ordered consumes the value preserving nested object key order.
func (c *valueConsumer) Ordered() (any, error) {
	c.used = true
	return decodeOrderedAny(c.dec)
}

// Plain consumes the value as a plain Go value (objects → map[string]any).
// Use it for wrappers whose internal order is irrelevant because the wrapper
// is discarded after peeking one field.
func (c *valueConsumer) Plain() (any, error) {
	c.used = true
	var v any
	if err := c.dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

// drain reads-and-discards the value if the hook consumed neither way.
func (c *valueConsumer) drain() error {
	if c.used {
		return nil
	}
	_, err := decodeOrderedAny(c.dec)
	return err
}

// decodeOrderedAny decodes the next JSON value, preserving object key order
// (objects → *scriptengine.Ordered, arrays → []any, primitives as-is). It is a
// local mirror of scriptengine's unexported decoder, needed because that
// package exposes only the top-level DecodeOrderedJSON entry point and we want
// to drive the token stream ourselves (to intercept keys).
func decodeOrderedAny(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			return decodeOrderedObject(dec, nil)
		case '[':
			arr := []any{}
			for dec.More() {
				v, err := decodeOrderedAny(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, v)
			}
			if _, err := dec.Token(); err != nil { // consume ']'
				return nil, err
			}
			return arr, nil
		}
	}
	return tok, nil
}

// loginOf extracts the `login` string from a wrapper value that was decoded as
// a plain map[string]any (gh wraps every user-shaped field this way). It
// returns ("", false) for a non-object or a missing/non-string login. The
// wrapper is discarded after this peek, so decoding it as an unordered map is
// fine.
func loginOf(val any) (string, bool) {
	m, ok := val.(map[string]any)
	if !ok {
		return "", false
	}
	login, ok := m["login"].(string)
	return login, ok
}

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
func ghPrList(ctx context.Context, call goja.FunctionCall) ([]*scriptengine.Ordered, error) {
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

// parsePRListJSON decodes the `--json` blob preserving each PR object's key
// order (so JSON.stringify output is stable run-to-run) and flattens the
// `author: { login, ... }` wrapper into a bare login string. Pulled out of
// ghPrList so it's testable without spawning gh.
//
// The top level is a JSON array of PR objects; each object's keys appear in
// the order `gh` emitted them (which follows prListFields). We rewrite only
// the `author` key (flattened to its login scalar, kept in place); every other
// key passes through unchanged in source order.
func parsePRListJSON(raw []byte) ([]*scriptengine.Ordered, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("gh.prList: parse JSON: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("gh.prList: parse JSON: expected array, got %v", tok)
	}
	out := []*scriptengine.Ordered{}
	for dec.More() {
		objTok, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("gh.prList: parse JSON: %w", err)
		}
		if d, ok := objTok.(json.Delim); !ok || d != '{' {
			return nil, fmt.Errorf("gh.prList: parse JSON: expected object, got %v", objTok)
		}
		pr, err := decodeOrderedObject(dec, flattenLoginRewrite("author"))
		if err != nil {
			return nil, fmt.Errorf("gh.prList: parse JSON: %w", err)
		}
		out = append(out, pr)
	}
	if _, err := dec.Token(); err != nil { // consume ']'
		return nil, fmt.Errorf("gh.prList: parse JSON: %w", err)
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
func ghRepoView(ctx context.Context, call goja.FunctionCall) (*scriptengine.Ordered, error) {
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

// parseRepoViewJSON decodes `gh repo view --json` output preserving key order
// (so JSON.stringify is stable run-to-run) and flattens the two object-wrapper
// fields scripts almost always want scalar versions of: `owner.login` →
// `owner` (kept in place), and `defaultBranchRef.name` → `defaultBranch`. The
// rename keeps `defaultBranch` in the position `defaultBranchRef` occupied;
// `defaultBranch` is always emitted (callers don't undefined-check), defaulting
// to "" if the ref is absent, null, or nameless — mirroring the legacy map
// logic. Pulled out for testability.
func parseRepoViewJSON(raw []byte) (*scriptengine.Ordered, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("gh.repoView: parse JSON: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, fmt.Errorf("gh.repoView: parse JSON: expected object, got %v", tok)
	}

	sawDefaultBranch := false
	data, err := decodeOrderedObject(dec, func(key string, c *valueConsumer) (string, any, bool, error) {
		switch key {
		case "owner":
			val, err := c.Plain()
			if err != nil {
				return "", nil, false, err
			}
			if login, ok := loginOf(val); ok {
				return "owner", login, true, nil
			}
			// Not a recognizable login wrapper: keep the original value.
			// (Mirrors flattenLogin leaving non-wrapper values untouched.)
			return "owner", normalizePlain(val), true, nil
		case "defaultBranchRef":
			// present-and-populated, present-as-null, or nameless: in all
			// cases emit `defaultBranch` here; "" unless we find a name.
			val, err := c.Plain()
			if err != nil {
				return "", nil, false, err
			}
			sawDefaultBranch = true
			branch := ""
			if m, ok := val.(map[string]any); ok {
				if name, ok := m["name"].(string); ok {
					branch = name
				}
			}
			return "defaultBranch", branch, true, nil
		default:
			val, err := c.Ordered()
			if err != nil {
				return "", nil, false, err
			}
			return key, val, true, nil
		}
	})
	if err != nil {
		return nil, fmt.Errorf("gh.repoView: parse JSON: %w", err)
	}
	if !sawDefaultBranch {
		data.Set("defaultBranch", "")
	}
	return data, nil
}

// flattenLoginRewrite returns a rewrite hook for decodeOrderedObject that
// flattens `key: { login, … }` to `key: "<login>"`, keeping the key in place;
// gh wraps every user-shaped field this way and scripts almost always want
// just the login. Any non-wrapper value passes through unchanged (decoded as a
// stable *Ordered so nested order is preserved). All other keys pass through
// in source order with their ordered value.
func flattenLoginRewrite(loginKey string) func(string, *valueConsumer) (string, any, bool, error) {
	return func(key string, c *valueConsumer) (string, any, bool, error) {
		if key == loginKey {
			val, err := c.Plain()
			if err != nil {
				return "", nil, false, err
			}
			if login, ok := loginOf(val); ok {
				return key, login, true, nil
			}
			return key, normalizePlain(val), true, nil
		}
		val, err := c.Ordered()
		if err != nil {
			return "", nil, false, err
		}
		return key, val, true, nil
	}
}

// normalizePlain converts a value decoded via valueConsumer.Plain (which uses
// encoding/json and therefore yields map[string]any for objects) into the
// ordered representation the rest of the result uses. It only matters on the
// rare path where a `login`-wrapper field isn't actually a recognizable login
// wrapper, so we re-encode and decode it through the ordered path to keep
// nested key order stable. Primitives and arrays-of-primitives pass through.
func normalizePlain(v any) any {
	switch v.(type) {
	case map[string]any, []any:
		b, err := json.Marshal(v)
		if err != nil {
			return v
		}
		if ord, err := scriptengine.DecodeOrderedJSON(b); err == nil {
			return ord
		}
		return v
	default:
		return v
	}
}
