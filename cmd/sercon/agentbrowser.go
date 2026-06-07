package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// agentBrowserBin is the CLI this namespace bridges to.
const agentBrowserBin = "agent-browser"

// abDefaultCallTimeout bounds a single agent-browser subprocess call so a
// wedged daemon throws instead of hanging the script. Override per handle via
// launch({ timeout: <ms> }); 0 disables. abCloseTimeout bounds session close
// (incl. the Run-end cleanup drain) so teardown can never hang.
const (
	abDefaultCallTimeout = 30 * time.Second
	abCloseTimeout       = 10 * time.Second
)

// abGlobalFlags maps a launch-option key to its CLI flag. String options
// emit `--flag value`; bool options emit a bare `--flag` only when true.
// Only keys here are honoured; unknown keys are ignored.
var abGlobalFlags = []struct {
	key  string
	flag string
	bool bool
}{
	{"headed", "--headed", true},
	{"ignoreHttpsErrors", "--ignore-https-errors", true},
	{"profile", "--profile", false},
	{"proxy", "--proxy", false},
	{"userAgent", "--user-agent", false},
	{"device", "--device", false},
	{"colorScheme", "--color-scheme", false},
	{"engine", "--engine", false},
	{"executablePath", "--executable-path", false},
	{"enable", "--enable", false},
	{"args", "--args", false},
}

// mergeLaunchOpts returns a new map with defaults overlaid by opts (opts
// wins on key conflicts). Neither input is mutated.
func mergeLaunchOpts(defaults, opts map[string]any) map[string]any {
	out := make(map[string]any, len(defaults)+len(opts))
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range opts {
		out[k] = v
	}
	return out
}

// buildGlobalArgs turns a launch-options map into ordered CLI global flags.
// Order follows abGlobalFlags so output is deterministic for tests.
func buildGlobalArgs(opts map[string]any) []string {
	var out []string
	for _, g := range abGlobalFlags {
		v, ok := opts[g.key]
		if !ok {
			continue
		}
		if g.bool {
			if b, _ := v.(bool); b {
				out = append(out, g.flag)
			}
			continue
		}
		if s := fmt.Sprintf("%v", v); s != "" {
			out = append(out, g.flag, s)
		}
	}
	return out
}

// abRun spawns `agent-browser <global> --json --session <name> <args...>`
// and returns captured streams + exit code. ctx is the engine's per-Run
// context. When timeout > 0, the call is additionally bounded by that
// duration; a deadline-exceeded error produces a clear "timed out" message
// so a wedged daemon is immediately diagnosable.
func abRun(ctx context.Context, session string, global []string, timeout time.Duration, args ...string) (string, string, int, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	full := make([]string, 0, len(global)+len(args)+4)
	full = append(full, global...)
	full = append(full, "--json")
	if session != "" {
		full = append(full, "--session", session)
	}
	full = append(full, args...)

	cmd := exec.CommandContext(ctx, agentBrowserBin, full...)
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err := cmd.Run()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			if errors.Is(ctxErr, context.DeadlineExceeded) {
				return stdoutBuf.String(), stderrBuf.String(), 0, fmt.Errorf("agent-browser %s: timed out after %s (daemon may be unresponsive)", firstArg(args), timeout)
			}
			return stdoutBuf.String(), stderrBuf.String(), 0, fmt.Errorf("agent-browser: %w", ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return stdoutBuf.String(), stderrBuf.String(), exitErr.ExitCode(), nil
		}
		return stdoutBuf.String(), stderrBuf.String(), 0, fmt.Errorf("agent-browser: %w", err)
	}
	return stdoutBuf.String(), stderrBuf.String(), 0, nil
}

// abRunChecked is the strict variant: a non-zero exit becomes an error
// carrying stderr (which surfaces as a JS throw). stdout is returned raw
// for the caller to parse. timeout is forwarded to abRun; 0 means no
// per-call deadline.
func abRunChecked(ctx context.Context, session string, global []string, timeout time.Duration, args ...string) (string, error) {
	if !abAvailable() {
		return "", errors.New("agent-browser CLI not found on PATH; install it to use services.agentBrowser")
	}
	stdout, stderr, code, err := abRun(ctx, session, global, timeout, args...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		if msg != "" {
			return "", fmt.Errorf("agent-browser %s: exited %d: %s", firstArg(args), code, msg)
		}
		return "", fmt.Errorf("agent-browser %s: exited %d", firstArg(args), code)
	}
	return stdout, nil
}

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "?"
}

// callTimeout reads opts["timeout"] (milliseconds) into a Duration. Missing
// key → abDefaultCallTimeout; explicit 0 → 0 (disabled).
func callTimeout(opts map[string]any) time.Duration {
	v, ok := opts["timeout"]
	if !ok {
		return abDefaultCallTimeout
	}
	ms := numToInt(v) // numToInt is in agentbrowser_inspect.go
	if ms <= 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

// abAvailable reports whether the agent-browser CLI is on PATH.
func abAvailable() bool {
	_, err := exec.LookPath(agentBrowserBin)
	return err == nil
}

// abVersion returns the CLI version string.
func abVersion(ctx context.Context, _ goja.FunctionCall) (string, error) {
	if !abAvailable() {
		return "", errors.New("agent-browser CLI not found on PATH; install it to use services.agentBrowser")
	}
	cmd := exec.CommandContext(ctx, agentBrowserBin, "--version")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("agent-browser --version: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// parseJSON decodes an agent-browser --json blob into an *Ordered, preserving
// native source key order via DecodeOrderedJSON. Non-object JSON (array,
// string, number) and plain text are wrapped under a "result" key. Empty
// output decodes to an empty object.
func parseJSON(raw string) (*scriptengine.Ordered, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return scriptengine.NewOrdered(), nil
	}
	v, err := scriptengine.DecodeOrderedJSON([]byte(raw))
	if err != nil {
		return scriptengine.NewOrdered().Set("result", raw), nil
	}
	if o, ok := v.(*scriptengine.Ordered); ok {
		return o, nil
	}
	return scriptengine.NewOrdered().Set("result", v), nil
}

// agentBrowserNamespace builds the services.agentBrowser member map. It
// allocates a per-Run registry and registers a best-effort cleanup that
// closes any session the script left open.
func agentBrowserNamespace(vm *goja.Runtime, loop *eventloop.EventLoop, e *scriptengine.Engine) map[string]any {
	reg := &abRegistry{sessions: map[string]struct{}{}, defaults: map[string]any{}}
	// Only register cleanup for a real Run; vm == nil during d.ts introspection.
	if vm != nil {
		e.AddRunCleanup(reg.closeAll)
	}

	return map[string]any{
		"available": abAvailable(),
		// Keep as AsyncBinding (not .Func) so the d.ts emitter renders Promise<string>.
		"version": scriptengine.PromisifyAsync(vm, loop, abVersion),
		"launch": func(call goja.FunctionCall) goja.Value {
			h := reg.newHandle(call.Argument(0), vm)
			return vm.ToValue(h.jsObject(vm, loop))
		},
		"defaultOptions": func(call goja.FunctionCall) goja.Value {
			reg.mu.Lock()
			cp := make(map[string]any, len(reg.defaults))
			for k, v := range reg.defaults {
				cp[k] = v
			}
			reg.mu.Unlock()
			return vm.ToValue(cp)
		},
		"setDefaultOptions": func(call goja.FunctionCall) goja.Value {
			m := map[string]any{}
			if obj, ok := call.Argument(0).Export().(map[string]any); ok {
				m = obj
			}
			reg.mu.Lock()
			reg.defaults = m
			reg.mu.Unlock()
			return goja.Undefined()
		},
		"clearDefaultOptions": func(call goja.FunctionCall) goja.Value {
			reg.mu.Lock()
			reg.defaults = map[string]any{}
			reg.mu.Unlock()
			return goja.Undefined()
		},
		"screenshot": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			url := strArg(call, 0)
			return reg.withEphemeral(ctx, url, func(h *abHandle) (any, error) {
				return h.screenshot(ctx, shiftCall(call, 1))
			})
		}).Func,
		"pdf": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			url := strArg(call, 0)
			return reg.withEphemeral(ctx, url, func(h *abHandle) (any, error) {
				return h.pdf(ctx, shiftCall(call, 1))
			})
		}).Func,
		"snapshot": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			url := strArg(call, 0)
			return reg.withEphemeral(ctx, url, func(h *abHandle) (any, error) {
				return h.snapshot(ctx, shiftCall(call, 1))
			})
		}).Func,
		"eval": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			url := strArg(call, 0)
			return reg.withEphemeral(ctx, url, func(h *abHandle) (any, error) {
				return h.evalJS(ctx, shiftCall(call, 1))
			})
		}).Func,
	}
}

// abRegistry tracks sessions launched this Run for best-effort cleanup.
type abRegistry struct {
	mu       sync.Mutex
	sessions map[string]struct{}
	counter  int
	defaults map[string]any // namespace-level launch defaults (per Run)
}

// allocSession returns the session name to use: the caller's explicit name
// when non-empty, else a unique auto id. The chosen name is recorded so
// closeAll can reach it if the script never closes the handle.
func (r *abRegistry) allocSession(explicit string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := explicit
	if name == "" {
		r.counter++
		name = fmt.Sprintf("sercon-%d-%d", os.Getpid(), r.counter)
	}
	r.sessions[name] = struct{}{}
	return name
}

// forget drops a session from the cleanup set (called on explicit close).
func (r *abRegistry) forget(name string) {
	r.mu.Lock()
	delete(r.sessions, name)
	r.mu.Unlock()
}

// closeAll fires `agent-browser --session <name> close` best-effort for
// every still-open session. Registered via Engine.AddRunCleanup so it runs
// after loop.Run returns on every exit path (normal, error, cancel, timeout).
func (r *abRegistry) closeAll() {
	if !abAvailable() {
		return
	}
	r.mu.Lock()
	names := make([]string, 0, len(r.sessions))
	for n := range r.sessions {
		names = append(names, n)
	}
	r.sessions = map[string]struct{}{}
	r.mu.Unlock()
	for _, n := range names {
		_, _, _, _ = abRun(context.Background(), n, nil, abCloseTimeout, "close")
	}
}

// newHandle builds a handle from the launch options argument. Per-call opts
// are merged over the registry's defaults so defaults flow through unless
// the caller overrides them.
func (r *abRegistry) newHandle(optsArg goja.Value, vm *goja.Runtime) *abHandle {
	opts := map[string]any{}
	if optsArg != nil && !goja.IsUndefined(optsArg) && !goja.IsNull(optsArg) {
		if m, ok := optsArg.Export().(map[string]any); ok {
			opts = m
		}
	}
	r.mu.Lock()
	merged := mergeLaunchOpts(r.defaults, opts)
	r.mu.Unlock()
	explicit, _ := merged["session"].(string)
	return &abHandle{
		session: r.allocSession(explicit),
		global:  buildGlobalArgs(merged),
		reg:     r,
		timeout: callTimeout(merged),
	}
}

// withEphemeral allocates a throwaway session, opens url, runs fn against a
// transient handle, and always closes the session afterward (best-effort).
// Used by the flat one-shot shortcuts. Errors (and skips allocation) when
// url is empty.
func (r *abRegistry) withEphemeral(ctx context.Context, url string, fn func(h *abHandle) (any, error)) (any, error) {
	if url == "" {
		return nil, errors.New("agentBrowser: url is required")
	}
	r.mu.Lock()
	merged := mergeLaunchOpts(r.defaults, map[string]any{})
	r.mu.Unlock()
	h := &abHandle{session: r.allocSession(""), global: buildGlobalArgs(merged), reg: r, timeout: callTimeout(r.defaults)}
	defer func() { _, _ = h.close(ctx, goja.FunctionCall{}) }()
	if _, err := abRunChecked(ctx, h.session, h.global, h.timeout, "open", url); err != nil {
		return nil, err
	}
	return fn(h)
}

// abHandle is one browser session. global holds the pre-built --flag args
// from launch opts (merged over defaults); they are prepended to every call.
// timeout is the effective per-call deadline (0 = no timeout).
type abHandle struct {
	session string
	global  []string
	reg     *abRegistry
	timeout time.Duration
	closed  atomic.Bool
}

// p wraps an async method as a bare goja callback for use inside jsObject.
// Handle methods intentionally use .Func here: jsObject returns a raw map
// handed to vm.ToValue (not a registered namespace), so AsyncBindings are
// never unwrapped by unwrapAsyncBindings and must be the bare function value.
func (h *abHandle) p(vm *goja.Runtime, loop *eventloop.EventLoop, work func(context.Context, goja.FunctionCall) (any, error)) func(goja.FunctionCall) goja.Value {
	return scriptengine.PromisifyAsync(vm, loop, work).Func
}

// jsObject returns the goja-facing handle object. Method groups are added
// across tasks; Phase 1 wires navigation, interaction, inspection, locator,
// and close. session is exposed read-only for diagnostics.
func (h *abHandle) jsObject(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	obj := map[string]any{
		"session": h.session,
		"close":   h.p(vm, loop, h.close),
	}
	h.addNav(obj, vm, loop)      // Task 3
	h.addInteract(obj, vm, loop) // Task 4
	h.addInspect(obj, vm, loop)  // Task 5
	h.addLocator(obj, vm, loop)  // Task 6
	h.addSettings(obj, vm, loop) // Phase 2
	h.addCapture(obj, vm, loop)  // Phase 2
	h.addNetwork(obj, vm, loop)  // Phase 3
	h.addStorage(obj, vm, loop)  // Phase 3
	h.addTabs(obj, vm, loop)     // Phase 3
	h.addDiff(obj, vm, loop)     // Phase 3
	h.addDebug(obj, vm, loop)    // Phase 4
	h.addReact(obj, vm, loop)    // Phase 4
	h.addAdvanced(obj, vm, loop) // Phase 4
	return obj
}

// runJSON is the shared "verb + operands" runner for Phase-3 handle method
// groups: it guards requireOpen, runs the args via abRunChecked, and returns
// the parsed JSON envelope. Phases 1/2 predate this and keep their own
// runNav/runVerb/runSet helpers.
func (h *abHandle) runJSON(ctx context.Context, args ...string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, args...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// requireOpen returns an error if the handle was already closed.
func (h *abHandle) requireOpen() error {
	if h.closed.Load() {
		return errors.New("agent-browser: session already closed")
	}
	return nil
}

// close ends the browser session. Idempotent: a second close is a no-op.
// Always uses abCloseTimeout regardless of h.timeout so teardown is bounded
// even when the handle disabled per-call timeouts.
func (h *abHandle) close(ctx context.Context, _ goja.FunctionCall) (any, error) {
	if h.closed.Swap(true) {
		return scriptengine.NewOrdered(), nil
	}
	h.reg.forget(h.session)
	_, err := abRunChecked(ctx, h.session, h.global, abCloseTimeout, "close")
	if err != nil {
		return nil, err
	}
	o := scriptengine.NewOrdered()
	o.Set("closed", true)
	return o, nil
}

