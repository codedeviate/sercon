package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// titleState upper-cases the first rune of s (e.g. "visible" -> "Visible").
// strings.Title is deprecated; this avoids that dependency.
func titleState(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// snapshotArgs builds the snapshot command flags from an options object.
func snapshotArgs(opts map[string]any) []string {
	args := []string{"snapshot"}
	if b, _ := opts["interactive"].(bool); b {
		args = append(args, "-i")
	}
	if b, _ := opts["compact"].(bool); b {
		args = append(args, "-c")
	}
	if d, ok := opts["depth"]; ok {
		args = append(args, "-d", fmt.Sprintf("%v", numToInt(d)))
	}
	if s, _ := opts["selector"].(string); s != "" {
		args = append(args, "-s", s)
	}
	return args
}

// numToInt coerces a JS number (float64 via goja Export) to an int.
func numToInt(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// optsArgMap extracts the argument at index i as a map[string]any.
// Returns an empty map when the argument is absent, undefined, or not an object.
func optsArgMap(call goja.FunctionCall, i int) map[string]any {
	if m, ok := call.Argument(i).Export().(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// getArgs carries the `get` subject plus the optional selector.
type getArgs struct {
	what, sel string
}

func getExtract(call goja.FunctionCall) (getArgs, error) {
	return getArgs{what: strArg(call, 0), sel: strArg(call, 1)}, nil
}

// get runs `get <what> [selector]`.
func (h *abHandle) get(ctx context.Context, a getArgs) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	if a.what == "" {
		return nil, errors.New("agentBrowser.get: what is required (text/html/value/attr/title/url/count/box/styles/cdp-url)")
	}
	args := []string{"get", a.what}
	if a.sel != "" {
		args = append(args, a.sel)
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, args...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// isState returns a work half that runs `is <state> <selector>`.
// Used for isVisible / isEnabled / isChecked.
func (h *abHandle) isState(state string) func(context.Context, string) (any, error) {
	return func(ctx context.Context, sel string) (any, error) {
		if err := h.requireOpen(); err != nil {
			return nil, err
		}
		if sel == "" {
			return nil, fmt.Errorf("agentBrowser.is%s: selector is required", titleState(state))
		}
		out, err := abRunChecked(ctx, h.session, h.global, h.timeout, "is", state, sel)
		if err != nil {
			return nil, err
		}
		return parseJSON(out)
	}
}

// evalJS runs `eval <code>` in the page context.
func (h *abHandle) evalJS(ctx context.Context, code string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	if code == "" {
		return nil, errors.New("agentBrowser.eval: js code is required")
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, "eval", code)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// snapshot captures the accessibility tree / DOM snapshot.
func (h *abHandle) snapshot(ctx context.Context, opts map[string]any) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, snapshotArgs(opts)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// logView returns a work half that runs `console` or `errors`, honouring {clear:true}.
func (h *abHandle) logView(verb string) func(context.Context, map[string]any) (any, error) {
	return func(ctx context.Context, opts map[string]any) (any, error) {
		if err := h.requireOpen(); err != nil {
			return nil, err
		}
		args := []string{verb}
		if b, _ := opts["clear"].(bool); b {
			args = append(args, "--clear")
		}
		out, err := abRunChecked(ctx, h.session, h.global, h.timeout, args...)
		if err != nil {
			return nil, err
		}
		return parseJSON(out)
	}
}

// highlight highlights the matched element(s) in the browser.
func (h *abHandle) highlight(ctx context.Context, sel string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	if sel == "" {
		return nil, errors.New("agentBrowser.highlight: selector is required")
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, "highlight", sel)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// addInspect wires the inspection surface into the handle object.
func (h *abHandle) addInspect(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["get"] = abAsync(vm, loop, getExtract, h.get)
	obj["isVisible"] = abAsync(vm, loop, abStrArg0, h.isState("visible"))
	obj["isEnabled"] = abAsync(vm, loop, abStrArg0, h.isState("enabled"))
	obj["isChecked"] = abAsync(vm, loop, abStrArg0, h.isState("checked"))
	obj["eval"] = abAsync(vm, loop, abStrArg0, h.evalJS)
	obj["snapshot"] = abAsync(vm, loop, abOptsArg0, h.snapshot)
	obj["console"] = abAsync(vm, loop, abOptsArg0, h.logView("console"))
	obj["errors"] = abAsync(vm, loop, abOptsArg0, h.logView("errors"))
	obj["highlight"] = abAsync(vm, loop, abStrArg0, h.highlight)
}
