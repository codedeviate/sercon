package main

import (
	"context"
	"errors"
	"math"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mcpNamespace builds the per-Run `mcp` global. It needs the vm/loop, so it is
// registered via RegisterNamespaceFactory (see registerSurface in main.go).
func mcpNamespace(eng *scriptengine.Engine, vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"serve": func(call goja.FunctionCall) goja.Value {
			cfg := call.Argument(0)
			o, ok := cfg.Export().(map[string]any)
			if !ok {
				panic(vm.NewTypeError("mcp.serve: a config object is required"))
			}
			name, _ := o["name"].(string)
			version, _ := o["version"].(string)
			if name == "" || version == "" {
				panic(vm.NewTypeError("mcp.serve: `name` and `version` are required"))
			}
			instructions, _ := o["instructions"].(string)
			pageSize := mcpPageSizeArg(vm, o)

			ms := &mcpServer{eng: eng, vm: vm, loop: loop}
			// SubscribeHandler/UnsubscribeHandler are set unconditionally
			// (never nil) and together: the go-sdk panics at NewServer time
			// if only one of the pair is set (see ServerOptions' validation
			// in the SDK's server.go), and setting both is also what makes
			// the SDK advertise the resources.subscribe capability during
			// initialize — a script that never calls
			// srv.onSubscribe/onUnsubscribe still gets working subscribe/
			// unsubscribe plumbing; the JS callback dispatch is simply skipped
			// when nil, see getOnSubscribe/getOnUnsubscribe in mcp_server.go).
			// The go-sdk tracks the actual subscriber set itself (see
			// mcpServer's subscribeMu doc comment in mcp_server.go); these
			// dispatchers only forward to the JS hook, they don't record
			// anything of their own.
			//
			// Both handlers run on an SDK goroutine (never the loop), so the
			// JS callback is invoked via LoopCallable.Call (which schedules
			// onto the loop itself), not CallOnLoop. The callback's
			// result/error is deliberately discarded: a script's
			// onSubscribe/onUnsubscribe hook is a best-effort start/
			// stop-watching notification, not a gate a client's subscribe/
			// unsubscribe request can fail — always return nil so the SDK
			// always accepts the request.
			ms.srv = mcp.NewServer(
				&mcp.Implementation{Name: name, Version: version},
				&mcp.ServerOptions{
					Instructions: instructions,
					PageSize:     pageSize,
					SubscribeHandler: func(_ context.Context, req *mcp.SubscribeRequest) error {
						uri := req.Params.URI
						if cb := ms.getOnSubscribe(); cb != nil {
							_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
								return []goja.Value{vm.ToValue(uri)}, nil
							})
						}
						return nil
					},
					UnsubscribeHandler: func(_ context.Context, req *mcp.UnsubscribeRequest) error {
						uri := req.Params.URI
						if cb := ms.getOnUnsubscribe(); cb != nil {
							_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
								return []goja.Value{vm.ToValue(uri)}, nil
							})
						}
						return nil
					},
					// CompletionHandler is likewise set unconditionally (see the
					// Subscribe/UnsubscribeHandler comment above for the same
					// reasoning): this is what makes the SDK advertise the
					// `completions` capability during initialize, and a script
					// that never calls srv.completion still gets a working
					// (empty) completion/complete response instead of the SDK
					// rejecting the method outright — see mcpCompletionHandler's
					// doc comment below.
					CompletionHandler: ms.mcpCompletionHandler,
					// RootsListChangedHandler is likewise set unconditionally: the
					// go-sdk's own signature for this notification has no
					// return value at all (see mcpRootsListChangedHandler's doc
					// comment in mcp_server.go), so — unlike Subscribe/
					// Unsubscribe/CompletionHandler — there is no capability this
					// wiring gates; it simply dispatches to the JS
					// srv.onRootsChanged hook when one is registered, and is a
					// no-op otherwise.
					RootsListChangedHandler: ms.mcpRootsListChangedHandler,
				},
			)
			// Arm the stdout->stderr redirect now (real CLI runs only), so any
			// script output between here and srv.stdio() cannot leak onto the
			// JSON-RPC stream. No-op for in-process engine tests. See
			// mcp_stdio_guard.go.
			installMCPStdioRedirectIfArmed()
			return ms.handle(vm)
		},
	}
}

// handle builds the goja object returned to the script:
// tool/resource/resourceTemplate/prompt (capability registration,
// runtime-mutable — see jsTool's doc comment) and their
// removeTool/removeResource/removePrompt counterparts, onSubscribe/
// onUnsubscribe/resourceUpdated (resource-subscription hooks, Task 5),
// completion (argument-completion hook, Task 6), onRootsChanged
// (client-roots-changed hook, Task 4 — its sibling ctx.roots() lives on the
// per-request ctx object built by newRequestContext, not here), stdio/listen
// (the two transports, still one-per-handle), and close (currently a no-op
// placeholder — see jsClose's doc comment). All are implemented in
// mcp_server.go.
func (ms *mcpServer) handle(vm *goja.Runtime) goja.Value {
	h := vm.NewObject()
	must := func(name string, fn func(goja.FunctionCall) goja.Value) { _ = h.Set(name, fn) }
	must("tool", ms.jsTool)
	must("resource", ms.jsResource)
	must("resourceTemplate", ms.jsResourceTemplate)
	must("prompt", ms.jsPrompt)
	must("removeTool", ms.jsRemoveTool)
	must("removeResource", ms.jsRemoveResource)
	must("removePrompt", ms.jsRemovePrompt)
	must("onSubscribe", ms.jsOnSubscribe)
	must("onUnsubscribe", ms.jsOnUnsubscribe)
	must("onRootsChanged", ms.jsOnRootsChanged)
	must("resourceUpdated", ms.jsResourceUpdated)
	must("completion", ms.jsCompletion)
	must("stdio", ms.jsStdio)
	must("listen", ms.jsListen)
	must("close", ms.jsClose)
	return h
}

// mcpPageSizeArg extracts an optional positive-integer `pageSize` from
// mcp.serve's config object (already Export()'d to map[string]any by the
// caller). It returns 0 (leaving ServerOptions.PageSize at the SDK's
// DefaultPageSize) when the key is absent. goja exports whole-number JS
// numbers as int64 and non-integer numbers as float64 (see toUint32/toInt32
// in exif_model.go for the same goja nuance), so both are accepted; anything
// else present under the key — including a non-integer float, zero,
// negative, or a non-number value like a string — throws, per the task
// brief's validation contract.
func mcpPageSizeArg(vm *goja.Runtime, o map[string]any) int {
	v, present := o["pageSize"]
	if !present {
		return 0
	}
	var n float64
	switch t := v.(type) {
	case int64:
		n = float64(t)
	case float64:
		n = t
	default:
		panic(vm.NewTypeError("mcp.serve: pageSize must be a positive integer"))
	}
	if n != math.Trunc(n) || n <= 0 {
		panic(vm.NewTypeError("mcp.serve: pageSize must be a positive integer"))
	}
	return int(n)
}

// errAlreadyStarted is thrown when a second transport (stdio()/listen()) is
// started on a handle that already has one running — support for concurrent
// transports on one handle is later-phase (if ever) work. It is NOT used for
// tool/resource/prompt registration: those may run at any time, including
// after a transport has started (see jsTool's doc comment in mcp_server.go).
var errAlreadyStarted = errors.New("mcp: server already started; a transport is already running")
