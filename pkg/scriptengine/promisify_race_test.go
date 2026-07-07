package scriptengine_test

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// PromisifyAsync work functions run in a fresh goroutine. goja.Runtime is
// single-threaded: any Value method that executes VM code (ToObject, Get on
// a getter-backed property, Export of an object, ToInteger via valueOf)
// must happen on the event loop, never on the work goroutine. The
// extract/work split enforces this: extract runs on-loop and hands the
// worker plain Go values.
//
// This test drives eight in-flight workers (Promise.all) whose options
// object has a JS getter — so reading `n` executes JS — while the event
// loop stays busy with a 0ms setInterval allocating objects. Run under
// `go test -race`: an off-loop VM access in the worker (the pre-split
// idiom, where work received the raw goja.FunctionCall) races with the
// interval callbacks on the loop; the extract/work split must be clean.
func TestPromisifyAsync_WorkerMustNotExecuteVMCode(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterFactory("slowOp", func(vm *goja.Runtime, loop *eventloop.EventLoop) any {
		return scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (int64, error) {
				// On-loop: executing the JS getter here is safe.
				opts := call.Argument(0).ToObject(vm)
				return opts.Get("n").ToInteger(), nil
			},
			func(ctx context.Context, n int64) (int64, error) {
				time.Sleep(time.Millisecond)
				return n * 2, nil
			})
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "race.ts", `
let ticks = 0;
const timer = setInterval(() => {
  ticks++;
  const o = { a: Math.random(), s: "tick" + ticks };
  void o;
}, 0);
const mk = (i: number) => ({ get n() { return i; } });
const res = await Promise.all([1, 2, 3, 4, 5, 6, 7, 8].map((i) => slowOp(mk(i))));
clearInterval(timer);
if (res.some((v, i) => v !== (i + 1) * 2)) throw new Error("bad results: " + res);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// An error from extract rejects the Promise — observable in JS exactly like
// a work error, so argument-validation failures stay catchable.
func TestPromisifyAsync_ExtractErrorRejects(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterFactory("needsArg", func(vm *goja.Runtime, loop *eventloop.EventLoop) any {
		return scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (string, error) {
				arg := call.Argument(0)
				if goja.IsUndefined(arg) || goja.IsNull(arg) {
					return "", errArgRequired
				}
				return arg.String(), nil
			},
			func(ctx context.Context, s string) (string, error) {
				return s, nil
			})
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "reject.ts", `
let caught = "";
try {
  await needsArg();
} catch (e) {
  caught = String(e);
}
if (!caught.includes("arg required")) throw new Error("expected rejection, got: " + caught);
const ok = await needsArg("fine");
if (ok !== "fine") throw new Error("expected passthrough, got " + ok);
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

var errArgRequired = errArg{}

type errArg struct{}

func (errArg) Error() string { return "arg required" }
