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
// it as a JS-callable that produces a Promise.
//
// The call is split in two to enforce goja's threading contract
// (goja.Runtime is single-threaded — executing VM code off the event loop
// is a data race):
//
//   - `extract` runs synchronously on the event loop thread, inside the JS
//     call. It is the ONLY place the goja arguments may be touched
//     (call.Argument, ToObject, Export, ToInteger, option helpers, ...).
//     It converts them into a plain-Go value A. An extract error rejects
//     the Promise immediately — same observable behaviour as a work error.
//   - `work` runs in a fresh goroutine and receives only A. It MUST NOT
//     touch the VM or any goja.Value: do not smuggle goja values (or
//     closures over them) through A.
//
// resolve/reject are scheduled back onto the event loop so the JS callbacks
// observe a consistent runtime state.
//
// `vm` and `loop` must belong to the same run — capture them inside a
// RegisterFactory / RegisterNamespaceFactory callback so each Run gets its
// own pair.
//
// The return value is an AsyncBinding carrier. The engine unwraps it for the
// runtime registration so goja can recognise the underlying callback signature
// as a host-callback. The .d.ts emitter reads the same carrier to emit
// `Promise<T>` instead of the previous `unknown`.
func PromisifyAsync[A, T any](vm *goja.Runtime, loop *eventloop.EventLoop,
	extract func(call goja.FunctionCall) (A, error),
	work func(ctx context.Context, args A) (T, error),
) AsyncBinding {
	fn := func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()
		ctx := runContextFromVM(vm)

		// On-loop argument extraction. Because this happens synchronously
		// inside the host call, goja's FunctionCall.Arguments reuse across
		// calls is not a hazard here — nothing goja-owned survives past
		// this call's return.
		args, err := extract(call)
		if err != nil {
			_ = reject(vm.NewGoError(err))
			return vm.ToValue(promise)
		}

		// goja_nodejs/eventloop only counts setTimeout/setInterval/setImmediate
		// as "live" tasks; RunOnLoop alone does not keep loop.Run from returning.
		// Park a long-duration setTimeout while the work is in flight and clear
		// it on resolution so the loop drains exactly when the work is done.
		keepAlive := loop.SetTimeout(func(*goja.Runtime) {}, 24*time.Hour)

		go func() {
			val, err := work(ctx, args)
			loop.RunOnLoop(func(vm *goja.Runtime) {
				// reject/resolve only error if the promise has already
				// settled — impossible here since we own both ends.
				if err != nil {
					_ = reject(vm.NewGoError(err))
				} else {
					// OrderedToValue builds any *Ordered result into a goja
					// object with deterministic key order, on the loop; other
					// result types fall through to vm.ToValue unchanged.
					_ = resolve(OrderedToValue(vm, val))
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

// runContextFromVM retrieves the current Run's context.Context, stashed on
// vm by Engine.Run under runCtxGlobalName (see engine.go) — the same
// "internal global on vm" mechanism used for __resolve/__reject. This lets
// PromisifyAsync (a package-level generic function with no Engine handle of
// its own) observe the enclosing Run's timeout/cancellation without
// widening its public signature. Falls back to context.Background() if
// nothing is stashed (e.g. a vm not produced by Engine.Run), matching the
// previous behaviour for that edge case.
func runContextFromVM(vm *goja.Runtime) context.Context {
	v := vm.Get(runCtxGlobalName)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return context.Background()
	}
	if h, ok := v.Export().(*runCtxHolder); ok && h != nil && h.ctx != nil {
		return h.ctx
	}
	return context.Background()
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
