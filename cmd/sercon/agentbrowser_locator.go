package main

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// abFindArgs builds `find <locator> <value> <action> [text]`. (ab-prefixed:
// webdriver_element.go owns the bare findArgs/findExtract names.)
func abFindArgs(locator, value, action, text string) []string {
	args := []string{"find", locator, value, action}
	if text != "" {
		args = append(args, text)
	}
	return args
}

// runFind resolves a locator and performs an action in one shot.
func (h *abHandle) runFind(ctx context.Context, locator, value, action, text string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, abFindArgs(locator, value, action, text)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// findParams carries the extracted find(locator, value, {action, text}) inputs.
type findParams struct {
	locator, value, action, text string
}

func abFindExtract(call goja.FunctionCall) (findParams, error) {
	p := findParams{locator: strArg(call, 0), value: strArg(call, 1)}
	opts := optsArgMap(call, 2)
	p.action, _ = opts["action"].(string)
	p.text, _ = opts["text"].(string)
	return p, nil
}

// find is the one-shot form: find(locator, value, {action, text}).
func (h *abHandle) find(ctx context.Context, p findParams) (any, error) {
	if p.locator == "" || p.value == "" {
		return nil, errors.New("agentBrowser.find: locator and value are required")
	}
	if p.action == "" {
		return nil, errors.New("agentBrowser.find: opts.action is required (e.g. click/fill/hover); for read-only matching use snapshot()")
	}
	return h.runFind(ctx, p.locator, p.value, p.action, p.text)
}

// addLocator wires find() and locator() into the handle object. locator(spec)
// returns a chainable handle bound to a (locator, value) spec; each method
// re-resolves the spec via `find` with the matching action. Because the CLI
// cannot hand back a reusable ref without acting, resolution is late — a
// stale match surfaces on use, not on creation.
//
// State-check actions (is-visible/is-enabled/is-checked) are NOT offered on
// the locator handle: `agent-browser find` only supports act-style verbs
// (click/fill/type/hover/focus/check/uncheck). Use the handle-level
// isVisible/isEnabled/isChecked methods for state checks instead.
func (h *abHandle) addLocator(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["find"] = abAsync(vm, loop, abFindExtract, h.find)

	// locator(spec) -> object whose action methods re-resolve via find.
	// Accepts either {locator, value} object OR positional (locator, value).
	obj["locator"] = func(call goja.FunctionCall) goja.Value {
		var loc, val string
		if m, ok := call.Argument(0).Export().(map[string]any); ok {
			loc, _ = m["locator"].(string)
			val, _ = m["value"].(string)
		} else {
			loc = strArg(call, 0)
			val = strArg(call, 1)
		}
		action := func(name string, withText bool) func(goja.FunctionCall) goja.Value {
			return abAsync(vm, loop,
				func(c goja.FunctionCall) (string, error) {
					if withText {
						return strArg(c, 0), nil
					}
					return "", nil
				},
				func(ctx context.Context, text string) (any, error) {
					if loc == "" || val == "" {
						return nil, errors.New("agentBrowser.locator: locator and value are required")
					}
					return h.runFind(ctx, loc, val, name, text)
				})
		}
		return vm.ToValue(map[string]any{
			"locator":  loc,
			"value":    val,
			"click":    action("click", false),
			"dblclick": action("dblclick", false),
			"hover":    action("hover", false),
			"focus":    action("focus", false),
			"check":    action("check", false),
			"uncheck":  action("uncheck", false),
			"fill":     action("fill", true),
			"type":     action("type", true),
		})
	}
}
