package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

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
	started bool
	release func()       //nolint:unused // HoldRun release, set when a transport starts; called by a later task's close()
	reqSeq  atomic.Int64 // monotonic counter backing newRequestContext's requestId
}

// newRequestContext builds the Phase-1 `ctx` object passed as the 2nd
// argument to every tool/resource/prompt JS handler: { requestId,
// clientInfo: { name, version } }. Phase 2/3 (progress/log/sample/elicit)
// hook in here later, holding onto sess for the SDK calls that need it
// (NotifyProgress, Log, CreateMessage, Elicit) — none of that exists yet.
//
// requestId: the go-sdk does carry a JSON-RPC request id per call, but it's
// stashed in an unexported context key (idContextKey, see server.go/
// streamable.go) with no public accessor, and AddTool/AddResource/AddPrompt
// handlers in this file discard their context.Context argument entirely.
// sess.ID() looked like the next-best fallback, but it's backed by an
// SDK-internal hasSessionID interface that only streamableServerConn
// implements — ioConn (stdio, and the in-memory transport used by this
// package's own tests) and sseServerConn both hard-code SessionID() to
// return "" (see transport.go/sse.go), so sess.ID() would be empty for the
// transports this project actually cares about (stdio is the primary
// target; stdio()/listen() aren't wired up yet, but in-memory already
// exercises the same empty-ID path in tests). Genuinely empty ids defeat
// the point of a request identifier, so instead requestId is built from a
// per-mcpServer monotonic sequence number (always non-empty, unique across
// every call this server instance handles), prefixed with the session id
// when the transport happens to provide one for extra traceability.
// Revisit if a future go-sdk version exposes the real per-call JSON-RPC id.
//
// This must be called ON the event loop (from a buildArgs callback, which
// callJSHandler always invokes on-loop) since it allocates goja values.
func (ms *mcpServer) newRequestContext(vm *goja.Runtime, sess *mcp.ServerSession) goja.Value {
	ctxObj := vm.NewObject()

	seq := ms.reqSeq.Add(1)
	requestID := fmt.Sprintf("req-%d", seq)
	if sess != nil {
		if sid := sess.ID(); sid != "" {
			requestID = fmt.Sprintf("%s-%d", sid, seq)
		}
	}
	_ = ctxObj.Set("requestId", requestID)

	name, version := "", ""
	if sess != nil {
		if ip := sess.InitializeParams(); ip != nil && ip.ClientInfo != nil {
			name = ip.ClientInfo.Name
			version = ip.ClientInfo.Version
		}
	}
	clientInfo := vm.NewObject()
	_ = clientInfo.Set("name", name)
	_ = clientInfo.Set("version", version)
	_ = ctxObj.Set("clientInfo", clientInfo)

	return ctxObj
}

// jsTool implements srv.tool({ name, description?, inputSchema, outputSchema?,
// handler }). It registers a real SDK tool whose ToolHandler bridges into the
// JS handler through callJSHandler: the SDK invokes the handler on its own
// goroutine, callJSHandler hops onto the event loop to run the JS function,
// and the result is converted to an *mcp.CallToolResult ON the loop (via
// toToolResult) before it crosses back — so no goja.Value ever escapes the
// loop goroutine.
//
// Registration must happen before the server is serving: adding a tool after a
// transport has started would need a list-changed notification, which is a
// later phase, so a post-start call throws.
func (ms *mcpServer) jsTool(call goja.FunctionCall) goja.Value {
	if ms.started {
		panic(ms.vm.NewGoError(errAlreadyStarted))
	}
	spec, ok := call.Argument(0).Export().(map[string]any)
	if !ok {
		panic(ms.vm.NewTypeError("mcp.tool: a spec object is required"))
	}
	name, _ := spec["name"].(string)
	if name == "" {
		panic(ms.vm.NewTypeError("mcp.tool: `name` is required"))
	}
	desc, _ := spec["description"].(string)

	hv := ms.vm.ToValue(spec["handler"])
	fn, isFn := goja.AssertFunction(hv)
	if !isFn {
		panic(ms.vm.NewTypeError("mcp.tool: `handler` must be a function"))
	}
	lc := scriptengine.NewLoopCallable(ms.loop, fn)

	// InputSchema/OutputSchema are `any` on mcp.Tool; the JS-provided schema
	// objects pass straight through (the SDK marshals them as-is on the wire).
	tool := &mcp.Tool{Name: name, Description: desc, InputSchema: spec["inputSchema"]}
	if os, has := spec["outputSchema"]; has {
		tool.OutputSchema = os
	}

	ms.srv.AddTool(tool, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// Captured off-loop (native Go field access); newRequestContext itself
		// runs on-loop inside buildArgs below, since it allocates goja values.
		sess := req.Session
		// Server-side Arguments is json.RawMessage, not `any` — unmarshal to a
		// native Go value before it reaches goja (which vm.ToValue can convert).
		var args any
		if len(req.Params.Arguments) > 0 {
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, err
			}
		}
		out, err := ms.callJSHandler(lc,
			func(vm *goja.Runtime) []goja.Value {
				return []goja.Value{vm.ToValue(args), ms.newRequestContext(vm, sess)}
			},
			func(vm *goja.Runtime, v goja.Value) (any, error) {
				return toToolResult(vm, v), nil
			},
		)
		if err != nil {
			// A thrown/rejected handler is a tool-level failure, reported as an
			// isError result rather than a protocol error.
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			}, nil
		}
		// convert always yields *mcp.CallToolResult on the success path; the
		// comma-ok guard keeps a future convert change from panicking here.
		result, ok := out.(*mcp.CallToolResult)
		if !ok {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "mcp: internal tool result conversion failed"}},
			}, nil
		}
		return result, nil
	})
	return goja.Undefined()
}

// jsResource implements srv.resource({ uri, name, mimeType?, read }). It
// registers a real SDK resource whose ResourceHandler bridges into the JS
// `read` function through callJSHandler — the same mechanism jsTool uses,
// swapping the SDK call (AddResource instead of AddTool) and the result
// converter (toReadResourceResult instead of toToolResult).
//
// Unlike a tool handler's error (surfaced as an isError CallToolResult), a
// resource read error is a protocol-level failure: the handler below returns
// (nil, err) straight to the SDK rather than wrapping it in a result value.
//
// Registration must happen before the server is serving, for the same
// list-changed-notification reason documented on jsTool.
func (ms *mcpServer) jsResource(call goja.FunctionCall) goja.Value {
	if ms.started {
		panic(ms.vm.NewGoError(errAlreadyStarted))
	}
	spec, ok := call.Argument(0).Export().(map[string]any)
	if !ok {
		panic(ms.vm.NewTypeError("mcp.resource: a spec object is required"))
	}
	uri, _ := spec["uri"].(string)
	if uri == "" {
		panic(ms.vm.NewTypeError("mcp.resource: `uri` is required"))
	}
	name, _ := spec["name"].(string)
	if name == "" {
		panic(ms.vm.NewTypeError("mcp.resource: `name` is required"))
	}
	mimeType, _ := spec["mimeType"].(string)

	hv := ms.vm.ToValue(spec["read"])
	fn, isFn := goja.AssertFunction(hv)
	if !isFn {
		panic(ms.vm.NewTypeError("mcp.resource: `read` must be a function"))
	}
	lc := scriptengine.NewLoopCallable(ms.loop, fn)

	ms.srv.AddResource(&mcp.Resource{URI: uri, Name: name, MIMEType: mimeType}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		sess := req.Session
		requestedURI := req.Params.URI
		out, err := ms.callJSHandler(lc,
			func(vm *goja.Runtime) []goja.Value {
				return []goja.Value{vm.ToValue(requestedURI), ms.newRequestContext(vm, sess)}
			},
			func(vm *goja.Runtime, v goja.Value) (any, error) {
				return toReadResourceResult(vm, requestedURI, mimeType, v)
			},
		)
		if err != nil {
			// A resource read failure is a protocol-level error, not an
			// isError result (there's no such shape for resources/read) — it
			// propagates straight to the SDK's error response.
			return nil, err
		}
		// convert always yields *mcp.ReadResourceResult on the success path;
		// the comma-ok guard keeps a future convert change from panicking.
		result, ok := out.(*mcp.ReadResourceResult)
		if !ok {
			return nil, errors.New("mcp: internal resource result conversion failed")
		}
		return result, nil
	})
	return goja.Undefined()
}

// jsPrompt implements srv.prompt({ name, description?, arguments?, get }). It
// registers a real SDK prompt whose PromptHandler bridges into the JS `get`
// function through callJSHandler — the same mechanism jsTool/jsResource use,
// swapping the SDK call (AddPrompt instead of AddTool/AddResource) and the
// result converter (toGetPromptResult instead of toToolResult/
// toReadResourceResult).
//
// Like a resource read error, a prompt get error is a protocol-level
// failure: the handler below returns (nil, err) straight to the SDK rather
// than wrapping it in a result value (there's no isError-equivalent shape
// for prompts/get, same as resources/read).
//
// Registration must happen before the server is serving, for the same
// list-changed-notification reason documented on jsTool.
func (ms *mcpServer) jsPrompt(call goja.FunctionCall) goja.Value {
	if ms.started {
		panic(ms.vm.NewGoError(errAlreadyStarted))
	}
	spec, ok := call.Argument(0).Export().(map[string]any)
	if !ok {
		panic(ms.vm.NewTypeError("mcp.prompt: a spec object is required"))
	}
	name, _ := spec["name"].(string)
	if name == "" {
		panic(ms.vm.NewTypeError("mcp.prompt: `name` is required"))
	}
	desc, _ := spec["description"].(string)

	var args []*mcp.PromptArgument
	if rawArgs, has := spec["arguments"]; has {
		list, ok := rawArgs.([]any)
		if !ok {
			panic(ms.vm.NewTypeError("mcp.prompt: `arguments` must be an array"))
		}
		for i, item := range list {
			am, ok := item.(map[string]any)
			if !ok {
				panic(ms.vm.NewTypeError(fmt.Sprintf("mcp.prompt: arguments[%d] must be an object", i)))
			}
			argName, _ := am["name"].(string)
			if argName == "" {
				panic(ms.vm.NewTypeError(fmt.Sprintf("mcp.prompt: arguments[%d].name is required", i)))
			}
			argDesc, _ := am["description"].(string)
			required, _ := am["required"].(bool)
			args = append(args, &mcp.PromptArgument{Name: argName, Description: argDesc, Required: required})
		}
	}

	hv := ms.vm.ToValue(spec["get"])
	fn, isFn := goja.AssertFunction(hv)
	if !isFn {
		panic(ms.vm.NewTypeError("mcp.prompt: `get` must be a function"))
	}
	lc := scriptengine.NewLoopCallable(ms.loop, fn)

	ms.srv.AddPrompt(&mcp.Prompt{Name: name, Description: desc, Arguments: args}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		sess := req.Session
		requestedArgs := req.Params.Arguments
		out, err := ms.callJSHandler(lc,
			func(vm *goja.Runtime) []goja.Value {
				return []goja.Value{vm.ToValue(requestedArgs), ms.newRequestContext(vm, sess)}
			},
			func(vm *goja.Runtime, v goja.Value) (any, error) {
				return toGetPromptResult(vm, v)
			},
		)
		if err != nil {
			// A prompt get failure is a protocol-level error, not an isError
			// result (there's no such shape for prompts/get) — it propagates
			// straight to the SDK's error response.
			return nil, err
		}
		// convert always yields *mcp.GetPromptResult on the success path; the
		// comma-ok guard keeps a future convert change from panicking here.
		result, ok := out.(*mcp.GetPromptResult)
		if !ok {
			return nil, errors.New("mcp: internal prompt result conversion failed")
		}
		return result, nil
	})
	return goja.Undefined()
}

// The methods below are intentional stubs: the stdio/listen transports are
// filled in by later tasks. close() is a no-op until a transport (and
// therefore a HoldRun) exists to release.
func (ms *mcpServer) jsStdio(call goja.FunctionCall) goja.Value {
	panic(ms.vm.NewTypeError("mcp: stdio() not yet implemented"))
}
func (ms *mcpServer) jsListen(call goja.FunctionCall) goja.Value {
	panic(ms.vm.NewTypeError("mcp: listen() not yet implemented"))
}
func (ms *mcpServer) jsClose(call goja.FunctionCall) goja.Value { return goja.Undefined() }
