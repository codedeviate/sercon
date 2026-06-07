package main

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// routeArgs builds `network route <url> [--abort | --body <json>] [--resource-type <csv>]`.
func routeArgs(url string, opts map[string]any) []string {
	args := []string{"network", "route", url}
	if b, _ := opts["abort"].(bool); b {
		args = append(args, "--abort")
	} else if body, ok := opts["body"]; ok {
		if j, err := json.Marshal(body); err == nil {
			args = append(args, "--body", string(j))
		}
	}
	if rt, _ := opts["resourceType"].(string); rt != "" {
		args = append(args, "--resource-type", rt)
	}
	return args
}

// requestsArgs builds `network requests [flags]`.
func requestsArgs(opts map[string]any) []string {
	args := []string{"network", "requests"}
	if b, _ := opts["clear"].(bool); b {
		args = append(args, "--clear")
	}
	if s, _ := opts["filter"].(string); s != "" {
		args = append(args, "--filter", s)
	}
	if s, _ := opts["type"].(string); s != "" {
		args = append(args, "--type", s)
	}
	if s, _ := opts["method"].(string); s != "" {
		args = append(args, "--method", s)
	}
	if s, _ := opts["status"].(string); s != "" {
		args = append(args, "--status", s)
	}
	return args
}

func (h *abHandle) netRoute(ctx context.Context, call goja.FunctionCall) (any, error) {
	url := strArg(call, 0)
	if url == "" {
		return nil, errors.New("agentBrowser.network.route: url pattern is required")
	}
	return h.runJSON(ctx, routeArgs(url, optsArgMap(call, 1))...)
}

func (h *abHandle) netUnroute(ctx context.Context, call goja.FunctionCall) (any, error) {
	args := []string{"network", "unroute"}
	if url := strArg(call, 0); url != "" {
		args = append(args, url)
	}
	return h.runJSON(ctx, args...)
}

func (h *abHandle) netRequests(ctx context.Context, call goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, requestsArgs(optsArgMap(call, 0))...)
}

func (h *abHandle) netRequest(ctx context.Context, call goja.FunctionCall) (any, error) {
	id := strArg(call, 0)
	if id == "" {
		return nil, errors.New("agentBrowser.network.request: requestId is required")
	}
	return h.runJSON(ctx, "network", "request", id)
}

// harOp returns a method running `network har <op> [path]`.
func (h *abHandle) harOp(op string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		args := []string{"network", "har", op}
		if p := strArg(call, 0); p != "" {
			args = append(args, p)
		}
		return h.runJSON(ctx, args...)
	}
}

// addNetwork wires the network surface into the handle object.
func (h *abHandle) addNetwork(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["network"] = map[string]any{
		"route":    h.p(vm, loop, h.netRoute),
		"unroute":  h.p(vm, loop, h.netUnroute),
		"requests": h.p(vm, loop, h.netRequests),
		"request":  h.p(vm, loop, h.netRequest),
		"har": map[string]any{
			"start": h.p(vm, loop, h.harOp("start")),
			"stop":  h.p(vm, loop, h.harOp("stop")),
		},
	}
}
