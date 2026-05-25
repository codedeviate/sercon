package scriptengine

import (
	"context"
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

// PromisifyAsync wraps a Go function that performs blocking work and returns
// it as a JS-callable that produces a Promise. The wrapped function runs in a
// fresh goroutine; resolve/reject are scheduled back onto the event loop so
// the JS callbacks observe a consistent runtime state.
//
// `vm` and `loop` must belong to the same run — capture them inside a
// RegisterFactory / RegisterNamespaceFactory callback so each Run gets its
// own pair.
func PromisifyAsync[T any](vm *goja.Runtime, loop *eventloop.EventLoop, work func(ctx context.Context, call goja.FunctionCall) (T, error)) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()
		ctx := context.Background()

		// goja_nodejs/eventloop only counts setTimeout/setInterval/setImmediate
		// as "live" tasks; RunOnLoop alone does not keep loop.Run from returning.
		// Park a long-duration setTimeout while the work is in flight and clear
		// it on resolution so the loop drains exactly when the work is done.
		keepAlive := loop.SetTimeout(func(*goja.Runtime) {}, 24*time.Hour)

		go func() {
			val, err := work(ctx, call)
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if err != nil {
					reject(vm.NewGoError(err))
				} else {
					resolve(vm.ToValue(val))
				}
				loop.ClearTimeout(keepAlive)
			})
		}()
		return vm.ToValue(promise)
	}
}

// Promised is a marker type for Go bindings whose return value is a
// Promise-wrapping goja.Value. The d.ts generator inspects this type to emit
// `Promise<T>` for the TS return type. At runtime Promised[T] behaves exactly
// like the wrapped function.
type Promised[T any] func(call goja.FunctionCall) goja.Value
