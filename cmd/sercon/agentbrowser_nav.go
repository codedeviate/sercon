package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// navArgs assembles CLI args for a navigation verb plus optional operands.
func navArgs(verb string, operands ...string) []string {
	return append([]string{verb}, operands...)
}

// strArg coerces a goja argument to a trimmed string; "" if undefined/null.
func strArg(call goja.FunctionCall, i int) string {
	v := call.Argument(i)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}

// runNav runs a navigation verb against the handle and returns the parsed
// JSON result object.
func (h *abHandle) runNav(ctx context.Context, verb string, operands ...string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, navArgs(verb, operands...)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

func (h *abHandle) open(ctx context.Context, call goja.FunctionCall) (any, error) {
	url := strArg(call, 0)
	if url == "" {
		return nil, errors.New("agentBrowser.open: url is required")
	}
	return h.runNav(ctx, "open", url)
}

func (h *abHandle) back(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runNav(ctx, "back")
}
func (h *abHandle) forward(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runNav(ctx, "forward")
}
func (h *abHandle) reload(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runNav(ctx, "reload")
}

// wait accepts a selector string or a number of milliseconds.
func (h *abHandle) wait(ctx context.Context, call goja.FunctionCall) (any, error) {
	arg := call.Argument(0)
	if arg == nil || goja.IsUndefined(arg) {
		return nil, errors.New("agentBrowser.wait: selector or ms required")
	}
	// goja numbers stringify cleanly (e.g. 500 -> "500").
	return h.runNav(ctx, "wait", fmt.Sprintf("%v", arg.Export()))
}

func (h *abHandle) connect(ctx context.Context, call goja.FunctionCall) (any, error) {
	target := strArg(call, 0)
	if target == "" {
		return nil, errors.New("agentBrowser.connect: port or url required")
	}
	return h.runNav(ctx, "connect", target)
}

// addNav wires the navigation methods into the handle object.
func (h *abHandle) addNav(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["open"] = h.p(vm, loop, h.open)
	obj["back"] = h.p(vm, loop, h.back)
	obj["forward"] = h.p(vm, loop, h.forward)
	obj["reload"] = h.p(vm, loop, h.reload)
	obj["wait"] = h.p(vm, loop, h.wait)
	obj["connect"] = h.p(vm, loop, h.connect)
}
