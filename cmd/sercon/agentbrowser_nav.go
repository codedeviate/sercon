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

// shiftCall returns a FunctionCall whose Arguments are call.Arguments[n:],
// so a namespace wrapper can forward trailing args to a handle method that
// expects them at position 0. Returns an empty-arg call if n is out of range.
func shiftCall(call goja.FunctionCall, n int) goja.FunctionCall {
	if n >= len(call.Arguments) {
		return goja.FunctionCall{This: call.This}
	}
	return goja.FunctionCall{This: call.This, Arguments: call.Arguments[n:]}
}

// runNav runs a navigation verb against the handle and returns the parsed
// JSON result object.
func (h *abHandle) runNav(ctx context.Context, verb string, operands ...string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, navArgs(verb, operands...)...)
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
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
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

// frame switches the session's frame context to an iframe (by CSS selector or
// @ref) or back to the top document with "main". Subsequent click/fill/find/
// snapshot operate inside that frame. Nest by calling sequentially; cross-origin
// frames work (CDP frame switch). Backed by `agent-browser frame <target>`.
func (h *abHandle) frame(ctx context.Context, call goja.FunctionCall) (any, error) {
	target := strArg(call, 0)
	if target == "" {
		return nil, errors.New("agentBrowser.frame: target required (a CSS selector, an @ref, or \"main\")")
	}
	return h.runNav(ctx, "frame", target)
}

// addNav wires the navigation methods into the handle object.
func (h *abHandle) addNav(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["open"] = h.p(vm, loop, h.open)
	obj["back"] = h.p(vm, loop, h.back)
	obj["forward"] = h.p(vm, loop, h.forward)
	obj["reload"] = h.p(vm, loop, h.reload)
	obj["wait"] = h.p(vm, loop, h.wait)
	obj["connect"] = h.p(vm, loop, h.connect)
	obj["frame"] = h.p(vm, loop, h.frame)
}
