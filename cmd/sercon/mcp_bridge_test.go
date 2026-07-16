package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mcpBridgeFixture wires an mcpServer whose vm/loop are captured from a
// per-Run RegisterNamespaceFactory (mirrors
// pkg/scriptengine/loop_callable_test.go's TestLoopCallable_CrossGoroutineCall),
// plus a `test.setHandler(fn)` / `test.fire()` pair of JS bindings so each
// test can hand callJSHandler a real captured goja.Callable without going
// through mcp.serve + tool/resource/prompt registration (that wiring is
// Task 5+, out of scope here).
//
// `fire()` holds the loop alive (HoldRun) for the duration of the
// goroutine that calls callJSHandler off-loop, then releases once the
// bridge settles — exactly the pattern a real SDK-goroutine caller will
// use once tool/resource/prompt handlers route through callJSHandler.
// exportConvert is the trivial on-loop converter the bridge tests pass to
// callJSHandler: it exports the settled goja.Value to native Go data (the
// same value the tests asserted on before callJSHandler grew its convert
// parameter). Real callers pass a richer converter (e.g. toToolResult).
func exportConvert(_ *goja.Runtime, v goja.Value) (any, error) {
	if v == nil {
		return nil, nil
	}
	return v.Export(), nil
}

// numEq reports whether an exported JS number equals want, tolerating goja's
// int64/float64 export ambiguity for whole numbers.
func numEq(v any, want float64) bool {
	switch n := v.(type) {
	case int64:
		return float64(n) == want
	case float64:
		return n == want
	default:
		return false
	}
}

func mcpBridgeFixture(t *testing.T, arg int64, onDone func(val any, err error)) *scriptengine.Engine {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var (
		ms      *mcpServer
		handler *scriptengine.LoopCallable
	)
	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		ms = &mcpServer{eng: eng, vm: vm, loop: loop}
		return map[string]any{
			"setHandler": func(call goja.FunctionCall) goja.Value {
				fn, ok := goja.AssertFunction(call.Argument(0))
				if !ok {
					panic(vm.NewTypeError("setHandler: expected function"))
				}
				handler = scriptengine.NewLoopCallable(loop, fn)
				return goja.Undefined()
			},
			"fire": func() goja.Value {
				release := eng.HoldRun("test-mcp-bridge")
				go func() {
					defer release()
					val, err := ms.callJSHandler(context.Background(), handler, func(vm *goja.Runtime) []goja.Value {
						return []goja.Value{vm.ToValue(arg)}
					}, exportConvert)
					onDone(val, err)
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}
	return eng
}

// mcpBridgeCtxFixture is mcpBridgeFixture parameterized over the context passed
// to the package-level callJSHandler, so a test can drive its cancellation /
// deadline behaviour. The handler is set from the script (typically one that
// returns a never-settling Promise).
func mcpBridgeCtxFixture(t *testing.T, ctx context.Context, onDone func(val any, err error)) *scriptengine.Engine {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	var handler *scriptengine.LoopCallable
	var theLoop *eventloop.EventLoop
	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		theLoop = loop
		return map[string]any{
			"setHandler": func(call goja.FunctionCall) goja.Value {
				fn, ok := goja.AssertFunction(call.Argument(0))
				if !ok {
					panic(vm.NewTypeError("setHandler: expected function"))
				}
				handler = scriptengine.NewLoopCallable(loop, fn)
				return goja.Undefined()
			},
			"fire": func() goja.Value {
				release := eng.HoldRun("test-mcp-bridge-ctx")
				go func() {
					defer release()
					val, err := callJSHandler(ctx, theLoop, handler,
						func(vm *goja.Runtime) []goja.Value { return nil },
						exportConvert)
					onDone(val, err)
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}
	return eng
}

// TestMCPBridge_ContextCanceled verifies callJSHandler returns promptly with the
// context error (instead of blocking forever) when its context is already
// cancelled and the handler never settles.
func TestMCPBridge_ContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before fire()
	done := make(chan error, 1)
	eng := mcpBridgeCtxFixture(t, ctx, func(_ any, err error) { done <- err })
	if _, err := eng.Run(context.Background(), "bridge_cancel.ts", `
test.setHandler(() => new Promise(() => {}));  // never settles
test.fire();
`); err != nil {
		t.Fatalf("run: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want context.Canceled, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("callJSHandler did not return after ctx cancel — it blocked on a never-settling handler")
	}
}

// TestMCPBridge_ContextDeadline verifies the handlerTimeout path: a short
// deadline on the context unblocks callJSHandler with DeadlineExceeded when the
// handler never settles.
func TestMCPBridge_ContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	eng := mcpBridgeCtxFixture(t, ctx, func(_ any, err error) { done <- err })
	if _, err := eng.Run(context.Background(), "bridge_deadline.ts", `
test.setHandler(() => new Promise(() => {}));  // never settles
test.fire();
`); err != nil {
		t.Fatalf("run: %v", err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("want context.DeadlineExceeded, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("callJSHandler did not return after deadline")
	}
}

// TestMCPBridge_AsyncResolve verifies that a handler returning a resolved
// Promise unblocks callJSHandler with the fulfilled value.
func TestMCPBridge_AsyncResolve(t *testing.T) {
	var gotVal any
	var gotErr error
	eng := mcpBridgeFixture(t, 41, func(val any, err error) {
		gotVal, gotErr = val, err
	})

	if _, err := eng.Run(context.Background(), "bridge_resolve.ts", `
test.setHandler(async (x) => x + 1);
test.fire();
`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotErr != nil {
		t.Fatalf("callJSHandler: unexpected error: %v", gotErr)
	}
	if !numEq(gotVal, 42) {
		t.Fatalf("got %v, want 42", gotVal)
	}
}

// TestMCPBridge_AsyncReject verifies that a handler whose Promise rejects
// surfaces as a Go error carrying the rejection reason.
func TestMCPBridge_AsyncReject(t *testing.T) {
	var gotVal any
	var gotErr error
	eng := mcpBridgeFixture(t, 41, func(val any, err error) {
		gotVal, gotErr = val, err
	})

	if _, err := eng.Run(context.Background(), "bridge_reject.ts", `
test.setHandler(async () => { throw new Error("boom"); });
test.fire();
`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotErr == nil {
		t.Fatal("callJSHandler: expected error, got nil")
	}
	if !strings.Contains(gotErr.Error(), "boom") {
		t.Fatalf("error = %q, want it to contain %q", gotErr.Error(), "boom")
	}
	if gotVal != nil {
		t.Fatalf("expected no value on rejection, got %v", gotVal)
	}
}

// TestMCPBridge_BuildArgsPanic verifies that a panic in buildArgs — which
// runs as a plain Go call inside the RunOnLoop callback, BEFORE goja's
// protected CallOnLoop — is recovered and surfaced as a Go error instead
// of crashing the eventloop's job() runner (and the test process with
// it). This exercises the exact hazard the recover guard in
// callJSHandler was added for.
func TestMCPBridge_BuildArgsPanic(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var (
		ms      *mcpServer
		handler *scriptengine.LoopCallable
	)
	var gotVal any
	var gotErr error
	done := make(chan struct{})

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		ms = &mcpServer{eng: eng, vm: vm, loop: loop}
		return map[string]any{
			"setHandler": func(call goja.FunctionCall) goja.Value {
				fn, ok := goja.AssertFunction(call.Argument(0))
				if !ok {
					panic(vm.NewTypeError("setHandler: expected function"))
				}
				handler = scriptengine.NewLoopCallable(loop, fn)
				return goja.Undefined()
			},
			"fire": func() goja.Value {
				release := eng.HoldRun("test-mcp-bridge-panic")
				go func() {
					defer release()
					val, err := ms.callJSHandler(context.Background(), handler, func(vm *goja.Runtime) []goja.Value {
						panic("boom")
					}, exportConvert)
					gotVal, gotErr = val, err
					close(done)
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := eng.Run(context.Background(), "bridge_panic.ts", `
test.setHandler((x) => x);
test.fire();
`); err != nil {
		t.Fatalf("run: %v", err)
	}
	<-done

	if gotErr == nil {
		t.Fatal("callJSHandler: expected error from panicking buildArgs, got nil")
	}
	if !strings.Contains(gotErr.Error(), "boom") {
		t.Fatalf("error = %q, want it to contain %q", gotErr.Error(), "boom")
	}
	if gotVal != nil {
		t.Fatalf("expected no value, got %v", gotVal)
	}
	// Reaching this line at all proves the panic was recovered rather
	// than crashing the test process.
}

// TestMCPBridge_SyncPassthrough verifies that a handler returning a plain
// (non-Promise) value passes straight through without any Promise bridge.
func TestMCPBridge_SyncPassthrough(t *testing.T) {
	var gotVal any
	var gotErr error
	eng := mcpBridgeFixture(t, 21, func(val any, err error) {
		gotVal, gotErr = val, err
	})

	if _, err := eng.Run(context.Background(), "bridge_sync.ts", `
test.setHandler((x) => x * 2);
test.fire();
`); err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotErr != nil {
		t.Fatalf("callJSHandler: unexpected error: %v", gotErr)
	}
	if !numEq(gotVal, 42) {
		t.Fatalf("got %v, want 42", gotVal)
	}
}
