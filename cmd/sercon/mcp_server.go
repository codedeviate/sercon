package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"

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
	release func()       // HoldRun release, set when a transport starts; cleared on serve end (and by close())
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

// jsStdio implements srv.stdio(): serve the MCP server over stdin/stdout using
// newline-delimited JSON-RPC, returning a Promise that resolves when the peer
// disconnects (stdin closes / the session ends).
//
// The critical contract is that stdout carries ONLY JSON-RPC — any of sercon's
// own output (console.log, runtime.log, stray writes) leaking onto stdout would
// corrupt the framing and break every client. Because goja_nodejs's console
// captured the original stdout at package-init time, a Go-level `os.Stdout`
// swap is not enough; installStdoutRedirect remaps fd 1 to stderr (unix) and
// hands back an *os.File bound to the real stdout, which we give to the SDK's
// IOTransport as the JSON-RPC writer. StdioTransport can't be used here — it
// hard-wires os.Stdin/os.Stdout and offers no seam to separate the two streams
// after the redirect, so we build an IOTransport explicitly.
//
// The hold keeps loop.Run alive for the serve duration; srv.Run blocks on its
// own goroutine and the settlement (restore fd, release hold, resolve/reject)
// is marshalled back onto the loop.
func (ms *mcpServer) jsStdio(_ goja.FunctionCall) goja.Value {
	if ms.started {
		panic(ms.vm.NewGoError(errAlreadyStarted))
	}
	ms.started = true

	p, resolve, reject := ms.vm.NewPromise()
	ms.release = ms.eng.HoldRun("mcp:stdio")

	// Normal CLI path: mcp.serve() already armed+installed the redirect (so any
	// output written between serve() and here already went to stderr), and the
	// CLI's disarm owns the restore — reuse the saved real stdout. Fallback path
	// (guard not armed, e.g. an in-process caller): install a redirect here and
	// restore it ourselves when the serve loop ends.
	realStdout := mcpStdioGuard.real
	var localRestore func() error
	if realStdout == nil {
		r, restore, err := installStdoutRedirect()
		if err != nil {
			if ms.release != nil {
				ms.release()
				ms.release = nil
			}
			_ = reject(ms.vm.NewGoError(err))
			return ms.vm.ToValue(p)
		}
		realStdout, localRestore = r, restore
	}

	transport := &mcp.IOTransport{
		Reader: os.Stdin,
		// Wrap the saved real stdout so the SDK closing the connection can't
		// close the fd out from under the restore step; the restore owner
		// (CLI disarm, or localRestore below) closes it exactly once.
		Writer: nopWriteCloser{realStdout},
	}

	go func() {
		runErr := ms.srv.Run(context.Background(), transport)
		ms.loop.RunOnLoop(func(*goja.Runtime) {
			if localRestore != nil {
				_ = localRestore()
			}
			if ms.release != nil {
				ms.release()
				ms.release = nil
			}
			if runErr != nil {
				_ = reject(ms.vm.NewGoError(runErr))
			} else {
				_ = resolve(goja.Undefined())
			}
		})
	}()

	return ms.vm.ToValue(p)
}

// nopWriteCloser adapts an io.Writer to io.WriteCloser with a no-op Close, so
// the MCP transport never closes the underlying real-stdout file (restore()
// closes it exactly once).
type nopWriteCloser struct{ io.Writer }

func (nopWriteCloser) Close() error { return nil }

// jsListen implements srv.listen({ port, host?, path? }): serve the MCP
// server over the Streamable HTTP transport (the cross-platform transport —
// unlike stdio, any number of clients/browsers can connect to a TCP
// endpoint). Mounts mcp.NewStreamableHTTPHandler on an *http.ServeMux at
// `path` (default "/mcp"), binds + Serves in a goroutine, and returns a
// Promise resolving to a handle `{ url, close() }`.
//
// Unlike jsStdio (which blocks until the peer disconnects), listen's Promise
// resolves as soon as the listener is bound — the handle's `close()` is what
// later resolves once shutdown completes. This mirrors server.http.listen's
// synchronous-bind-then-handle shape (see server_http.go's httpListen), just
// wrapped in a real Promise per this binding's documented interface.
//
// No stdout redirect here: HTTP has no stdout-framing conflict (that
// constraint is stdio-only, see jsStdio's doc comment).
func (ms *mcpServer) jsListen(call goja.FunctionCall) goja.Value {
	if ms.started {
		panic(ms.vm.NewGoError(errAlreadyStarted))
	}

	opts := call.Argument(0)
	if opts == nil || goja.IsUndefined(opts) || goja.IsNull(opts) {
		panic(ms.vm.NewTypeError("mcp: listen: options object required"))
	}
	optsObj := opts.ToObject(ms.vm)

	portVal := optsObj.Get("port")
	if portVal == nil || goja.IsUndefined(portVal) {
		panic(ms.vm.NewTypeError("mcp: listen: `port` is required"))
	}
	port := int(portVal.ToInteger())

	host := "127.0.0.1"
	if v := optsObj.Get("host"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		if s := v.String(); s != "" {
			host = s
		}
	}
	path := "/mcp"
	if v := optsObj.Get("path"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		if s := v.String(); s != "" {
			path = s
		}
	}

	getServer := func(*http.Request) *mcp.Server { return ms.srv }
	streamableHandler := mcp.NewStreamableHTTPHandler(getServer, nil)
	mux := http.NewServeMux()
	mux.Handle(path, streamableHandler)

	addr := net.JoinHostPort(host, strconv.Itoa(port))
	httpSrv := &http.Server{Addr: addr, Handler: mux}

	// Bind synchronously so the script learns about port-in-use errors
	// immediately (mirrors httpListen in server_http.go). A bind failure
	// means no transport actually started, so — unlike a post-bind failure —
	// ms.started is not set, letting the script retry listen() with a
	// different port on the same handle.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		panic(ms.vm.NewGoError(fmt.Errorf("mcp: listen %s: %w", addr, err)))
	}
	ms.started = true

	tcpAddr, _ := ln.Addr().(*net.TCPAddr)
	actualPort := port
	if tcpAddr != nil {
		actualPort = tcpAddr.Port
	}
	urlHost := host
	if urlHost == "0.0.0.0" || urlHost == "::" || urlHost == "" {
		urlHost = "127.0.0.1"
	}
	url := fmt.Sprintf("http://%s%s", net.JoinHostPort(urlHost, strconv.Itoa(actualPort)), path)

	ms.release = ms.eng.HoldRun("mcp:http")

	// stoppedPromise settles exactly once, from the Serve goroutine below,
	// regardless of *why* Serve() returns — an explicit close() (clean
	// shutdown, resolves) or Serve() failing on its own (e.g. an accept-loop
	// error, rejects). This mirrors httpListen's single-settle-point design
	// in server_http.go: close() itself never releases the hold or settles
	// the promise directly, it only asks Shutdown to unblock Serve(), which
	// stays the sole authority over the promise/hold lifecycle no matter
	// which path triggered the exit. That keeps a post-bind failure (Serve
	// exiting without close() ever being called) from leaking the hold or
	// leaving the script waiting on a promise that never settles.
	stoppedPromise, stoppedResolve, stoppedReject := ms.vm.NewPromise()
	closed := atomic.Bool{}

	go func() {
		err := httpSrv.Serve(ln)
		ms.loop.RunOnLoop(func(vm *goja.Runtime) {
			// Guard against a future second settle point; today Serve()
			// only ever returns once, so this fires exactly once, but the
			// nil-check mirrors the release-guard idiom used elsewhere
			// (jsStdio, closeFn) rather than assuming that invariant.
			if ms.release != nil {
				ms.release()
				ms.release = nil
			}
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				_ = stoppedReject(vm.NewGoError(err))
				return
			}
			_ = stoppedResolve(goja.Undefined())
		})
	}()

	closeFn := func(goja.FunctionCall) goja.Value {
		if closed.Swap(true) {
			return ms.vm.ToValue(stoppedPromise)
		}
		go func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = httpSrv.Shutdown(shutdownCtx)
			// stoppedResolve/stoppedReject (and the hold release) fire from
			// the Serve goroutine above, once Shutdown unblocks it.
		}()
		return ms.vm.ToValue(stoppedPromise)
	}

	handle := ms.vm.NewObject()
	_ = handle.Set("url", url)
	_ = handle.Set("stopped", stoppedPromise)
	_ = handle.Set("close", closeFn)

	p, resolve, _ := ms.vm.NewPromise()
	_ = resolve(handle)
	return ms.vm.ToValue(p)
}

func (ms *mcpServer) jsClose(call goja.FunctionCall) goja.Value { return goja.Undefined() }
