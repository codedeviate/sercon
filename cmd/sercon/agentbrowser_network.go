package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// routeArgs builds `network route <url> [--abort | --body <json>] [--resource-type <csv>]`.
// When both abort and body are set, abort wins (the CLI's --abort and --body are mutually
// exclusive); body is ignored.
func routeArgs(url string, opts map[string]any) ([]string, error) {
	args := []string{"network", "route", url}
	if b, _ := opts["abort"].(bool); b {
		args = append(args, "--abort")
	} else if body, ok := opts["body"]; ok {
		j, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("agentBrowser.network.route: body is not JSON-encodable: %w", err)
		}
		args = append(args, "--body", string(j))
	}
	if rt, _ := opts["resourceType"].(string); rt != "" {
		args = append(args, "--resource-type", rt)
	}
	return args, nil
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

// netRouteArgs carries the url pattern plus the route options map.
type netRouteArgs struct {
	url  string
	opts map[string]any
}

func netRouteExtract(call goja.FunctionCall) (netRouteArgs, error) {
	return netRouteArgs{url: strArg(call, 0), opts: optsArgMap(call, 1)}, nil
}

func (h *abHandle) netRoute(ctx context.Context, a netRouteArgs) (any, error) {
	if a.url == "" {
		return nil, errors.New("agentBrowser.network.route: url pattern is required")
	}
	args, err := routeArgs(a.url, a.opts)
	if err != nil {
		return nil, err
	}
	return h.runJSON(ctx, args...)
}

func (h *abHandle) netUnroute(ctx context.Context, url string) (any, error) {
	args := []string{"network", "unroute"}
	if url != "" {
		args = append(args, url)
	}
	return h.runJSON(ctx, args...)
}

func (h *abHandle) netRequests(ctx context.Context, opts map[string]any) (any, error) {
	return h.runJSON(ctx, requestsArgs(opts)...)
}

func (h *abHandle) netRequest(ctx context.Context, id string) (any, error) {
	if id == "" {
		return nil, errors.New("agentBrowser.network.request: requestId is required")
	}
	return h.runJSON(ctx, "network", "request", id)
}

// harOp returns a work half running `network har <op> [path]`.
func (h *abHandle) harOp(op string) func(context.Context, string) (any, error) {
	return func(ctx context.Context, p string) (any, error) {
		args := []string{"network", "har", op}
		if p != "" {
			args = append(args, p)
		}
		return h.runJSON(ctx, args...)
	}
}

// addNetwork wires the network surface into the handle object.
func (h *abHandle) addNetwork(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["network"] = map[string]any{
		"route":    abAsync(vm, loop, netRouteExtract, h.netRoute),
		"unroute":  abAsync(vm, loop, abStrArg0, h.netUnroute),
		"requests": abAsync(vm, loop, abOptsArg0, h.netRequests),
		"request":  abAsync(vm, loop, abStrArg0, h.netRequest),
		"har": map[string]any{
			"start": abAsync(vm, loop, abStrArg0, h.harOp("start")),
			"stop":  abAsync(vm, loop, abStrArg0, h.harOp("stop")),
		},
	}
}
