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

// startStop returns a method running `<verb> <op> [path]` for a save-on-stop
// recorder (trace, profiler). op is "start" or "stop".
func (h *abHandle) startStop(verb, op string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		args := []string{verb, op}
		if op == "stop" {
			if p := strArg(call, 0); p != "" {
				args = append(args, p)
			}
		}
		return h.runJSON(ctx, args...)
	}
}

func (h *abHandle) profilerStart(ctx context.Context, call goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, profilerStartArgs(optsArgMap(call, 0))...)
}

func (h *abHandle) inspect(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, "inspect")
}

func (h *abHandle) clipboard(ctx context.Context, call goja.FunctionCall) (any, error) {
	op := strArg(call, 0)
	if op == "" {
		return nil, errors.New("agentBrowser.clipboard: operation is required (read/write/copy/paste)")
	}
	return h.runJSON(ctx, clipboardArgs(op, strArg(call, 1))...)
}

func (h *abHandle) vitals(ctx context.Context, call goja.FunctionCall) (any, error) {
	args := []string{"vitals"}
	if url := strArg(call, 0); url != "" {
		args = append(args, url)
	}
	return h.runJSON(ctx, args...)
}

func (h *abHandle) pushstate(ctx context.Context, call goja.FunctionCall) (any, error) {
	url := strArg(call, 0)
	if url == "" {
		return nil, errors.New("agentBrowser.pushstate: url is required")
	}
	return h.runJSON(ctx, "pushstate", url)
}

// addDebug wires the debug/perf surface into the handle object.
func (h *abHandle) addDebug(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["trace"] = map[string]any{
		"start": h.p(vm, loop, h.startStop("trace", "start")),
		"stop":  h.p(vm, loop, h.startStop("trace", "stop")),
	}
	obj["profiler"] = map[string]any{
		"start": h.p(vm, loop, h.profilerStart),
		"stop":  h.p(vm, loop, h.startStop("profiler", "stop")),
	}
	obj["inspect"] = h.p(vm, loop, h.inspect)
	obj["clipboard"] = h.p(vm, loop, h.clipboard)
	obj["vitals"] = h.p(vm, loop, h.vitals)
	obj["pushstate"] = h.p(vm, loop, h.pushstate)
}
