package scriptengine

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/dop251/goja_nodejs/require"
)

// reservedRuntimeName is the global name the engine patches `argv` onto
// after every Run. Hosts (the sercon CLI) typically register a `runtime`
// namespace covering log/assert/time/env; the engine then adds `argv`.
// If no host registered it, the engine synthesises a minimal
// `runtime = {argv: ...}` global.
const reservedRuntimeName = "runtime"

// Options configures an Engine.
type Options struct {
	// Timeout is the per-script wall clock limit. Zero disables the timeout.
	Timeout time.Duration
	// ScriptRoot is the base directory used to resolve relative require/import
	// specifiers from the entry script. If empty it defaults to the current
	// working directory at Run time.
	ScriptRoot string
	// DisableConsole turns off the standard "console" module. The console is
	// enabled by default; the negative-sense field keeps the Go zero value
	// aligned with the "default true" behaviour described in the spec.
	DisableConsole bool
	// Verbose, if non-nil, receives engine diagnostic traces — the rewritten
	// entry-script JS (full body) and each module-resolution event. Lines
	// are prefixed with `[sercon] ` so they're easy to grep. Most callers
	// leave this nil; the sercon CLI plugs in os.Stderr behind `-v`.
	Verbose io.Writer
	// ModuleLoader, when non-nil, is consulted for every require/import
	// candidate path BEFORE the filesystem — the hook for embedders that
	// want to serve modules from somewhere other than disk (an in-memory
	// FS, a network source, an embedded bundle). It receives each candidate
	// path goja probes during resolution and returns:
	//
	//	(source, true,  nil) — serve the module from `source` (transpiled
	//	                       when the path ends in .ts / .tsx)
	//	("",     false, nil) — not handled; fall through to the filesystem
	//	("",     false, err) — abort resolution with err
	//
	// Because goja probes several candidate paths per specifier (`./x`,
	// `./x.ts`, `./x/index.ts`, …), a loader typically matches on a
	// suffix or basename rather than an exact path. Returning source for a
	// `.ts` candidate gets it transpiled just like a disk read would.
	ModuleLoader func(candidatePath string) (source string, found bool, err error)
	// ProgramName is argv[0] in the per-script runtime.argv. When empty,
	// New defaults it to filepath.Base(os.Args[0]). The sercon CLI sets
	// it to "sercon".
	ProgramName string
	// WatchMode signals the engine is being driven by the sercon CLI's
	// --watch loop. Bindings that are incompatible with watch-style
	// re-runs (notably api.tui, which takes over the terminal) read
	// this via Engine.WatchMode() and throw at use.
	WatchMode bool
}

// Engine is the embeddable TypeScript script engine.
type Engine struct {
	opts          Options
	enableConsole bool

	regMu         sync.RWMutex
	registrations []registration

	// resolveHook, when set, is called with the absolute path of every
	// module file the require loader resolves during a Run. The sercon
	// CLI's --watch mode uses it to build a per-entry import graph so a
	// file change re-runs only the entries that import the changed file.
	// Set via SetResolveHook; not safe to swap concurrently with a Run.
	resolveHook func(absPath string)

	// docs maps a dotted registration path to its documentation string.
	// Top-level bindings use the bare name ("log", "http"); namespace
	// members use "namespace.member" ("http.get", "exec.shell"). The d.ts
	// emitter consults this map to render JSDoc blocks above each
	// declaration. Lives on the engine (not the registration) so the same
	// SetDocs call can attach docs to a registration that was made via
	// any of the five Register variants.
	docs map[string]string

	// regCache reuses a Registry across runs so compiled module bytecode is
	// cached on the Engine. Module exports remain per-runtime because each
	// run gets a fresh runtime via a new eventloop.
	regOnce sync.Once
	reg     *require.Registry

	// runCleanupMu guards runCleanups; the engine drains the slice in
	// LIFO order after every Run's loop.Run returns (success or
	// failure). Bindings register via AddRunCleanup, typically from a
	// namespace factory, to release per-Run resources (TUI applications,
	// network handles, ...) before the next Run starts.
	runCleanupMu sync.Mutex
	runCleanups  []func()
}

// New constructs an Engine with the supplied options. Defaults are applied for
// zero-value fields.
func New(opts Options) *Engine {
	if opts.ScriptRoot == "" {
		if wd, err := os.Getwd(); err == nil {
			opts.ScriptRoot = wd
		}
	}
	if opts.ProgramName == "" {
		opts.ProgramName = filepath.Base(os.Args[0])
	}
	return &Engine{
		opts:          opts,
		enableConsole: !opts.DisableConsole,
	}
}

// WatchMode reports whether the engine was constructed with
// Options.WatchMode set. Bindings consult this when they must refuse to
// run inside --watch (e.g., the TUI takes over the terminal and would
// fight the watch loop).
func (e *Engine) WatchMode() bool { return e.opts.WatchMode }

// Register adds a named binding visible to scripts as a global. value may be
// any Go value that goja can convert: function, struct, map, slice, primitive.
// Must be called before Run; concurrent registrations during a Run are not
// supported.
func (e *Engine) Register(name string, value any) error {
	if name == "" {
		return errors.New("scriptengine: empty registration name")
	}
	e.regMu.Lock()
	defer e.regMu.Unlock()
	e.registrations = append(e.registrations, registration{name: name, kind: regValue, value: value})
	return nil
}

// RegisterNamespace adds a named object whose properties are the given
// members. Useful for exposing a grouped surface like "api.http" without
// having to define a Go struct.
func (e *Engine) RegisterNamespace(name string, members map[string]any) error {
	if name == "" {
		return errors.New("scriptengine: empty namespace name")
	}
	if members == nil {
		members = map[string]any{}
	}
	e.regMu.Lock()
	defer e.regMu.Unlock()
	e.registrations = append(e.registrations, registration{name: name, kind: regNamespace, members: members})
	return nil
}

// RegisterConstructor adds a Go constructor function that can be called with
// `new` from JS to produce instances of the returned struct type.
func (e *Engine) RegisterConstructor(name string, ctor any) error {
	if name == "" {
		return errors.New("scriptengine: empty constructor name")
	}
	e.regMu.Lock()
	defer e.regMu.Unlock()
	e.registrations = append(e.registrations, registration{name: name, kind: regConstructor, value: ctor})
	return nil
}

// RegisterFactory registers a binding constructed lazily per Run with access
// to the EventLoop and the runtime. Use this for bindings that need to
// schedule resolutions back onto the loop (Promise-returning I/O is the
// common case).
func (e *Engine) RegisterFactory(name string, factory func(vm *goja.Runtime, loop *eventloop.EventLoop) any) error {
	if name == "" {
		return errors.New("scriptengine: empty factory name")
	}
	e.regMu.Lock()
	defer e.regMu.Unlock()
	e.registrations = append(e.registrations, registration{name: name, kind: regValue, value: factoryMarker{fn: factory}})
	return nil
}

type factoryMarker struct {
	fn func(vm *goja.Runtime, loop *eventloop.EventLoop) any
}

// RegisterNamespaceFactory is the loop-aware variant of RegisterNamespace. The
// callback receives the VM and EventLoop and returns the member map.
func (e *Engine) RegisterNamespaceFactory(name string, factory func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any) error {
	if name == "" {
		return errors.New("scriptengine: empty namespace name")
	}
	e.regMu.Lock()
	defer e.regMu.Unlock()
	e.registrations = append(e.registrations, registration{name: name, kind: regNamespace, value: namespaceFactoryMarker{fn: factory}})
	return nil
}

type namespaceFactoryMarker struct {
	fn func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any
}

// SetDocs attaches a documentation string to a registered binding so the
// .d.ts emitter renders it as a JSDoc comment above the declaration.
// `path` is the dotted lookup key: a bare name for a top-level binding
// ("log", "http"), or "namespace.member" for namespace members
// ("http.get", "exec.shell"). Multi-line docs use `\n`; each line is
// emitted as a separate `*` line inside the JSDoc block.
//
// SetDocs is additive: calling it for a path that already has a doc
// string replaces the previous value. Calling SetDocs before the
// matching Register / RegisterNamespace / RegisterFactory call is
// allowed — the doc is held until the emitter runs.
//
// Concurrency: like the Register methods, SetDocs must not race with a
// Run / WriteTypes / Reset on the same engine.
func (e *Engine) SetDocs(path, doc string) {
	if path == "" {
		return
	}
	e.regMu.Lock()
	defer e.regMu.Unlock()
	if e.docs == nil {
		e.docs = map[string]string{}
	}
	if doc == "" {
		delete(e.docs, path)
		return
	}
	e.docs[path] = doc
}

// SetMemberDocs is a convenience for documenting many members of a
// namespace in one call. The keys are bare member names (no dots);
// SetMemberDocs prepends the namespace prefix when storing them. The
// namespace itself can be documented separately via
// `SetDocs("namespace", ...)`.
func (e *Engine) SetMemberDocs(namespace string, docs map[string]string) {
	if namespace == "" {
		return
	}
	e.regMu.Lock()
	defer e.regMu.Unlock()
	if e.docs == nil {
		e.docs = map[string]string{}
	}
	for member, doc := range docs {
		key := namespace + "." + member
		if doc == "" {
			delete(e.docs, key)
			continue
		}
		e.docs[key] = doc
	}
}

func (e *Engine) registry() *require.Registry {
	e.regOnce.Do(func() {
		e.reg = require.NewRegistry(require.WithLoader(e.newSourceLoader()))
	})
	return e.reg
}

// ResetModuleCache discards the cached module registry so the next Run
// re-reads and re-compiles every imported module from source. The
// registry otherwise caches compiled bytecode across runs (a speed
// win), which means edits to imported files wouldn't be seen — the
// CLI's --watch mode calls this before each re-run so a changed
// module's new source actually takes effect. Not safe to call
// concurrently with a Run.
func (e *Engine) ResetModuleCache() {
	e.regOnce = sync.Once{}
	e.reg = nil
}

// RunOption customises a single Run / RunFile invocation without rebuilding
// the Engine. Options compose; if the same setting is set twice the later
// option wins.
type RunOption func(*runConfig)

type runConfig struct {
	scriptRoot string
	args       []string
}

// WithScriptRoot points this Run at a different base directory for
// require/import resolution, overriding Options.ScriptRoot for the lifetime
// of the call. Useful when one Engine is reused across many scripts that
// each live under their own directory.
func WithScriptRoot(dir string) RunOption {
	return func(c *runConfig) { c.scriptRoot = dir }
}

// WithArgs sets the user argument vector exposed to the script as
// runtime.argv[2:]. argv[0] is Options.ProgramName and argv[1] is the
// running script's path; these args follow. Applies only to this Run.
func WithArgs(args []string) RunOption {
	return func(c *runConfig) { c.args = args }
}

// Run executes source as the entry script for this engine. name is used in
// stack traces and diagnostics. The returned value is the resolved value of
// the script's top-level expression (currently always undefined; the spec
// reserves this slot for future top-level export support).
func (e *Engine) Run(ctx context.Context, name, source string, opts ...RunOption) (goja.Value, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	cfg := runConfig{scriptRoot: e.opts.ScriptRoot}
	for _, opt := range opts {
		opt(&cfg)
	}

	// Make name absolute against the effective ScriptRoot so that
	// require/import resolution from the entry script anchors at the right
	// directory. Subsequent module lookups walk the dirname chain via
	// goja_nodejs's getCurrentModulePath and need no further help.
	if !filepath.IsAbs(name) && cfg.scriptRoot != "" {
		name = filepath.Join(cfg.scriptRoot, name)
	}

	transpiled, err := transpileEntry(source, name)
	if err == nil {
		e.trace("transpile entry %s ->\n%s", name, transpiled.JS)
	}
	if err != nil {
		return nil, err
	}

	reg := e.registry()
	loop := eventloop.NewEventLoop(
		eventloop.WithRegistry(reg),
		eventloop.EnableConsole(e.enableConsole),
	)

	var (
		result    goja.Value
		scriptErr error
		vmRef     atomic.Pointer[goja.Runtime]
		timedOut  atomic.Bool
		canceled  atomic.Bool
	)

	done := make(chan struct{})

	// Watcher: interrupts the runtime on ctx cancellation or timeout. Always
	// exits via the done channel so watchers do not accumulate.
	go func() {
		var timeoutC <-chan time.Time
		if e.opts.Timeout > 0 {
			t := time.NewTimer(e.opts.Timeout)
			defer t.Stop()
			timeoutC = t.C
		}
		select {
		case <-done:
			return
		case <-ctx.Done():
			canceled.Store(true)
		case <-timeoutC:
			timedOut.Store(true)
		}
		if vm := vmRef.Load(); vm != nil {
			if timedOut.Load() {
				vm.Interrupt(ErrScriptTimeout)
			} else {
				vm.Interrupt(ctx.Err())
			}
		}
		loop.Terminate()
	}()

	loop.Run(func(vm *goja.Runtime) {
		vmRef.Store(vm)
		vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))

		if err := e.applyRegistrations(vm, loop); err != nil {
			scriptErr = err
			return
		}

		// Patch runtime.argv onto the registered `runtime` global. argv
		// mirrors Node/Bun: [programName, scriptPath, ...userArgs]. If the
		// host did not register a `runtime` namespace (e.g. a library-style
		// caller that bypassed the CLI's registerSurface), fall back to
		// creating a minimal one so argv is still reachable. Runs after
		// applyRegistrations so the CLI's runtime object is the patch target.
		argv := make([]string, 0, 2+len(cfg.args))
		argv = append(argv, e.opts.ProgramName, name)
		argv = append(argv, cfg.args...)
		argvValue := vm.ToValue(argv)
		if existing := vm.Get(reservedRuntimeName); existing != nil && !goja.IsUndefined(existing) && !goja.IsNull(existing) {
			obj, ok := existing.(*goja.Object)
			if !ok {
				scriptErr = fmt.Errorf("scriptengine: cannot patch %s.argv: host registered %q as %T, expected *goja.Object", reservedRuntimeName, reservedRuntimeName, existing.Export())
				return
			}
			_ = obj.Set("argv", argvValue)
		} else {
			// Fallback: no host-registered `runtime` namespace. Synthesise a
			// minimal object holding just `argv` so library-style callers still
			// see argv. Intentionally argv-only — if a host wants `runtime.log`,
			// `runtime.time`, etc., they must register the namespace themselves;
			// the engine won't grow this stub.
			runtimeObj := vm.NewObject()
			_ = runtimeObj.Set("argv", argvValue)
			_ = vm.Set(reservedRuntimeName, runtimeObj)
		}

		// __resolve / __reject are called by the async IIFE wrapper to surface
		// completion of the top-level script (including any awaited promises).
		_ = vm.Set("__resolve", func(call goja.FunctionCall) goja.Value {
			if scriptErr == nil {
				result = call.Argument(0)
			}
			return goja.Undefined()
		})
		_ = vm.Set("__reject", func(call goja.FunctionCall) goja.Value {
			arg := call.Argument(0)
			scriptErr = jsErrToGo(vm, arg)
			return goja.Undefined()
		})

		_, err := vm.RunScript(name, transpiled.JS)
		if err != nil {
			var ie *goja.InterruptedError
			if errors.As(err, &ie) {
				if v, ok := ie.Value().(error); ok {
					scriptErr = v
					return
				}
			}
			scriptErr = err
		}
	})

	close(done)

	// Drain per-Run cleanups in LIFO order. Bindings register here (via
	// Engine.AddRunCleanup) when they own per-Run resources that must
	// be released before the next Run — see api.tui for the canonical
	// case. We run cleanups before returning so the script-end error
	// path (timedOut / canceled) sees the terminal/resources already
	// restored.
	for _, fn := range reversed(e.drainRunCleanups()) {
		fn()
	}

	if scriptErr == nil {
		if timedOut.Load() {
			scriptErr = ErrScriptTimeout
		} else if canceled.Load() {
			scriptErr = ctx.Err()
		}
	}

	return result, scriptErr
}

// RunFile reads path from disk and executes it as the entry script.
func (e *Engine) RunFile(ctx context.Context, path string, opts ...RunOption) (goja.Value, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	return e.Run(ctx, abs, string(data), opts...)
}

// Reset clears every registered binding from the Engine. Useful when reusing
// a long-lived Engine across unrelated batches of scripts that want a clean
// global namespace. Not safe to call concurrently with Run / RunFile —
// registrations are part of the Run setup, and removing them mid-Run would
// race with applyRegistrations.
func (e *Engine) Reset() {
	e.regMu.Lock()
	defer e.regMu.Unlock()
	e.registrations = nil
}

// AddRunCleanup registers fn to run after the current Run's loop.Run
// returns. Callbacks fire in LIFO order. Each Run starts with an empty
// cleanup list; cleanups registered during one Run do not leak into the
// next. Safe to call concurrently from within a Run.
func (e *Engine) AddRunCleanup(fn func()) {
	if fn == nil {
		return
	}
	e.runCleanupMu.Lock()
	e.runCleanups = append(e.runCleanups, fn)
	e.runCleanupMu.Unlock()
}

// drainRunCleanups removes and returns the current set of cleanup
// callbacks. Called by Run after loop.Run completes.
func (e *Engine) drainRunCleanups() []func() {
	e.runCleanupMu.Lock()
	out := e.runCleanups
	e.runCleanups = nil
	e.runCleanupMu.Unlock()
	return out
}

func (e *Engine) applyRegistrations(vm *goja.Runtime, loop *eventloop.EventLoop) error {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	for _, reg := range e.registrations {
		switch reg.kind {
		case regValue:
			value := reg.value
			if m, ok := value.(factoryMarker); ok {
				value = m.fn(vm, loop)
			}
			if err := vm.Set(reg.name, unwrapAsyncBindings(value)); err != nil {
				return fmt.Errorf("register %s: %w", reg.name, err)
			}
		case regNamespace:
			members := reg.members
			if m, ok := reg.value.(namespaceFactoryMarker); ok {
				members = m.fn(vm, loop)
			}
			obj := vm.NewObject()
			for k, v := range members {
				if err := obj.Set(k, unwrapAsyncBindings(v)); err != nil {
					return fmt.Errorf("register %s.%s: %w", reg.name, k, err)
				}
			}
			if err := vm.Set(reg.name, obj); err != nil {
				return fmt.Errorf("register %s: %w", reg.name, err)
			}
		case regConstructor:
			if err := vm.Set(reg.name, unwrapAsyncBindings(reg.value)); err != nil {
				return fmt.Errorf("register constructor %s: %w", reg.name, err)
			}
		}
	}
	return nil
}

// jsErrToGo turns a JS value thrown to __reject into a Go error.
func jsErrToGo(vm *goja.Runtime, v goja.Value) error {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return errors.New("script rejected with undefined")
	}
	if obj, ok := v.(*goja.Object); ok {
		if msg := obj.Get("message"); msg != nil && !goja.IsUndefined(msg) {
			return errors.New(msg.String())
		}
	}
	return errors.New(v.String())
}

// SetResolveHook installs (or clears, with nil) a callback invoked
// with the absolute path of each module file resolved during a Run.
// Used by the CLI's --watch mode to capture per-entry import graphs.
// Must not be swapped concurrently with a Run on the same engine.
func (e *Engine) SetResolveHook(fn func(absPath string)) {
	e.resolveHook = fn
}

// WriteTypes emits a TypeScript declaration file for the registered surface.
func (e *Engine) WriteTypes(w io.Writer) error {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	return writeDTS(w, e.registrations, e.docs)
}

// reversed returns a new slice with the elements of in in reverse order.
// Used to drain run-cleanups LIFO.
func reversed(in []func()) []func() {
	out := make([]func(), len(in))
	for i, fn := range in {
		out[len(in)-1-i] = fn
	}
	return out
}

// trace writes one diagnostic line to Options.Verbose when set. Each line is
// prefixed with `[sercon] ` so the source is obvious in mixed output.
func (e *Engine) trace(format string, args ...any) {
	if e.opts.Verbose == nil {
		return
	}
	fmt.Fprintf(e.opts.Verbose, "[sercon] "+format+"\n", args...)
}
