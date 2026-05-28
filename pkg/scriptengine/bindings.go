package scriptengine

import (
	"context"
	"reflect"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// registrationKind tags a registered host value so the engine and the d.ts
// generator can treat each kind correctly.
type registrationKind int

const (
	regValue       registrationKind = iota // generic value (function, struct, primitive)
	regNamespace                           // map[string]any to be exposed as an object
	regConstructor                         // function used as a constructor
)

type registration struct {
	name    string
	kind    registrationKind
	value   any
	members map[string]any
}

// AsyncBinding is the carrier returned by PromisifyAsync. It pairs the raw
// `func(goja.FunctionCall) goja.Value` callback (which goja recognises as a
// host function and packs JS args into) with the TypeScript type expression
// for the Promise's resolved value. The engine unwraps AsyncBinding to its
// raw Func at registration time so goja's special-case detection still
// fires; the .d.ts emitter reads TSReturnType to emit `Promise<T>`.
type AsyncBinding struct {
	// Func is the underlying goja-callable host function.
	Func func(goja.FunctionCall) goja.Value
	// TSReturnType is the TypeScript expression for the resolved value type
	// (e.g. "number", "string", "Record<string, unknown>"). Populated from
	// the generic parameter via reflect at construction time.
	TSReturnType string
}

// PromisifyAsync wraps a Go function that performs blocking work and returns
// it as a JS-callable that produces a Promise. The wrapped function runs in a
// fresh goroutine; resolve/reject are scheduled back onto the event loop so
// the JS callbacks observe a consistent runtime state.
//
// `vm` and `loop` must belong to the same run — capture them inside a
// RegisterFactory / RegisterNamespaceFactory callback so each Run gets its
// own pair.
//
// The return value is an AsyncBinding carrier. The engine unwraps it for the
// runtime registration so goja can recognise the underlying callback signature
// as a host-callback. The .d.ts emitter reads the same carrier to emit
// `Promise<T>` instead of the previous `unknown`.
func PromisifyAsync[T any](vm *goja.Runtime, loop *eventloop.EventLoop, work func(ctx context.Context, call goja.FunctionCall) (T, error)) AsyncBinding {
	fn := func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()
		ctx := context.Background()

		// goja_nodejs/eventloop only counts setTimeout/setInterval/setImmediate
		// as "live" tasks; RunOnLoop alone does not keep loop.Run from returning.
		// Park a long-duration setTimeout while the work is in flight and clear
		// it on resolution so the loop drains exactly when the work is done.
		keepAlive := loop.SetTimeout(func(*goja.Runtime) {}, 24*time.Hour)

		// goja documents that FunctionCall.Arguments must not be retained
		// past the native function's return — goja reuses the slice's
		// backing array across calls. With Promise.all (or any pattern
		// where multiple async bindings fire before any resolves), a later
		// call would mutate the earlier goroutine's view. Snapshot the
		// arguments now so each work goroutine has a stable view.
		argsCopy := make([]goja.Value, len(call.Arguments))
		copy(argsCopy, call.Arguments)
		snap := goja.FunctionCall{This: call.This, Arguments: argsCopy}

		go func() {
			val, err := work(ctx, snap)
			loop.RunOnLoop(func(vm *goja.Runtime) {
				// reject/resolve only error if the promise has already
				// settled — impossible here since we own both ends.
				if err != nil {
					_ = reject(vm.NewGoError(err))
				} else {
					_ = resolve(vm.ToValue(val))
				}
				loop.ClearTimeout(keepAlive)
			})
		}()
		return vm.ToValue(promise)
	}

	// Compute the TS expression for T at registration time. The pointer
	// trick (`reflect.TypeOf((*T)(nil)).Elem()`) gets us a Type even when
	// T's zero value is nil-ish (e.g. an interface type, slice, map).
	tsRet := tsType(newTypeCtx(), reflect.TypeOf((*T)(nil)).Elem())

	return AsyncBinding{Func: fn, TSReturnType: tsRet}
}

// unwrapAsyncBindings walks v and replaces any AsyncBinding with its bare
// Func so goja's host-callback special case fires at vm.Set time. Recurses
// into `map[string]any` values so nested namespace shapes (e.g.
// `api.http: { get: PromisifyAsync(...) }`) are unwrapped too. Other values
// pass through unchanged.
func unwrapAsyncBindings(v any) any {
	switch t := v.(type) {
	case AsyncBinding:
		return t.Func
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			out[k] = unwrapAsyncBindings(val)
		}
		return out
	default:
		return v
	}
}
