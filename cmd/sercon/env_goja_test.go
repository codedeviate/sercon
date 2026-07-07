// cmd/sercon/env_goja_test.go
package main

import (
	"os"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// envLoadVM builds a goja runtime exposing a minimal runtime.env with get+load,
// mirroring the main.go wiring, so the async load binding can be exercised.
func envLoadVM(t *testing.T, vm *goja.Runtime, loop *eventloop.EventLoop) {
	t.Helper()
	env := vm.NewObject()
	_ = env.Set("get", func(call goja.FunctionCall) goja.Value {
		if v, ok := os.LookupEnv(call.Argument(0).String()); ok {
			return vm.ToValue(v)
		}
		return goja.Undefined()
	})
	_ = env.Set("load", scriptengine.PromisifyAsyncLegacy(vm, loop, envLoadBinding(vm)).Func)
	rt := vm.NewObject()
	_ = rt.Set("env", env)
	_ = vm.Set("runtime", rt)
}

func TestEnvGoja_LoadApplies(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	if err := os.WriteFile(path, []byte("GOJA_LOADED=yes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loop := eventloop.NewEventLoop()
	var got string
	var rerr error
	loop.Run(func(vm *goja.Runtime) {
		envLoadVM(t, vm, loop)
		_ = vm.Set("p", path)
		_, rerr = vm.RunString(`
			runtime.env.load(p).then(m => {
				globalThis.__out = m.GOJA_LOADED + "|" + runtime.env.get("GOJA_LOADED");
			});
		`)
	})
	if rerr != nil {
		t.Fatal(rerr)
	}
	loop.Run(func(vm *goja.Runtime) {
		got = vm.Get("__out").String()
	})
	if got != "yes|yes" {
		t.Fatalf("got %q, want yes|yes", got)
	}
}
