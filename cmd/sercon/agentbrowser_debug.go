package main

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// profilerStartArgs builds `profiler start [--categories <csv>]`.
func profilerStartArgs(opts map[string]any) []string {
	args := []string{"profiler", "start"}
	if c, _ := opts["categories"].(string); c != "" {
		args = append(args, "--categories", c)
	}
	return args
}

// clipboardArgs builds `clipboard <op> [text]`.
func clipboardArgs(op, text string) []string {
	args := []string{"clipboard", op}
	if text != "" {
		args = append(args, text)
	}
	return args
}

// startStop returns a work half running `<verb> <op> [path]` for a
// save-on-stop recorder (trace, profiler). op is "start" or "stop"; the
// extracted path is only used on "stop".
func (h *abHandle) startStop(verb, op string) func(context.Context, string) (any, error) {
	return func(ctx context.Context, p string) (any, error) {
		args := []string{verb, op}
		if op == "stop" && p != "" {
			args = append(args, p)
		}
		return h.runJSON(ctx, args...)
	}
}

func (h *abHandle) profilerStart(ctx context.Context, opts map[string]any) (any, error) {
	return h.runJSON(ctx, profilerStartArgs(opts)...)
}

func (h *abHandle) inspect(ctx context.Context, _ struct{}) (any, error) {
	return h.runJSON(ctx, "inspect")
}

// clipboardParams carries the clipboard operation plus its optional text.
type clipboardParams struct {
	op, text string
}

func clipboardExtract(call goja.FunctionCall) (clipboardParams, error) {
	return clipboardParams{op: strArg(call, 0), text: strArg(call, 1)}, nil
}

func (h *abHandle) clipboard(ctx context.Context, p clipboardParams) (any, error) {
	if p.op == "" {
		return nil, errors.New("agentBrowser.clipboard: operation is required (read/write/copy/paste)")
	}
	return h.runJSON(ctx, clipboardArgs(p.op, p.text)...)
}

func (h *abHandle) vitals(ctx context.Context, url string) (any, error) {
	args := []string{"vitals"}
	if url != "" {
		args = append(args, url)
	}
	return h.runJSON(ctx, args...)
}

func (h *abHandle) pushstate(ctx context.Context, url string) (any, error) {
	if url == "" {
		return nil, errors.New("agentBrowser.pushstate: url is required")
	}
	return h.runJSON(ctx, "pushstate", url)
}

// addDebug wires the debug/perf surface into the handle object.
func (h *abHandle) addDebug(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["trace"] = map[string]any{
		"start": abAsync(vm, loop, abStrArg0, h.startStop("trace", "start")),
		"stop":  abAsync(vm, loop, abStrArg0, h.startStop("trace", "stop")),
	}
	obj["profiler"] = map[string]any{
		"start": abAsync(vm, loop, abOptsArg0, h.profilerStart),
		"stop":  abAsync(vm, loop, abStrArg0, h.startStop("profiler", "stop")),
	}
	obj["inspect"] = abAsync(vm, loop, abNoArgs, h.inspect)
	obj["clipboard"] = abAsync(vm, loop, clipboardExtract, h.clipboard)
	obj["vitals"] = abAsync(vm, loop, abStrArg0, h.vitals)
	obj["pushstate"] = abAsync(vm, loop, abStrArg0, h.pushstate)
}
