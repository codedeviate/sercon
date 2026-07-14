package main

import (
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mcpServer is the per-Run state behind a mcp.serve(...) handle.
type mcpServer struct {
	eng     *scriptengine.Engine
	vm      *goja.Runtime
	loop    *eventloop.EventLoop
	srv     *mcp.Server
	started bool   //nolint:unused // set by a later task's listen/stdio transport
	release func() //nolint:unused // HoldRun release, set when a transport starts; called by a later task's close()
}

// The methods below are intentional stubs: tool/resource/prompt registration
// and the stdio/listen transports are filled in by later tasks. close() is
// a no-op until a transport (and therefore a HoldRun) exists to release.
func (ms *mcpServer) jsTool(call goja.FunctionCall) goja.Value {
	panic(ms.vm.NewTypeError("mcp: tool() not yet implemented"))
}
func (ms *mcpServer) jsResource(call goja.FunctionCall) goja.Value {
	panic(ms.vm.NewTypeError("mcp: resource() not yet implemented"))
}
func (ms *mcpServer) jsPrompt(call goja.FunctionCall) goja.Value {
	panic(ms.vm.NewTypeError("mcp: prompt() not yet implemented"))
}
func (ms *mcpServer) jsStdio(call goja.FunctionCall) goja.Value {
	panic(ms.vm.NewTypeError("mcp: stdio() not yet implemented"))
}
func (ms *mcpServer) jsListen(call goja.FunctionCall) goja.Value {
	panic(ms.vm.NewTypeError("mcp: listen() not yet implemented"))
}
func (ms *mcpServer) jsClose(call goja.FunctionCall) goja.Value { return goja.Undefined() }
