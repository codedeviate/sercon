package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runCloudScript registers the real cloud namespace and runs body, returning
// the value passed to __capture(__result).
func runCloudScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("cloud", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return cloudNamespace(vm, loop)
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
	if _, err := eng.Run(context.Background(), "c.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

func TestCloudGoogle_HandleShape(t *testing.T) {
	got := runCloudScript(t, `
		const g = cloud.google({ project: "p" });
		const __result = {
			isFn: typeof cloud.google === "function",
			storage: typeof g.storage,
			compute: typeof g.compute,
			iam: typeof g.iam,
			secrets: typeof g.secrets,
			call: typeof g.call,
		};
	`)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected object, got %T", got)
	}
	for _, k := range []string{"storage", "compute", "iam", "secrets", "call"} {
		if m[k] != "function" {
			t.Fatalf("expected g.%s to be a function, got %v", k, m[k])
		}
	}
	if m["isFn"] != true {
		t.Fatal("cloud.google must be callable")
	}
}

func TestCloudGoogle_RejectsNonObjectConfig(t *testing.T) {
	got := runCloudScript(t, `
		let msg = "";
		try { cloud.google([1,2,3]); } catch (e) { msg = e.message; }
		const __result = { msg };
	`)
	m := got.(map[string]any)
	if s, _ := m["msg"].(string); s == "" || !strings.Contains(s, "options must be an object") {
		t.Fatalf("expected a catchable 'options must be an object' error, got %q", s)
	}
}
