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

func (h *abHandle) tabList(ctx context.Context, _ struct{}) (any, error) {
	return h.runJSON(ctx, "tab", "list")
}

// tabNewParams carries the optional url + label for `tab new`.
type tabNewParams struct {
	url, label string
}

func tabNewExtract(call goja.FunctionCall) (tabNewParams, error) {
	p := tabNewParams{url: strArg(call, 0)}
	p.label, _ = optsArgMap(call, 1)["label"].(string)
	return p, nil
}

func (h *abHandle) tabNew(ctx context.Context, p tabNewParams) (any, error) {
	return h.runJSON(ctx, tabNewArgs(p.url, p.label)...)
}

func (h *abHandle) tabClose(ctx context.Context, ref string) (any, error) {
	args := []string{"tab", "close"}
	if ref != "" {
		args = append(args, ref)
	}
	return h.runJSON(ctx, args...)
}

func (h *abHandle) tabSelect(ctx context.Context, ref string) (any, error) {
	if ref == "" {
		return nil, errors.New("agentBrowser.tabs.select: a tab ref (t<N> or label) is required")
	}
	return h.runJSON(ctx, "tab", ref)
}

// addTabs wires tab management into the handle object.
func (h *abHandle) addTabs(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["tabs"] = map[string]any{
		"list":   abAsync(vm, loop, abNoArgs, h.tabList),
		"new":    abAsync(vm, loop, tabNewExtract, h.tabNew),
		"close":  abAsync(vm, loop, abStrArg0, h.tabClose),
		"select": abAsync(vm, loop, abStrArg0, h.tabSelect),
	}
}
