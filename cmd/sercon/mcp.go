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
			return ms.handle(vm)
		},
	}
}

// handle builds the goja object returned to the script. Methods for
// tool/resource/prompt/stdio/listen are added by later tasks; this establishes
// the object and close().
func (ms *mcpServer) handle(vm *goja.Runtime) goja.Value {
	h := vm.NewObject()
	must := func(name string, fn func(goja.FunctionCall) goja.Value) { _ = h.Set(name, fn) }
	must("tool", ms.jsTool)
	must("resource", ms.jsResource)
	must("prompt", ms.jsPrompt)
	must("stdio", ms.jsStdio)
	must("listen", ms.jsListen)
	must("close", ms.jsClose)
	return h
}

//nolint:unused // consumed by a later task's listen/stdio transport guard
var errAlreadyStarted = errors.New("mcp: server already started; register tools/resources/prompts before serving")
