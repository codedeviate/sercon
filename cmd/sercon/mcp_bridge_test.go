package main

import (
	"context"
	"strings"
	"testing"

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
func mcpBridgeFixture(t *testing.T, arg int64, onDone func(val goja.Value, err error)) *scriptengine.Engine {
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
					val, err := ms.callJSHandler(handler, func(vm *goja.Runtime) []goja.Value {
						return []goja.Value{vm.ToValue(arg)}
					})
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

// TestMCPBridge_AsyncResolve verifies that a handler returning a resolved
// Promise unblocks callJSHandler with the fulfilled value.
func TestMCPBridge_AsyncResolve(t *testing.T) {
	var gotVal goja.Value
	var gotErr error
	eng := mcpBridgeFixture(t, 41, func(val goja.Value, err error) {
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
	if gotVal == nil || gotVal.ToInteger() != 42 {
		t.Fatalf("got %v, want 42", gotVal)
	}
}

// TestMCPBridge_AsyncReject verifies that a handler whose Promise rejects
// surfaces as a Go error carrying the rejection reason.
func TestMCPBridge_AsyncReject(t *testing.T) {
	var gotVal goja.Value
	var gotErr error
	eng := mcpBridgeFixture(t, 41, func(val goja.Value, err error) {
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
	if gotVal != nil && !goja.IsUndefined(gotVal) {
		t.Fatalf("expected no value on rejection, got %v", gotVal)
	}
}

// TestMCPBridge_SyncPassthrough verifies that a handler returning a plain
// (non-Promise) value passes straight through without any Promise bridge.
func TestMCPBridge_SyncPassthrough(t *testing.T) {
	var gotVal goja.Value
	var gotErr error
	eng := mcpBridgeFixture(t, 21, func(val goja.Value, err error) {
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
	if gotVal == nil || gotVal.ToInteger() != 42 {
		t.Fatalf("got %v, want 42", gotVal)
	}
}
