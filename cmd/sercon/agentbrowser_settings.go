package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// setArgs builds `set <setting> <operands...>`.
func setArgs(setting string, operands ...string) []string {
	return append([]string{"set", setting}, operands...)
}

// recordArgs builds `record <op> <operands...>`.
func recordArgs(op string, operands ...string) []string {
	return append([]string{"record", op}, operands...)
}

// offlineArg maps a bool to the CLI's on/off token.
func offlineArg(on bool) string {
	if on {
		return "on"
	}
	return "off"
}

// numStr stringifies a JS number argument (goja exports as float64/int64).
func numStr(call goja.FunctionCall, i int) string {
	return fmt.Sprintf("%v", call.Argument(i).Export())
}

// runSet runs a `set` subcommand and returns the parsed JSON result.
func (h *abHandle) runSet(ctx context.Context, setting string, operands ...string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, setArgs(setting, operands...)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

func (h *abHandle) setViewport(ctx context.Context, call goja.FunctionCall) (any, error) {
	w, hh := numStr(call, 0), numStr(call, 1)
	ops := []string{w, hh}
	if s := call.Argument(2); s != nil && !goja.IsUndefined(s) {
		ops = append(ops, fmt.Sprintf("%v", s.Export())) // optional scale
	}
	return h.runSet(ctx, "viewport", ops...)
}

func (h *abHandle) setDevice(ctx context.Context, call goja.FunctionCall) (any, error) {
	name := strArg(call, 0)
	if name == "" {
		return nil, errors.New("agentBrowser.set.device: name is required")
	}
	return h.runSet(ctx, "device", name)
}

func (h *abHandle) setGeo(ctx context.Context, call goja.FunctionCall) (any, error) {
	return h.runSet(ctx, "geo", numStr(call, 0), numStr(call, 1))
}

func (h *abHandle) setOffline(ctx context.Context, call goja.FunctionCall) (any, error) {
	on := true // default: enable offline
	if a := call.Argument(0); a != nil && !goja.IsUndefined(a) && !goja.IsNull(a) {
		on = a.ToBoolean()
	}
	return h.runSet(ctx, "offline", offlineArg(on))
}

func (h *abHandle) setHeaders(ctx context.Context, call goja.FunctionCall) (any, error) {
	obj := call.Argument(0).Export()
	if obj == nil {
		return nil, errors.New("agentBrowser.set.headers: an object of header name/value pairs is required")
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("agentBrowser.set.headers: %w", err)
	}
	return h.runSet(ctx, "headers", string(b))
}

func (h *abHandle) setCredentials(ctx context.Context, call goja.FunctionCall) (any, error) {
	user, pass := strArg(call, 0), strArg(call, 1)
	if user == "" {
		return nil, errors.New("agentBrowser.set.credentials: username is required")
	}
	return h.runSet(ctx, "credentials", user, pass)
}

// setMedia maps media(scheme?, reducedMotion?) -> `set media [dark|light] [reduced-motion]`.
func (h *abHandle) setMedia(ctx context.Context, call goja.FunctionCall) (any, error) {
	var ops []string
	if s := strArg(call, 0); s != "" {
		ops = append(ops, s) // "dark" | "light"
	}
	if rm := call.Argument(1); rm != nil && !goja.IsUndefined(rm) && rm.ToBoolean() {
		ops = append(ops, "reduced-motion")
	}
	if len(ops) == 0 {
		return nil, errors.New("agentBrowser.set.media: pass a scheme (\"dark\"|\"light\") and/or reducedMotion=true")
	}
	return h.runSet(ctx, "media", ops...)
}

// recordStart runs `record start <path.webm> [url]`.
func (h *abHandle) recordStart(ctx context.Context, call goja.FunctionCall) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	path := strArg(call, 0)
	if path == "" {
		return nil, errors.New("agentBrowser.record.start: a .webm output path is required")
	}
	ops := []string{"start", path}
	if url := strArg(call, 1); url != "" {
		ops = append(ops, url)
	}
	out, err := abRunChecked(ctx, h.session, h.global, recordArgs(ops[0], ops[1:]...)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

func (h *abHandle) recordStop(ctx context.Context, _ goja.FunctionCall) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, recordArgs("stop")...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// addSettings wires the settings + record surface into the handle object.
func (h *abHandle) addSettings(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["set"] = map[string]any{
		"viewport":    h.p(vm, loop, h.setViewport),
		"device":      h.p(vm, loop, h.setDevice),
		"geo":         h.p(vm, loop, h.setGeo),
		"offline":     h.p(vm, loop, h.setOffline),
		"headers":     h.p(vm, loop, h.setHeaders),
		"credentials": h.p(vm, loop, h.setCredentials),
		"media":       h.p(vm, loop, h.setMedia),
	}
	obj["record"] = map[string]any{
		"start": h.p(vm, loop, h.recordStart),
		"stop":  h.p(vm, loop, h.recordStop),
	}
}
