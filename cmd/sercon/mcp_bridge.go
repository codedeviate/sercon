package main

import (
	"errors"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mcpThenChain chains .then/.catch onto a handler-returned Promise so its
// settled value or rejection reaches the onSettle/onReject callbacks passed
// in from Go. Mirrors bridgeProg in server_http.go (see bridgeHandlerResult
// / propagateResult there) — same mechanic, distilled into a reusable
// blocking call for the MCP tool/resource/prompt handlers. *goja.Program is
// safe to share across runtimes (per goja docs), so this is compiled once
// per process.
var mcpThenChain = goja.MustCompile("internal:mcpThenChain",
	`(p, onSettle, onReject) => p.then(onSettle, onReject)`, false)

// mcpSettled carries the outcome of a handler invocation from the loop
// goroutine back to callJSHandler's caller via a buffered channel.
type mcpSettled struct {
	val goja.Value
	err error
}

// callJSHandler invokes fn on the event loop with the args built by
// buildArgs, and blocks the calling goroutine until the result settles:
//
//   - a synchronous (non-Promise) return passes straight through;
//   - a returned Promise is chained via mcpThenChain, and the caller blocks
//     until it resolves (returning the fulfilled value) or rejects
//     (returning a Go error carrying the rejection reason).
//
// callJSHandler must be called from a goroutine OTHER than the event
// loop's own goroutine — it blocks on a channel that a loop callback
// fills in, so calling it from inside a loop callback would deadlock
// (see scriptengine.LoopCallable.CallOnLoop's docs for the same hazard).
// This is the single async primitive every MCP tool/resource/prompt
// handler uses to bridge into synchronous Go code (e.g. the go-sdk's
// request-handling goroutines).
//
// The completion channel is buffered (cap 1) so the loop callback below
// never blocks trying to send: whichever of the "no result" / "sync
// result" / "settle" / "reject" branches runs, it sends exactly once and
// returns immediately, keeping the loop free to process other work.
func (ms *mcpServer) callJSHandler(fn *scriptengine.LoopCallable, buildArgs func(vm *goja.Runtime) []goja.Value) (goja.Value, error) {
	done := make(chan mcpSettled, 1)

	if !ms.loop.RunOnLoop(func(vm *goja.Runtime) {
		res, err := fn.CallOnLoop(vm, buildArgs(vm)...)
		if err != nil {
			done <- mcpSettled{err: err}
			return
		}
		if res == nil || goja.IsUndefined(res) || goja.IsNull(res) {
			done <- mcpSettled{val: res}
			return
		}
		promise, ok := res.Export().(*goja.Promise)
		if !ok {
			done <- mcpSettled{val: res} // synchronous return
			return
		}

		thenVal, err := vm.RunProgram(mcpThenChain)
		if err != nil {
			done <- mcpSettled{err: err}
			return
		}
		thenFn, ok := goja.AssertFunction(thenVal)
		if !ok {
			done <- mcpSettled{err: errors.New("mcp: internal then-chain bridge is not callable")}
			return
		}
		onSettle := func(call goja.FunctionCall) goja.Value {
			done <- mcpSettled{val: call.Argument(0)}
			return goja.Undefined()
		}
		onReject := func(call goja.FunctionCall) goja.Value {
			done <- mcpSettled{err: promiseRejectionError(call.Argument(0))}
			return goja.Undefined()
		}
		if _, err := thenFn(goja.Undefined(), vm.ToValue(promise), vm.ToValue(onSettle), vm.ToValue(onReject)); err != nil {
			done <- mcpSettled{err: err}
		}
	}) {
		// The loop was already terminated (timeout/cancel watcher fired), so
		// the callback above was never enqueued and will never run — <-done
		// would block forever. Mirrors LoopCallable.Call's same guard.
		return nil, errors.New("mcp: event loop terminated before handler ran")
	}

	s := <-done
	return s.val, s.err
}

// promiseRejectionError converts a Promise rejection reason to a Go error.
// Reasons are typically an Error object (reason.String() renders as
// "Error: message") or any other thrown value; a nil/undefined/null
// rejection still produces a non-nil error so callers can distinguish
// "rejected" from "resolved".
func promiseRejectionError(reason goja.Value) error {
	if reason == nil || goja.IsUndefined(reason) || goja.IsNull(reason) {
		return errors.New("mcp: handler rejected with no reason")
	}
	return errors.New(reason.String())
}
