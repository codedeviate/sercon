package main

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// tabNewArgs builds `tab new [--label <label>] [url]`.
func tabNewArgs(url, label string) []string {
	args := []string{"tab", "new"}
	if label != "" {
		args = append(args, "--label", label)
	}
	if url != "" {
		args = append(args, url)
	}
	return args
}

func (h *abHandle) tabList(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, "tab", "list")
}

func (h *abHandle) tabNew(ctx context.Context, call goja.FunctionCall) (any, error) {
	url := strArg(call, 0)
	label, _ := optsArgMap(call, 1)["label"].(string)
	return h.runJSON(ctx, tabNewArgs(url, label)...)
}

func (h *abHandle) tabClose(ctx context.Context, call goja.FunctionCall) (any, error) {
	args := []string{"tab", "close"}
	if ref := strArg(call, 0); ref != "" {
		args = append(args, ref)
	}
	return h.runJSON(ctx, args...)
}

func (h *abHandle) tabSelect(ctx context.Context, call goja.FunctionCall) (any, error) {
	ref := strArg(call, 0)
	if ref == "" {
		return nil, errors.New("agentBrowser.tabs.select: a tab ref (t<N> or label) is required")
	}
	return h.runJSON(ctx, "tab", ref)
}

// addTabs wires tab management into the handle object.
func (h *abHandle) addTabs(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["tabs"] = map[string]any{
		"list":   h.p(vm, loop, h.tabList),
		"new":    h.p(vm, loop, h.tabNew),
		"close":  h.p(vm, loop, h.tabClose),
		"select": h.p(vm, loop, h.tabSelect),
	}
}
