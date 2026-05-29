package scriptengine

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// TestLoopCallable_CapturedDuringFactory verifies that NewLoopCallable
// can be constructed inside a binding factory and the resulting pointer
// is non-nil. The actual cross-goroutine .Call paths are exercised by
// TestLoopCallable_CrossGoroutineCall and friends below.
func TestLoopCallable_CapturedDuringFactory(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	var captured *LoopCallable
	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"setHandler": func(call goja.FunctionCall) goja.Value {
				fn, ok := goja.AssertFunction(call.Argument(0))
				if !ok {
					panic(vm.NewTypeError("setHandler: expected function"))
				}
				captured = NewLoopCallable(loop, fn)
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "test.ts", `test.setHandler(x => x.toUpperCase());`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if captured == nil {
		t.Fatal("captured nil")
	}
}

// TestLoopCallable_CrossGoroutineCall exercises the actual point of
// LoopCallable: invoke a captured JS function from a non-loop goroutine.
// A HoldRun keeps the loop alive while the goroutine schedules the call.
func TestLoopCallable_CrossGoroutineCall(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	var (
		captured *LoopCallable
		gotValue string
		gotErr   error
		release  func()
	)

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"setHandler": func(call goja.FunctionCall) goja.Value {
				fn, ok := goja.AssertFunction(call.Argument(0))
				if !ok {
					panic(vm.NewTypeError("setHandler: expected function"))
				}
				captured = NewLoopCallable(loop, fn)
				return goja.Undefined()
			},
			"fireFromGoroutine": func() goja.Value {
				// Hold the loop alive so the goroutine's .Call can reach RunOnLoop.
				release = eng.HoldRun("test-cross-goroutine")
				go func() {
					defer release()
					val, err := captured.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
						return []goja.Value{vm.ToValue("hello")}, nil
					})
					gotErr = err
					if val != nil {
						gotValue = val.String()
					}
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	_, err := eng.Run(context.Background(), "cross.ts", `
test.setHandler(x => x.toUpperCase());
test.fireFromGoroutine();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotErr != nil {
		t.Fatalf("Call: %v", gotErr)
	}
	if gotValue != "HELLO" {
		t.Fatalf("got %q, want %q", gotValue, "HELLO")
	}
}

// TestLoopCallable_BuildArgsError verifies that an error from buildArgs
// is returned to the caller and the wrapped callable is not invoked.
func TestLoopCallable_BuildArgsError(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	var (
		captured *LoopCallable
		gotErr   error
		invoked  bool
		release  func()
	)

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"setHandler": func(call goja.FunctionCall) goja.Value {
				fn, ok := goja.AssertFunction(call.Argument(0))
				if !ok {
					panic(vm.NewTypeError("setHandler: expected function"))
				}
				captured = NewLoopCallable(loop, fn)
				return goja.Undefined()
			},
			"observeInvocation": func() goja.Value {
				invoked = true
				return goja.Undefined()
			},
			"fire": func() goja.Value {
				release = eng.HoldRun("test-buildargs-error")
				go func() {
					defer release()
					_, gotErr = captured.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
						return nil, errors.New("synthetic buildArgs failure")
					})
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	_, err := eng.Run(context.Background(), "buildargs.ts", `
test.setHandler(() => test.observeInvocation());
test.fire();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "synthetic buildArgs failure") {
		t.Fatalf("Call err = %v, want synthetic buildArgs failure", gotErr)
	}
	if invoked {
		t.Fatal("wrapped callable should NOT have been invoked when buildArgs errored")
	}
}

// TestLoopCallable_BuildArgsPanic verifies a panicking buildArgs is
// recovered and surfaced as an error (not a process crash).
func TestLoopCallable_BuildArgsPanic(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	var (
		captured *LoopCallable
		gotErr   error
		release  func()
	)

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"setHandler": func(call goja.FunctionCall) goja.Value {
				fn, _ := goja.AssertFunction(call.Argument(0))
				captured = NewLoopCallable(loop, fn)
				return goja.Undefined()
			},
			"fire": func() goja.Value {
				release = eng.HoldRun("test-buildargs-panic")
				go func() {
					defer release()
					_, gotErr = captured.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
						panic("buildArgs blew up")
					})
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "panic.ts", `test.setHandler(() => null); test.fire();`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotErr == nil || !strings.Contains(gotErr.Error(), "panicked") {
		t.Fatalf("Call err = %v, want panic-wrapped error", gotErr)
	}
}

// TestLoopCallable_JSThrow verifies a synchronously-thrown JS error is
// returned wrapped with the scriptengine prefix.
func TestLoopCallable_JSThrow(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	var (
		captured *LoopCallable
		gotErr   error
		release  func()
	)

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"setHandler": func(call goja.FunctionCall) goja.Value {
				fn, _ := goja.AssertFunction(call.Argument(0))
				captured = NewLoopCallable(loop, fn)
				return goja.Undefined()
			},
			"fire": func() goja.Value {
				release = eng.HoldRun("test-js-throw")
				go func() {
					defer release()
					_, gotErr = captured.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
						return nil, nil
					})
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "throw.ts", `
test.setHandler(() => { throw new Error("boom"); });
test.fire();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if gotErr == nil {
		t.Fatal("expected Call to return JS throw as error")
	}
	if !strings.Contains(gotErr.Error(), "scriptengine: LoopCallable.Call") {
		t.Fatalf("expected scriptengine-prefixed error, got %v", gotErr)
	}
	if !strings.Contains(gotErr.Error(), "boom") {
		t.Fatalf("expected original error text preserved, got %v", gotErr)
	}
}
