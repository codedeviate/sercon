package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// interactArgs is the generic verb+operands assembler (mirrors navArgs but
// kept separate so the two surfaces evolve independently).
func interactArgs(verb string, operands ...string) []string {
	return append([]string{verb}, operands...)
}

// runVerb runs an interaction verb and returns the parsed JSON result.
func (h *abHandle) runVerb(ctx context.Context, verb string, operands ...string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, interactArgs(verb, operands...)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// selectorVerb returns a method that runs `verb <sel>`, erroring on a
// missing selector. Used for click/dblclick/hover/focus/check/uncheck/
// scrollIntoView.
func (h *abHandle) selectorVerb(verb string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		sel := strArg(call, 0)
		if sel == "" {
			return nil, fmt.Errorf("agentBrowser.%s: selector is required", verb)
		}
		return h.runVerb(ctx, verb, sel)
	}
}

// selectorTextVerb returns a method that runs `verb <sel> <text>`. Used for
// fill and type.
func (h *abHandle) selectorTextVerb(verb string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		sel := strArg(call, 0)
		if sel == "" {
			return nil, fmt.Errorf("agentBrowser.%s: selector is required", verb)
		}
		return h.runVerb(ctx, verb, sel, strArg(call, 1))
	}
}

func (h *abHandle) press(ctx context.Context, call goja.FunctionCall) (any, error) {
	key := strArg(call, 0)
	if key == "" {
		return nil, errors.New("agentBrowser.press: key is required")
	}
	return h.runVerb(ctx, "press", key)
}

// selectOpt runs `select <sel> <val...>` for dropdowns.
func (h *abHandle) selectOpt(ctx context.Context, call goja.FunctionCall) (any, error) {
	sel := strArg(call, 0)
	if sel == "" {
		return nil, errors.New("agentBrowser.select: selector is required")
	}
	ops := []string{sel}
	for i := 1; i < len(call.Arguments); i++ {
		ops = append(ops, strArg(call, i))
	}
	return h.runVerb(ctx, "select", ops...)
}

// scroll runs `scroll <dir> [px]`.
func (h *abHandle) scroll(ctx context.Context, call goja.FunctionCall) (any, error) {
	dir := strArg(call, 0)
	if dir == "" {
		return nil, errors.New("agentBrowser.scroll: direction is required (up/down/left/right)")
	}
	if px := call.Argument(1); px != nil && !goja.IsUndefined(px) {
		return h.runVerb(ctx, "scroll", dir, fmt.Sprintf("%v", px.Export()))
	}
	return h.runVerb(ctx, "scroll", dir)
}

func (h *abHandle) drag(ctx context.Context, call goja.FunctionCall) (any, error) {
	src, dst := strArg(call, 0), strArg(call, 1)
	if src == "" || dst == "" {
		return nil, errors.New("agentBrowser.drag: source and destination selectors are required")
	}
	return h.runVerb(ctx, "drag", src, dst)
}

func (h *abHandle) upload(ctx context.Context, call goja.FunctionCall) (any, error) {
	sel := strArg(call, 0)
	if sel == "" {
		return nil, errors.New("agentBrowser.upload: selector is required")
	}
	ops := []string{sel}
	// files may be a single string or a string[].
	if arr, ok := call.Argument(1).Export().([]any); ok {
		for _, f := range arr {
			ops = append(ops, fmt.Sprintf("%v", f))
		}
	} else {
		ops = append(ops, strArg(call, 1))
	}
	return h.runVerb(ctx, "upload", ops...)
}

func (h *abHandle) download(ctx context.Context, call goja.FunctionCall) (any, error) {
	sel, path := strArg(call, 0), strArg(call, 1)
	if sel == "" || path == "" {
		return nil, errors.New("agentBrowser.download: selector and path are required")
	}
	return h.runVerb(ctx, "download", sel, path)
}

func (h *abHandle) keyboardType(ctx context.Context, call goja.FunctionCall) (any, error) {
	return h.runVerb(ctx, "keyboard", "type", strArg(call, 0))
}
func (h *abHandle) keyboardInsert(ctx context.Context, call goja.FunctionCall) (any, error) {
	return h.runVerb(ctx, "keyboard", "inserttext", strArg(call, 0))
}

func (h *abHandle) mouseAction(action string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		ops := []string{action}
		for i := 0; i < len(call.Arguments); i++ {
			ops = append(ops, fmt.Sprintf("%v", call.Argument(i).Export()))
		}
		return h.runVerb(ctx, "mouse", ops...)
	}
}

// addInteract wires the interaction surface into the handle object.
func (h *abHandle) addInteract(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	for _, v := range []string{"click", "dblclick", "hover", "focus", "check", "uncheck"} {
		obj[v] = h.p(vm, loop, h.selectorVerb(v))
	}
	obj["scrollIntoView"] = h.p(vm, loop, h.selectorVerb("scrollintoview"))
	obj["fill"] = h.p(vm, loop, h.selectorTextVerb("fill"))
	obj["type"] = h.p(vm, loop, h.selectorTextVerb("type"))
	obj["press"] = h.p(vm, loop, h.press)
	obj["select"] = h.p(vm, loop, h.selectOpt)
	obj["scroll"] = h.p(vm, loop, h.scroll)
	obj["drag"] = h.p(vm, loop, h.drag)
	obj["upload"] = h.p(vm, loop, h.upload)
	obj["download"] = h.p(vm, loop, h.download)
	obj["keyboard"] = map[string]any{
		"type":       h.p(vm, loop, h.keyboardType),
		"insertText": h.p(vm, loop, h.keyboardInsert),
	}
	obj["mouse"] = map[string]any{
		"move":  h.p(vm, loop, h.mouseAction("move")),
		"down":  h.p(vm, loop, h.mouseAction("down")),
		"up":    h.p(vm, loop, h.mouseAction("up")),
		"wheel": h.p(vm, loop, h.mouseAction("wheel")),
	}
}
