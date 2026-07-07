package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// cookieStringOpts are cookie set options that take a string value, in stable order.
// expires is handled separately (rendered as an integer) and is NOT in this list.
var cookieStringOpts = []struct{ key, flag string }{
	{"url", "--url"},
	{"domain", "--domain"},
	{"path", "--path"},
	{"sameSite", "--sameSite"},
}

// cookieSetArgs builds `cookies set <name> <value> [options]`. String options
// are emitted first (stable order), then expires (as integer), then the boolean
// flags httpOnly/secure.
func cookieSetArgs(name, value string, opts map[string]any) []string {
	args := []string{"cookies", "set", name, value}
	for _, o := range cookieStringOpts {
		if v, ok := opts[o.key]; ok {
			if s := fmt.Sprintf("%v", v); s != "" {
				args = append(args, o.flag, s)
			}
		}
	}
	// expires is a Unix-seconds integer; render with %d to avoid scientific
	// notation (e.g. "1.7e+09") that the CLI would reject.
	if v, ok := opts["expires"]; ok {
		args = append(args, "--expires", fmt.Sprintf("%d", numToInt(v)))
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

func (h *abHandle) cookiesGet(ctx context.Context, _ struct{}) (any, error) {
	return h.runJSON(ctx, "cookies", "get")
}

func (h *abHandle) cookiesClear(ctx context.Context, _ struct{}) (any, error) {
	return h.runJSON(ctx, "cookies", "clear")
}

// cookiesSetParams carries name/value plus the cookie options map.
type cookiesSetParams struct {
	name, value string
	opts        map[string]any
}

func cookiesSetExtract(call goja.FunctionCall) (cookiesSetParams, error) {
	return cookiesSetParams{name: strArg(call, 0), value: strArg(call, 1), opts: optsArgMap(call, 2)}, nil
}

func (h *abHandle) cookiesSet(ctx context.Context, p cookiesSetParams) (any, error) {
	if p.name == "" {
		return nil, errors.New("agentBrowser.cookies.set: name is required")
	}
	return h.runJSON(ctx, cookieSetArgs(p.name, p.value, p.opts)...)
}

// storageGet runs `storage <kind> get [key]`.
func (h *abHandle) storageGet(kind string) func(context.Context, string) (any, error) {
	return func(ctx context.Context, key string) (any, error) {
		if key != "" {
			return h.runJSON(ctx, storageArgs(kind, "get", key)...)
		}
		return h.runJSON(ctx, storageArgs(kind, "get")...)
	}
}

// storageSetParams carries the key/value pair for `storage <kind> set`.
type storageSetParams struct {
	key, value string
}

func storageSetExtract(call goja.FunctionCall) (storageSetParams, error) {
	return storageSetParams{key: strArg(call, 0), value: strArg(call, 1)}, nil
}

func (h *abHandle) storageSet(kind string) func(context.Context, storageSetParams) (any, error) {
	return func(ctx context.Context, p storageSetParams) (any, error) {
		if p.key == "" {
			return nil, fmt.Errorf("agentBrowser.storage.%s.set: key is required", kind)
		}
		return h.runJSON(ctx, storageArgs(kind, "set", p.key, p.value)...)
	}
}

func (h *abHandle) storageClear(kind string) func(context.Context, struct{}) (any, error) {
	return func(ctx context.Context, _ struct{}) (any, error) {
		return h.runJSON(ctx, storageArgs(kind, "clear")...)
	}
}

func (h *abHandle) storageObj(kind string, vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"get":   abAsync(vm, loop, abStrArg0, h.storageGet(kind)),
		"set":   abAsync(vm, loop, storageSetExtract, h.storageSet(kind)),
		"clear": abAsync(vm, loop, abNoArgs, h.storageClear(kind)),
	}
}

// addStorage wires cookies + local/session storage into the handle object.
func (h *abHandle) addStorage(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["cookies"] = map[string]any{
		"get":   abAsync(vm, loop, abNoArgs, h.cookiesGet),
		"set":   abAsync(vm, loop, cookiesSetExtract, h.cookiesSet),
		"clear": abAsync(vm, loop, abNoArgs, h.cookiesClear),
	}
	obj["storage"] = map[string]any{
		"local":   h.storageObj("local", vm, loop),
		"session": h.storageObj("session", vm, loop),
	}
}
