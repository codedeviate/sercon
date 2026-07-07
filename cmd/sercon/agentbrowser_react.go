package main

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// suspenseArgs builds `react suspense [--only-dynamic]`.
func suspenseArgs(opts map[string]any) []string {
	args := []string{"react", "suspense"}
	if b, _ := opts["onlyDynamic"].(bool); b {
		args = append(args, "--only-dynamic")
	}
	return args
}

func (h *abHandle) reactTree(ctx context.Context, _ struct{}) (any, error) {
	return h.runJSON(ctx, "react", "tree")
}

func (h *abHandle) reactInspect(ctx context.Context, id string) (any, error) {
	if id == "" {
		return nil, errors.New("agentBrowser.react.inspect: a fiber id is required")
	}
	return h.runJSON(ctx, "react", "inspect", id)
}

func (h *abHandle) reactRenders(op string) func(context.Context, struct{}) (any, error) {
	return func(ctx context.Context, _ struct{}) (any, error) {
		return h.runJSON(ctx, "react", "renders", op)
	}
}

func (h *abHandle) reactSuspense(ctx context.Context, opts map[string]any) (any, error) {
	return h.runJSON(ctx, suspenseArgs(opts)...)
}

// addReact wires the React DevTools surface into the handle object. Requires
// the session launched with launch({ enable: "react-devtools" }); agent-browser
// returns a clear error (surfaced as a throw) when it was not.
func (h *abHandle) addReact(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["react"] = map[string]any{
		"tree":    abAsync(vm, loop, abNoArgs, h.reactTree),
		"inspect": abAsync(vm, loop, abStrArg0, h.reactInspect),
		"renders": map[string]any{
			"start": abAsync(vm, loop, abNoArgs, h.reactRenders("start")),
			"stop":  abAsync(vm, loop, abNoArgs, h.reactRenders("stop")),
		},
		"suspense": abAsync(vm, loop, abOptsArg0, h.reactSuspense),
	}
}
