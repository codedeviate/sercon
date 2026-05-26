package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func TestBuildAIArgv(t *testing.T) {
	cases := []struct {
		provider string
		want     []string
	}{
		{"claude", []string{"claude", "-p", "hi"}},
		{"codex", []string{"codex", "exec", "hi"}},
		{"copilot", []string{"copilot", "-p", "hi"}},
		{"gemini", []string{"gemini", "-p", "hi"}},
	}
	for _, tc := range cases {
		got, err := buildAIArgv(tc.provider, "hi")
		if err != nil {
			t.Fatalf("%s: %v", tc.provider, err)
		}
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s argv: %v, want %v", tc.provider, got, tc.want)
		}
	}
	if _, err := buildAIArgv("bogus", "hi"); err == nil {
		t.Error("unknown provider should error")
	}
}

func TestBuildAIPrompt(t *testing.T) {
	got := buildAIPrompt("be terse", "the repo is sercon", "what is it?")
	want := "System: be terse\n\nContext: the repo is sercon\n\nwhat is it?"
	if got != want {
		t.Errorf("prompt:\n%q\nwant\n%q", got, want)
	}
	// prompt-only (no system/context).
	if got := buildAIPrompt("", "", "just this"); got != "just this" {
		t.Errorf("prompt-only: %q", got)
	}
}

// send() against a fake provider: put an executable named "claude" on
// PATH that echoes its last arg, and confirm the full exec path works.
func TestAISend_FakeProvider(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "claude")
	// Echo arg $2 (the prompt after -p) so we can assert it round-trips.
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"[fake] $2\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 5 * time.Second})
	if err := eng.RegisterNamespaceFactory("ai", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return aiNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) { captured = v.Export() }); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "a.ts", `
		const r = await ai.send({ prompt: "hello", provider: "claude" });
		__capture([r.provider, r.exitCode, r.output].join("|"));
	`); err != nil {
		t.Fatalf("script: %v", err)
	}
	got, _ := captured.(string)
	if got != "claude|0|[fake] hello" {
		t.Errorf("send: %q", got)
	}
}

func TestAISend_NoProviderThrows(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("ai", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return aiNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await ai.send({ prompt: "hi" });`)
	if err == nil {
		t.Fatal("expected no-provider error")
	}
	if !strings.Contains(err.Error(), "no AI provider") {
		t.Errorf("expected no-provider message; got %v", err)
	}
}

func TestAISend_MissingPromptThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("ai", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return aiNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "x.ts", `await ai.send({});`); err == nil {
		t.Error("missing prompt should throw")
	}
}

func TestAIProviders_DetectsFake(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"claude", "gemini"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	got := detectAIProviders()
	want := []string{"claude", "gemini"} // preference order
	if !reflect.DeepEqual(got, want) {
		t.Errorf("providers: %v, want %v", got, want)
	}
}
