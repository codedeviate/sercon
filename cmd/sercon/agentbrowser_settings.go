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
// Returns "" when the argument is absent, undefined, or null.
func numStr(call goja.FunctionCall, i int) string {
	v := call.Argument(i)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return fmt.Sprintf("%v", v.Export())
}

// runSet runs a `set` subcommand and returns the parsed JSON result.
func (h *abHandle) runSet(ctx context.Context, setting string, operands ...string) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, setArgs(setting, operands...)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// viewportArgs carries width/height plus the optional scale.
type viewportArgs struct {
	w, h     string
	scale    string
	hasScale bool
}

func viewportExtract(call goja.FunctionCall) (viewportArgs, error) {
	a := viewportArgs{w: numStr(call, 0), h: numStr(call, 1)}
	if s := call.Argument(2); s != nil && !goja.IsUndefined(s) {
		a.scale, a.hasScale = fmt.Sprintf("%v", s.Export()), true // optional scale
	}
	return a, nil
}

func (h *abHandle) setViewport(ctx context.Context, a viewportArgs) (any, error) {
	if a.w == "" || a.h == "" {
		return nil, errors.New("agentBrowser.set.viewport: width and height are required")
	}
	ops := []string{a.w, a.h}
	if a.hasScale {
		ops = append(ops, a.scale)
	}
	return h.runSet(ctx, "viewport", ops...)
}

func (h *abHandle) setDevice(ctx context.Context, name string) (any, error) {
	if name == "" {
		return nil, errors.New("agentBrowser.set.device: name is required")
	}
	return h.runSet(ctx, "device", name)
}

// geoArgs carries latitude/longitude as stringified numbers.
type geoArgs struct {
	lat, lng string
}

func geoExtract(call goja.FunctionCall) (geoArgs, error) {
	return geoArgs{lat: numStr(call, 0), lng: numStr(call, 1)}, nil
}

func (h *abHandle) setGeo(ctx context.Context, a geoArgs) (any, error) {
	if a.lat == "" || a.lng == "" {
		return nil, errors.New("agentBrowser.set.geo: latitude and longitude are required")
	}
	return h.runSet(ctx, "geo", a.lat, a.lng)
}

func offlineExtract(call goja.FunctionCall) (bool, error) {
	on := true // default: enable offline
	if a := call.Argument(0); a != nil && !goja.IsUndefined(a) && !goja.IsNull(a) {
		on = a.ToBoolean()
	}
	return on, nil
}

func (h *abHandle) setOffline(ctx context.Context, on bool) (any, error) {
	return h.runSet(ctx, "offline", offlineArg(on))
}

func headersExtract(call goja.FunctionCall) (any, error) {
	return call.Argument(0).Export(), nil
}

func (h *abHandle) setHeaders(ctx context.Context, obj any) (any, error) {
	if obj == nil {
		return nil, errors.New("agentBrowser.set.headers: an object of header name/value pairs is required")
	}
	b, err := json.Marshal(obj)
	if err != nil {
		return nil, fmt.Errorf("agentBrowser.set.headers: %w", err)
	}
	return h.runSet(ctx, "headers", string(b))
}

// credentialsArgs carries the basic-auth username/password pair.
type credentialsArgs struct {
	user, pass string
}

func credentialsExtract(call goja.FunctionCall) (credentialsArgs, error) {
	return credentialsArgs{user: strArg(call, 0), pass: strArg(call, 1)}, nil
}

func (h *abHandle) setCredentials(ctx context.Context, a credentialsArgs) (any, error) {
	if a.user == "" {
		return nil, errors.New("agentBrowser.set.credentials: username is required")
	}
	return h.runSet(ctx, "credentials", a.user, a.pass)
}

// mediaArgs carries the optional colour scheme + reduced-motion flag.
type mediaArgs struct {
	scheme        string
	reducedMotion bool
}

func mediaExtract(call goja.FunctionCall) (mediaArgs, error) {
	a := mediaArgs{scheme: strArg(call, 0)} // "dark" | "light"
	if rm := call.Argument(1); rm != nil && !goja.IsUndefined(rm) && rm.ToBoolean() {
		a.reducedMotion = true
	}
	return a, nil
}

// setMedia maps media(scheme?, reducedMotion?) -> `set media [dark|light] [reduced-motion]`.
func (h *abHandle) setMedia(ctx context.Context, a mediaArgs) (any, error) {
	var ops []string
	if a.scheme != "" {
		ops = append(ops, a.scheme)
	}
	if a.reducedMotion {
		ops = append(ops, "reduced-motion")
	}
	if len(ops) == 0 {
		return nil, errors.New("agentBrowser.set.media: pass a scheme (\"dark\"|\"light\") and/or reducedMotion=true")
	}
	return h.runSet(ctx, "media", ops...)
}

// recordStartArgs carries the output path plus the optional url.
type recordStartArgs struct {
	path, url string
}

func recordStartExtract(call goja.FunctionCall) (recordStartArgs, error) {
	return recordStartArgs{path: strArg(call, 0), url: strArg(call, 1)}, nil
}

// recordStart runs `record start <path.webm> [url]`.
func (h *abHandle) recordStart(ctx context.Context, a recordStartArgs) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	if a.path == "" {
		return nil, errors.New("agentBrowser.record.start: a .webm output path is required")
	}
	ops := []string{a.path}
	if a.url != "" {
		ops = append(ops, a.url)
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, recordArgs("start", ops...)...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

func (h *abHandle) recordStop(ctx context.Context, _ struct{}) (any, error) {
	if err := h.requireOpen(); err != nil {
		return nil, err
	}
	out, err := abRunChecked(ctx, h.session, h.global, h.timeout, recordArgs("stop")...)
	if err != nil {
		return nil, err
	}
	return parseJSON(out)
}

// addSettings wires the settings + record surface into the handle object.
func (h *abHandle) addSettings(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["set"] = map[string]any{
		"viewport":    abAsync(vm, loop, viewportExtract, h.setViewport),
		"device":      abAsync(vm, loop, abStrArg0, h.setDevice),
		"geo":         abAsync(vm, loop, geoExtract, h.setGeo),
		"offline":     abAsync(vm, loop, offlineExtract, h.setOffline),
		"headers":     abAsync(vm, loop, headersExtract, h.setHeaders),
		"credentials": abAsync(vm, loop, credentialsExtract, h.setCredentials),
		"media":       abAsync(vm, loop, mediaExtract, h.setMedia),
	}
	obj["record"] = map[string]any{
		"start": abAsync(vm, loop, recordStartExtract, h.recordStart),
		"stop":  abAsync(vm, loop, abNoArgs, h.recordStop),
	}
}
