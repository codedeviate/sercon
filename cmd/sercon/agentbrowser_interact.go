package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// exportStr coerces a goja.Value export to a string, returning "" for
// nil/undefined/null exports so callers never see a bare "<nil>".
func exportStr(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	ex := v.Export()
	if ex == nil {
		return ""
	}
	return fmt.Sprintf("%v", ex)
}

// anyStr coerces an arbitrary exported value to a string, returning "" for nil.
func anyStr(f any) string {
	if f == nil {
		return ""
	}
	return fmt.Sprintf("%v", f)
}

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
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, interactArgs(verb, operands...)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// selectorVerb returns a work half that runs `verb <sel>`, erroring on a
// missing selector. Used for click/dblclick/hover/focus/check/uncheck/
// scrollIntoView.
func (h *abHandle) selectorVerb(verb string) func(context.Context, string) (any, error) {
	return func(ctx context.Context, sel string) (any, error) {
		if sel == "" {
			return nil, fmt.Errorf("agentBrowser.%s: selector is required", verb)
		}
		return h.runVerb(ctx, verb, sel)
	}
}

// selTextArgs carries selector + text for the fill/type verbs.
type selTextArgs struct {
	sel, text string
}

func selTextExtract(call goja.FunctionCall) (selTextArgs, error) {
	return selTextArgs{sel: strArg(call, 0), text: strArg(call, 1)}, nil
}

// selectorTextVerb returns a work half that runs `verb <sel> <text>`. Used
// for fill and type.
func (h *abHandle) selectorTextVerb(verb string) func(context.Context, selTextArgs) (any, error) {
	return func(ctx context.Context, a selTextArgs) (any, error) {
		if a.sel == "" {
			return nil, fmt.Errorf("agentBrowser.%s: selector is required", verb)
		}
		return h.runVerb(ctx, verb, a.sel, a.text)
	}
}

func (h *abHandle) press(ctx context.Context, key string) (any, error) {
	if key == "" {
		return nil, errors.New("agentBrowser.press: key is required")
	}
	return h.runVerb(ctx, "press", key)
}

// selectOptExtract collects the selector and every value as strings.
func selectOptExtract(call goja.FunctionCall) ([]string, error) {
	ops := make([]string, 0, len(call.Arguments))
	for i := 0; i < len(call.Arguments); i++ {
		ops = append(ops, strArg(call, i))
	}
	return ops, nil
}

// selectOpt runs `select <sel> <val...>` for dropdowns.
func (h *abHandle) selectOpt(ctx context.Context, ops []string) (any, error) {
	if len(ops) == 0 || ops[0] == "" {
		return nil, errors.New("agentBrowser.select: selector is required")
	}
	return h.runVerb(ctx, "select", ops...)
}

// scrollArgs carries the direction plus the optional pixel amount.
type scrollArgs struct {
	dir   string
	px    string
	hasPx bool
}

func scrollExtract(call goja.FunctionCall) (scrollArgs, error) {
	a := scrollArgs{dir: strArg(call, 0)}
	if px := call.Argument(1); px != nil && !goja.IsUndefined(px) {
		a.px, a.hasPx = fmt.Sprintf("%v", px.Export()), true
	}
	return a, nil
}

// scroll runs `scroll <dir> [px]`.
func (h *abHandle) scroll(ctx context.Context, a scrollArgs) (any, error) {
	if a.dir == "" {
		return nil, errors.New("agentBrowser.scroll: direction is required (up/down/left/right)")
	}
	if a.hasPx {
		return h.runVerb(ctx, "scroll", a.dir, a.px)
	}
	return h.runVerb(ctx, "scroll", a.dir)
}

// dragArgs carries source + destination selectors.
type dragArgs struct {
	src, dst string
}

func dragExtract(call goja.FunctionCall) (dragArgs, error) {
	return dragArgs{src: strArg(call, 0), dst: strArg(call, 1)}, nil
}

func (h *abHandle) drag(ctx context.Context, a dragArgs) (any, error) {
	if a.src == "" || a.dst == "" {
		return nil, errors.New("agentBrowser.drag: source and destination selectors are required")
	}
	return h.runVerb(ctx, "drag", a.src, a.dst)
}

// uploadArgs carries the selector plus the file path list.
type uploadArgs struct {
	sel   string
	files []string
}

func uploadExtract(call goja.FunctionCall) (uploadArgs, error) {
	a := uploadArgs{sel: strArg(call, 0)}
	// files may be a single string or a string[]; elements are assumed to be string paths.
	if arr, ok := call.Argument(1).Export().([]any); ok {
		for _, f := range arr {
			a.files = append(a.files, anyStr(f))
		}
	} else {
		a.files = append(a.files, strArg(call, 1))
	}
	return a, nil
}

func (h *abHandle) upload(ctx context.Context, a uploadArgs) (any, error) {
	if a.sel == "" {
		return nil, errors.New("agentBrowser.upload: selector is required")
	}
	return h.runVerb(ctx, "upload", append([]string{a.sel}, a.files...)...)
}

// downloadArgs carries the selector plus the output path.
type downloadArgs struct {
	sel, path string
}

func downloadExtract(call goja.FunctionCall) (downloadArgs, error) {
	return downloadArgs{sel: strArg(call, 0), path: strArg(call, 1)}, nil
}

func (h *abHandle) download(ctx context.Context, a downloadArgs) (any, error) {
	if a.sel == "" || a.path == "" {
		return nil, errors.New("agentBrowser.download: selector and path are required")
	}
	return h.runVerb(ctx, "download", a.sel, a.path)
}

func (h *abHandle) keyboardType(ctx context.Context, text string) (any, error) {
	return h.runVerb(ctx, "keyboard", "type", text)
}
func (h *abHandle) keyboardInsert(ctx context.Context, text string) (any, error) {
	return h.runVerb(ctx, "keyboard", "inserttext", text)
}

// mouseExtract stringifies every argument via exportStr (Export + %v, unlike
// strArg's String()) so mouse coordinates keep their original rendering.
func mouseExtract(call goja.FunctionCall) ([]string, error) {
	ops := make([]string, 0, len(call.Arguments))
	for i := 0; i < len(call.Arguments); i++ {
		ops = append(ops, exportStr(call.Argument(i)))
	}
	return ops, nil
}

func (h *abHandle) mouseAction(action string) func(context.Context, []string) (any, error) {
	return func(ctx context.Context, args []string) (any, error) {
		return h.runVerb(ctx, "mouse", append([]string{action}, args...)...)
	}
}

// addInteract wires the interaction surface into the handle object.
func (h *abHandle) addInteract(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	for _, v := range []string{"click", "dblclick", "hover", "focus", "check", "uncheck"} {
		obj[v] = abAsync(vm, loop, abStrArg0, h.selectorVerb(v))
	}
	obj["scrollIntoView"] = abAsync(vm, loop, abStrArg0, h.selectorVerb("scrollintoview"))
	obj["fill"] = abAsync(vm, loop, selTextExtract, h.selectorTextVerb("fill"))
	obj["type"] = abAsync(vm, loop, selTextExtract, h.selectorTextVerb("type"))
	obj["press"] = abAsync(vm, loop, abStrArg0, h.press)
	obj["select"] = abAsync(vm, loop, selectOptExtract, h.selectOpt)
	obj["scroll"] = abAsync(vm, loop, scrollExtract, h.scroll)
	obj["drag"] = abAsync(vm, loop, dragExtract, h.drag)
	obj["upload"] = abAsync(vm, loop, uploadExtract, h.upload)
	obj["download"] = abAsync(vm, loop, downloadExtract, h.download)
	obj["keyboard"] = map[string]any{
		"type":       abAsync(vm, loop, abStrArg0, h.keyboardType),
		"insertText": abAsync(vm, loop, abStrArg0, h.keyboardInsert),
	}
	obj["mouse"] = map[string]any{
		"move":  abAsync(vm, loop, mouseExtract, h.mouseAction("move")),
		"down":  abAsync(vm, loop, mouseExtract, h.mouseAction("down")),
		"up":    abAsync(vm, loop, mouseExtract, h.mouseAction("up")),
		"wheel": abAsync(vm, loop, mouseExtract, h.mouseAction("wheel")),
	}
}
