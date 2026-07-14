package main

import (
	"context"
	"errors"

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
// stdio/listen (the two transports, still one-per-handle), and close
// (currently a no-op placeholder — see jsClose's doc comment). All are
// implemented in mcp_server.go.
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
	must("resourceUpdated", ms.jsResourceUpdated)
	must("stdio", ms.jsStdio)
	must("listen", ms.jsListen)
	must("close", ms.jsClose)
	return h
}

// errAlreadyStarted is thrown when a second transport (stdio()/listen()) is
// started on a handle that already has one running — support for concurrent
// transports on one handle is later-phase (if ever) work. It is NOT used for
// tool/resource/prompt registration: those may run at any time, including
// after a transport has started (see jsTool's doc comment in mcp_server.go).
var errAlreadyStarted = errors.New("mcp: server already started; a transport is already running")
