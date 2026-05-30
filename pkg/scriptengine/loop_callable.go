package scriptengine

import (
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// LoopCallable wraps a goja.Callable plus the EventLoop that owns its
// runtime, so the callable can be invoked safely from any goroutine.
// Without LoopCallable, calling a captured Callable directly from a
// non-loop goroutine corrupts goja's single-threaded runtime state.
//
// Typical use:
//
//	lc := scriptengine.NewLoopCallable(loop, handlerFn)
//	go func() {
//	    result, err := lc.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
//	        return []goja.Value{vm.ToValue("hello")}, nil
//	    })
//	}()
//
// The args-builder runs on the loop (where vm is valid). The return value
// (which may be a Promise) flows back to the caller's goroutine.
// LoopCallable does not await Promises — the caller is responsible for
// chaining .then via a JS bridge if the JS function is async.
type LoopCallable struct {
	fn   goja.Callable
	loop *eventloop.EventLoop
}

// NewLoopCallable constructs a LoopCallable. loop and fn must come from
// the same goja.Runtime / EventLoop pair (typically captured during a
// RegisterNamespaceFactory).
func NewLoopCallable(loop *eventloop.EventLoop, fn goja.Callable) *LoopCallable {
	return &LoopCallable{fn: fn, loop: loop}
}

// CallOnLoop invokes the wrapped callable directly without scheduling.
// The caller MUST already be running on the event loop (e.g. inside a
// RunOnLoop callback or a binding called from JS). Using Call() while
// already on the loop deadlocks because Call enqueues a new RunOnLoop and
// waits for it, but the loop is single-threaded and can't process the new
// job until the current callback returns.
//
// vm is the loop's runtime (the one received by the enclosing RunOnLoop
// callback). args are the JS arguments to pass; build them with vm.ToValue
// or by passing already-loop-bound goja.Values.
func (lc *LoopCallable) CallOnLoop(vm *goja.Runtime, args ...goja.Value) (goja.Value, error) {
	if lc == nil || lc.fn == nil {
		return nil, errors.New("scriptengine: LoopCallable.CallOnLoop on uninitialised value")
	}
	return lc.fn(goja.Undefined(), args...)
}

// Call schedules a callback on the loop, builds the arg list on the loop
// via buildArgs, invokes the wrapped callable, and returns the result to
// the caller goroutine. Blocks until the loop callback completes.
//
// If buildArgs returns an error, Call returns that error without invoking
// the callable. If the callable panics or throws synchronously, Call
// returns the recovered/thrown error wrapped as a Go error.
//
// Note: Call does NOT keep the loop alive. Use Engine.HoldRun on the
// owning binding to ensure loop.Run does not exit while Calls are
// outstanding.
func (lc *LoopCallable) Call(buildArgs func(vm *goja.Runtime) ([]goja.Value, error)) (goja.Value, error) {
	if lc == nil || lc.fn == nil || lc.loop == nil {
		return nil, errors.New("scriptengine: LoopCallable.Call on uninitialised value")
	}
	type result struct {
		val goja.Value
		err error
	}
	done := make(chan result, 1)
	// RunOnLoop returns false when the loop has been terminated (timeout /
	// cancel watcher) — the job is NOT enqueued and would never run, so the
	// `<-done` below would block forever, leaking the calling goroutine (and
	// any fd it holds). Detect that and return promptly instead; every Call
	// site treats a returned error as "stop", so the caller's goroutine exits.
	if !lc.loop.RunOnLoop(func(vm *goja.Runtime) {
		defer func() {
			if r := recover(); r != nil {
				done <- result{nil, fmt.Errorf("scriptengine: LoopCallable.Call panicked: %v", r)}
			}
		}()
		args, err := buildArgs(vm)
		if err != nil {
			done <- result{nil, err}
			return
		}
		v, err := lc.fn(goja.Undefined(), args...)
		if err != nil {
			err = fmt.Errorf("scriptengine: LoopCallable.Call: %w", err)
		}
		done <- result{v, err}
	}) {
		return nil, errLoopTerminated
	}
	r := <-done
	return r.val, r.err
}

// errLoopTerminated is returned by Call when the event loop was terminated
// before the scheduled callback could run (so it will never run). Call sites
// treat any error as a signal to stop and unwind their goroutine.
var errLoopTerminated = errors.New("scriptengine: event loop terminated before call ran")
