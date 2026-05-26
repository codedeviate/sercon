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
}

// Engine is the embeddable TypeScript script engine.
type Engine struct {
	opts          Options
	enableConsole bool

	regMu         sync.RWMutex
	registrations []registration

	// regCache reuses a Registry across runs so compiled module bytecode is
	// cached on the Engine. Module exports remain per-runtime because each
	// run gets a fresh runtime via a new eventloop.
	regOnce sync.Once
	reg     *require.Registry
}

// New constructs an Engine with the supplied options. Defaults are applied for
// zero-value fields.
func New(opts Options) *Engine {
	if opts.ScriptRoot == "" {
		if wd, err := os.Getwd(); err == nil {
			opts.ScriptRoot = wd
		}
	}
	return &Engine{
		opts:          opts,
		enableConsole: !opts.DisableConsole,
	}
}

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

func (e *Engine) registry() *require.Registry {
	e.regOnce.Do(func() {
		e.reg = require.NewRegistry(require.WithLoader(e.newSourceLoader()))
	})
	return e.reg
}

// RunOption customises a single Run / RunFile invocation without rebuilding
// the Engine. Options compose; if the same setting is set twice the later
// option wins.
type RunOption func(*runConfig)

type runConfig struct {
	scriptRoot string
}

// WithScriptRoot points this Run at a different base directory for
// require/import resolution, overriding Options.ScriptRoot for the lifetime
// of the call. Useful when one Engine is reused across many scripts that
// each live under their own directory.
func WithScriptRoot(dir string) RunOption {
	return func(c *runConfig) { c.scriptRoot = dir }
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

// WriteTypes emits a TypeScript declaration file for the registered surface.
func (e *Engine) WriteTypes(w io.Writer) error {
	e.regMu.RLock()
	defer e.regMu.RUnlock()
	return writeDTS(w, e.registrations)
}

// trace writes one diagnostic line to Options.Verbose when set. Each line is
// prefixed with `[sercon] ` so the source is obvious in mixed output.
func (e *Engine) trace(format string, args ...any) {
	if e.opts.Verbose == nil {
		return
	}
	fmt.Fprintf(e.opts.Verbose, "[sercon] "+format+"\n", args...)
}
