package main

import (
	"bytes"
	"fmt"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// outStreamBinding builds the runtime.stdout / runtime.stderr handle for one
// stream. Registered per Run through the runtime namespace factory, so vm and
// loop are the live ones for this Run.
func outStreamBinding(vm *goja.Runtime, loop *eventloop.EventLoop, e *scriptengine.Engine, s *stream) map[string]any {
	return map[string]any{
		"to": func(call goja.FunctionCall) goja.Value {
			d, err := parseStreamTarget(vm, loop, e, s.base.name, call.Argument(0), teeOpt(call.Argument(1)))
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return restoreFn(vm, s.push(d))
		},
		"toFile": func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			opts := call.Argument(1)
			d, err := fileDest(path, boolOpt(opts, "append"), boolOpt(opts, "tee"))
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("toFile: %w", err)))
			}
			return restoreFn(vm, s.push(d))
		},
		"silence": func(goja.FunctionCall) goja.Value {
			return restoreFn(vm, s.push(nullDest(false)))
		},
		"reset": func(goja.FunctionCall) goja.Value {
			s.reset()
			return goja.Undefined()
		},
		"target": func(goja.FunctionCall) goja.Value {
			return vm.ToValue(s.targetInfo())
		},
		// scoped(target, fn) or scoped(target, opts, fn)
		"scoped": func(call goja.FunctionCall) goja.Value {
			target := call.Argument(0)
			optsArg := call.Argument(1)
			fnArg := call.Argument(2)
			if _, ok := goja.AssertFunction(fnArg); !ok {
				// Two-argument form: the callback sits where opts would.
				fnArg = optsArg
				optsArg = goja.Undefined()
			}
			fn, ok := goja.AssertFunction(fnArg)
			if !ok {
				panic(vm.NewGoError(fmt.Errorf("scoped: last argument must be a function")))
			}
			d, err := parseStreamTarget(vm, loop, e, s.base.name, target, teeOpt(optsArg))
			if err != nil {
				panic(vm.NewGoError(err))
			}
			restore := s.push(d)
			return callScopedFn(vm, fn, restore, func() goja.Value { return goja.Undefined() })
		},
		// capture(fn) -> Promise<string>
		"capture": func(call goja.FunctionCall) goja.Value {
			fn, ok := goja.AssertFunction(call.Argument(0))
			if !ok {
				panic(vm.NewGoError(fmt.Errorf("capture: argument must be a function")))
			}
			// Always exclusive — never tees. Use scoped with { tee: true } to
			// keep output on the terminal as well.
			sink := &lockedBuffer{}
			restore := s.push(destination{kind: destBuffer, w: sink})
			return callScopedFn(vm, fn, restore, func() goja.Value { return vm.ToValue(sink.String()) })
		},
	}
}

// lockedBuffer is capture()'s sink. The stream mutex already serialises writes,
// but capture() reads the buffer from the loop after the callback settles, so
// the buffer carries its own lock to keep that read safe under -race.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// settleAfter runs cleanup once result has settled, then resolves the returned
// promise with value() (or rejects with result's rejection reason).
//
// It handles a sync and an async fn with one path: if result is a thenable we
// chain onto it, otherwise cleanup runs immediately. Either way the caller gets
// a Promise back, so `await` is always correct and a synchronous fn cannot leak
// the redirect.
func settleAfter(vm *goja.Runtime, result goja.Value, cleanup func(), value func() goja.Value) goja.Value {
	promise, resolve, reject := vm.NewPromise()

	thenable := false
	if obj, ok := result.(*goja.Object); ok && obj != nil {
		if then, ok := goja.AssertFunction(obj.Get("then")); ok {
			thenable = true
			onOK := vm.ToValue(func(goja.FunctionCall) goja.Value {
				cleanup()
				_ = resolve(value())
				return goja.Undefined()
			})
			onErr := vm.ToValue(func(call goja.FunctionCall) goja.Value {
				cleanup()
				_ = reject(call.Argument(0))
				return goja.Undefined()
			})
			if _, err := then(obj, onOK, onErr); err != nil {
				cleanup()
				_ = reject(exceptionValue(vm, err))
			}
		}
	}
	if !thenable {
		cleanup()
		_ = resolve(value())
	}
	return vm.ToValue(promise)
}

// callScopedFn invokes fn, converting a Go-side error from fn(...) — how goja
// surfaces a synchronous JS throw — into a rejected promise after cleanup has
// run.
//
// settleAfter reads result.then via (*goja.Object).Get, which goja documents
// as panicking with a *goja.Exception when the read itself throws (a JS
// getter, or a revoked Proxy) — and that read happens after cleanup's push is
// already live. The deferred recover is the only thing standing between that
// panic and a permanently leaked redirect: it restores, then re-panics with
// the same value so the panic still reaches goja's call boundary and becomes
// the script's catchable throw, same as every other panic(vm.NewGoError(...))
// site in this file.
func callScopedFn(vm *goja.Runtime, fn goja.Callable, cleanup func(), value func() goja.Value) goja.Value {
	defer func() {
		if r := recover(); r != nil {
			cleanup() // idempotent — push's restore is sync.Once-guarded
			panic(r)
		}
	}()
	res, err := fn(goja.Undefined())
	if err != nil {
		cleanup()
		promise, _, reject := vm.NewPromise()
		_ = reject(exceptionValue(vm, err))
		return vm.ToValue(promise)
	}
	return settleAfter(vm, res, cleanup, value)
}

// exceptionValue recovers the original thrown JS value from a synchronous
// throw. fn(...) reports a JS throw as a *goja.Exception; vm.ToValue(err)
// on that would wrap it as an opaque Go value — losing the message and
// failing `instanceof Error` on the JS side — so unwrap it back to the value
// the script actually threw. Any other Go-side error (there's currently no
// path that produces one here) still surfaces as a proper Error object
// rather than being swallowed.
func exceptionValue(vm *goja.Runtime, err error) goja.Value {
	if exc, ok := err.(*goja.Exception); ok {
		return exc.Value()
	}
	return vm.NewGoError(err)
}

// restoreFn wraps a Go restore closure as a JS function. The closure is already
// idempotent (see stream.push), so a script may call it any number of times.
func restoreFn(vm *goja.Runtime, restore func()) goja.Value {
	return vm.ToValue(func(goja.FunctionCall) goja.Value {
		restore()
		return goja.Undefined()
	})
}

// teeOpt reads { tee: bool } from an options argument.
func teeOpt(v goja.Value) bool { return boolOpt(v, "tee") }

// boolOpt reads one boolean field from an options object, defaulting to false
// when the object or the field is absent.
func boolOpt(v goja.Value, name string) bool {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return false
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return false
	}
	f := obj.Get(name)
	return f != nil && !goja.IsUndefined(f) && f.ToBoolean()
}

// parseStreamTarget turns a StreamTarget JS value into a destination.
// streamName ("stdout" | "stderr") identifies the stream this target is
// being pushed onto — only used to name the stream in a function target's
// reportThrow diagnostic (see stdio_callback.go).
//
//	"stdout" | "stderr"          -> fold onto that PROCESS stream
//	"null"                       -> discard
//	{ file, append? }            -> a file
//	(line: string) => void       -> a JS line handler
func parseStreamTarget(vm *goja.Runtime, loop *eventloop.EventLoop, e *scriptengine.Engine, streamName string, target goja.Value, tee bool) (destination, error) {
	if target == nil || goja.IsUndefined(target) || goja.IsNull(target) {
		return destination{}, fmt.Errorf("to: a target is required (\"stdout\" | \"stderr\" | \"null\" | { file } | function)")
	}

	// A callable target is a line handler.
	if fn, ok := goja.AssertFunction(target); ok {
		return callbackDest(loop, streamName, fn, tee)
	}

	if obj, ok := target.(*goja.Object); ok {
		fileVal := obj.Get("file")
		if fileVal != nil && !goja.IsUndefined(fileVal) && !goja.IsNull(fileVal) {
			return fileDest(fileVal.String(), boolOpt(target, "append"), tee)
		}
		return destination{}, fmt.Errorf("to: object target needs a `file` property")
	}

	switch name := target.String(); name {
	case "null":
		if tee {
			return destination{}, fmt.Errorf("to: { tee: true } is meaningless with the \"null\" target — tee-ing to the void discards both copies")
		}
		return nullDest(false), nil
	case "stdout", "stderr":
		return processStreamDest(name, tee)
	default:
		return destination{}, fmt.Errorf("to: unknown target %q (want \"stdout\", \"stderr\", \"null\", { file }, or a function)", name)
	}
}
