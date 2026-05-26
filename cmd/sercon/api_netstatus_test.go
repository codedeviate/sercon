package main

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func runNetstatusScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("netstatus", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return netstatusNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "n.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// Against localhost with nothing listening on 443: dns resolves
// (localhost always does), tcp fails → not reachable. The sub-probe
// failures are data, not a thrown error.
func TestNetstatus_UnreachableIsData(t *testing.T) {
	got := runNetstatusScript(t, `
		const s = await netstatus.check("127.0.0.1", { port: "1", timeout: 1000 });
		const __result = [s.reachable, s.tcp.ok, typeof s.tcp.error].join(",");
	`)
	if got != "false,false,string" {
		t.Errorf("unreachable: %v (want false,false,string)", got)
	}
}

// The result always carries all four sub-probe objects regardless of
// individual outcomes.
func TestNetstatus_ShapeComplete(t *testing.T) {
	got := runNetstatusScript(t, `
		const s = await netstatus.check("127.0.0.1", { port: "1", timeout: 1000 });
		const __result = [
			"host" in s, "reachable" in s, "elapsedMs" in s,
			"dns" in s, "tcp" in s, "tls" in s, "http" in s,
		].join(",");
	`)
	if got != "true,true,true,true,true,true,true" {
		t.Errorf("shape: %v", got)
	}
}

// Missing host throws.
func TestNetstatus_MissingHostThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("netstatus", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return netstatusNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await netstatus.check("");`); err == nil {
		t.Error("empty host should throw")
	}
}
