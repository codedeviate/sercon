package main

import (
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

			ms := &mcpServer{
				eng: eng, vm: vm, loop: loop,
				srv: mcp.NewServer(
					&mcp.Implementation{Name: name, Version: version},
					&mcp.ServerOptions{Instructions: instructions},
				),
			}
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
// removeTool/removeResource/removePrompt counterparts, stdio/listen (the two
// transports, still one-per-handle), and close (currently a no-op
// placeholder — see jsClose's doc comment). All ten are implemented in
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
