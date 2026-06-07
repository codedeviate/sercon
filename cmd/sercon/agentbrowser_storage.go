package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// cookieStringOpts are cookie set options that take a value, in stable order.
var cookieStringOpts = []struct{ key, flag string }{
	{"url", "--url"},
	{"domain", "--domain"},
	{"path", "--path"},
	{"sameSite", "--sameSite"},
	{"expires", "--expires"},
}

// cookieSetArgs builds `cookies set <name> <value> [options]`. String options
// are emitted first (stable order), then the boolean flags httpOnly/secure.
func cookieSetArgs(name, value string, opts map[string]any) []string {
	args := []string{"cookies", "set", name, value}
	for _, o := range cookieStringOpts {
		if v, ok := opts[o.key]; ok {
			if s := fmt.Sprintf("%v", v); s != "" {
				args = append(args, o.flag, s)
			}
		}
	}
	if b, _ := opts["httpOnly"].(bool); b {
		args = append(args, "--httpOnly")
	}
	if b, _ := opts["secure"].(bool); b {
		args = append(args, "--secure")
	}
	return args
}

// storageArgs builds `storage <kind> <op> [operands...]`.
func storageArgs(kind, op string, operands ...string) []string {
	return append([]string{"storage", kind, op}, operands...)
}

func (h *abHandle) cookiesGet(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, "cookies", "get")
}

func (h *abHandle) cookiesClear(ctx context.Context, _ goja.FunctionCall) (any, error) {
	return h.runJSON(ctx, "cookies", "clear")
}

func (h *abHandle) cookiesSet(ctx context.Context, call goja.FunctionCall) (any, error) {
	name, value := strArg(call, 0), strArg(call, 1)
	if name == "" {
		return nil, errors.New("agentBrowser.cookies.set: name is required")
	}
	return h.runJSON(ctx, cookieSetArgs(name, value, optsArgMap(call, 2))...)
}

// storageGet runs `storage <kind> get [key]`.
func (h *abHandle) storageGet(kind string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		if key := strArg(call, 0); key != "" {
			return h.runJSON(ctx, storageArgs(kind, "get", key)...)
		}
		return h.runJSON(ctx, storageArgs(kind, "get")...)
	}
}

func (h *abHandle) storageSet(kind string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		key := strArg(call, 0)
		if key == "" {
			return nil, fmt.Errorf("agentBrowser.storage.%s.set: key is required", kind)
		}
		return h.runJSON(ctx, storageArgs(kind, "set", key, strArg(call, 1))...)
	}
}

func (h *abHandle) storageClear(kind string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, _ goja.FunctionCall) (any, error) {
		return h.runJSON(ctx, storageArgs(kind, "clear")...)
	}
}

func (h *abHandle) storageObj(kind string, vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"get":   h.p(vm, loop, h.storageGet(kind)),
		"set":   h.p(vm, loop, h.storageSet(kind)),
		"clear": h.p(vm, loop, h.storageClear(kind)),
	}
}

// addStorage wires cookies + local/session storage into the handle object.
func (h *abHandle) addStorage(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["cookies"] = map[string]any{
		"get":   h.p(vm, loop, h.cookiesGet),
		"set":   h.p(vm, loop, h.cookiesSet),
		"clear": h.p(vm, loop, h.cookiesClear),
	}
	obj["storage"] = map[string]any{
		"local":   h.storageObj("local", vm, loop),
		"session": h.storageObj("session", vm, loop),
	}
}
