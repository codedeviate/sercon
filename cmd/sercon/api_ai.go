package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// aiProviders is the ordered list of supported AI CLIs. `send` with
// no explicit provider auto-detects by walking this list and using
// the first one on PATH — claude first because it's the project's
// own tool. Each provider runs in a non-interactive "print" mode and
// takes the prompt as a single argument.
var aiProviders = []string{"claude", "codex", "copilot", "gemini"}

// aiNamespace wires `api.tools.ai.*`. Two members:
//
//   - `providers()` — which of the supported CLIs are on PATH.
//   - `send(opts)` — run a one-shot prompt through a provider and
//     return its output.
//
// This is the options-object shape rather than the rhai-style builder
// chain (`.system().prompt().send()`): an options object is the
// idiomatic JS equivalent and avoids threading a mutable builder
// handle through goja. `opts.system` / `opts.context` are prepended
// to the prompt (uniform across providers, which each have different
// system-prompt flags).
//
// Library: `os/exec` (stdlib). The argv builder is a pure function so
// it's unit-testable without the CLIs installed; the demo gracefully
// degrades when no provider is present.
func aiNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"providers": func() []string { return detectAIProviders() },
		"send":      scriptengine.PromisifyAsync(vm, loop, aiSend),
	}
}

// detectAIProviders returns the supported CLIs found on PATH, in
// preference order.
func detectAIProviders() []string {
	var found []string
	for _, p := range aiProviders {
		if _, err := exec.LookPath(p); err == nil {
			found = append(found, p)
		}
	}
	if found == nil {
		found = []string{}
	}
	return found
}

// aiSend runs a one-shot prompt. opts:
//
//	{ prompt: string (required), provider?: string, system?: string,
//	  context?: string, timeout?: number }
//
// Returns `{ provider, output, exitCode }`. A non-zero exit doesn't
// throw (the model CLI may exit non-zero with a useful message on
// stdout/stderr); spawn failure (no provider on PATH) and context
// deadline throw.
func aiSend(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	opts := optsAsMap(call) // opts is the 2nd positional? No — see below.
	// aiSend is called as send(opts), so opts is the FIRST argument.
	// optsAsMap reads index 1; read index 0 directly instead.
	if m, ok := firstArgMap(call); ok {
		opts = m
	}
	prompt := optString(opts, "prompt", "")
	if prompt == "" {
		return nil, errors.New("ai.send: opts.prompt required")
	}
	provider := optString(opts, "provider", "")
	if provider == "" {
		avail := detectAIProviders()
		if len(avail) == 0 {
			return nil, errors.New("ai.send: no AI provider on PATH (looked for claude / codex / copilot / gemini)")
		}
		provider = avail[0]
	}
	timeout := optMillis(opts, "timeout", 120*time.Second)

	full := buildAIPrompt(optString(opts, "system", ""), optString(opts, "context", ""), prompt)
	argv, err := buildAIArgv(provider, full)
	if err != nil {
		return nil, fmt.Errorf("ai.send: %w", err)
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, argv[0], argv[1:]...) //nolint:gosec // provider + prompt are intentional
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runErr != nil {
		if ctxErr := runCtx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("ai.send: %w", ctxErr)
		}
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			// Non-zero exit: surface output + code rather than throwing.
			out := stdout.String()
			if out == "" {
				out = stderr.String()
			}
			return map[string]any{"provider": provider, "output": strings.TrimRight(out, "\n"), "exitCode": exitErr.ExitCode()}, nil
		}
		return nil, fmt.Errorf("ai.send: %s: %w", provider, runErr)
	}
	return map[string]any{
		"provider": provider,
		"output":   strings.TrimRight(stdout.String(), "\n"),
		"exitCode": 0,
	}, nil
}

// buildAIPrompt folds system + context + prompt into one text blob.
// Providers each have different system-prompt flags, so prepending is
// the portable common denominator. Empty sections are skipped.
func buildAIPrompt(system, context, prompt string) string {
	var b strings.Builder
	if system != "" {
		b.WriteString("System: ")
		b.WriteString(system)
		b.WriteString("\n\n")
	}
	if context != "" {
		b.WriteString("Context: ")
		b.WriteString(context)
		b.WriteString("\n\n")
	}
	b.WriteString(prompt)
	return b.String()
}

// buildAIArgv maps a provider name + prompt to its non-interactive
// invocation. The flags are each CLI's "print / one-shot" mode.
// A pure function so the mapping is unit-testable without the CLIs.
func buildAIArgv(provider, prompt string) ([]string, error) {
	switch provider {
	case "claude":
		return []string{"claude", "-p", prompt}, nil
	case "codex":
		return []string{"codex", "exec", prompt}, nil
	case "copilot":
		return []string{"copilot", "-p", prompt}, nil
	case "gemini":
		return []string{"gemini", "-p", prompt}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: %s)", provider, strings.Join(aiProviders, ", "))
	}
}

// firstArgMap reads the first positional argument as a map (the opts
// object for send). Returns (nil, false) when it's absent or not an
// object.
func firstArgMap(call goja.FunctionCall) (map[string]any, bool) {
	if len(call.Arguments) < 1 {
		return nil, false
	}
	arg := call.Argument(0)
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return nil, false
	}
	m, ok := arg.Export().(map[string]any)
	return m, ok
}
