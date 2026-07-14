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
	"sync"
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
	release func()       // HoldRun release, set when a transport starts; cleared when that transport's serve loop ends (jsStdio's/jsListen's own goroutine clears it — jsClose is currently a no-op and does NOT clear it)
	reqSeq  atomic.Int64 // monotonic counter backing newRequestContext's requestId

	// Resource-subscription state (Task 5). subscribeMu guards all three
	// fields below: subscribed, onSubscribeCB, onUnsubscribeCB. They are
	// touched from both the main script goroutine (on-loop, via
	// jsOnSubscribe/jsOnUnsubscribe registering a callback) and the go-sdk's
	// own request-handling goroutines (via the SubscribeHandler/
	// UnsubscribeHandler dispatchers set in mcp.serve, which record into
	// `subscribed` and read the callbacks) — hence the mutex rather than
	// relying on the event loop's single-threadedness the way most of this
	// file's goja access does.
	subscribeMu     sync.Mutex
	subscribed      map[string]struct{}
	onSubscribeCB   *scriptengine.LoopCallable
	onUnsubscribeCB *scriptengine.LoopCallable
}

// recordSubscribe marks uri as subscribed in ms's own tracking set. Called
// from the SDK's SubscribeHandler dispatcher (an SDK goroutine, not the
// loop) — mutex-guarded because it races jsOnSubscribe/jsOnUnsubscribe
// registering a callback from the main script. Note the go-sdk already
// tracks per-session subscriptions itself (resourceSubscriptions in
// server.go) and filters ResourceUpdated's notification fan-out by it; this
// set is purely ms's own bookkeeping, independent of that internal state.
func (ms *mcpServer) recordSubscribe(uri string) {
	ms.subscribeMu.Lock()
	defer ms.subscribeMu.Unlock()
	if ms.subscribed == nil {
		ms.subscribed = make(map[string]struct{})
	}
	ms.subscribed[uri] = struct{}{}
}

// recordUnsubscribe removes uri from ms's tracking set. See recordSubscribe.
func (ms *mcpServer) recordUnsubscribe(uri string) {
	ms.subscribeMu.Lock()
	defer ms.subscribeMu.Unlock()
	delete(ms.subscribed, uri)
}

// getOnSubscribe/getOnUnsubscribe return the currently-registered JS
// callback (nil if none was ever set via jsOnSubscribe/jsOnUnsubscribe),
// mutex-guarded for the same cross-goroutine reason as recordSubscribe.
func (ms *mcpServer) getOnSubscribe() *scriptengine.LoopCallable {
	ms.subscribeMu.Lock()
	defer ms.subscribeMu.Unlock()
	return ms.onSubscribeCB
}

func (ms *mcpServer) getOnUnsubscribe() *scriptengine.LoopCallable {
	ms.subscribeMu.Lock()
	defer ms.subscribeMu.Unlock()
	return ms.onUnsubscribeCB
}

// setOnSubscribe/setOnUnsubscribe store the JS callback registered via
// srv.onSubscribe(fn)/srv.onUnsubscribe(fn). Called on-loop (jsOnSubscribe/
// jsOnUnsubscribe run as goja bindings), but still mutex-guarded since the
// SDK's dispatcher goroutines read the same field concurrently.
func (ms *mcpServer) setOnSubscribe(lc *scriptengine.LoopCallable) {
	ms.subscribeMu.Lock()
	defer ms.subscribeMu.Unlock()
	ms.onSubscribeCB = lc
}

func (ms *mcpServer) setOnUnsubscribe(lc *scriptengine.LoopCallable) {
	ms.subscribeMu.Lock()
	defer ms.subscribeMu.Unlock()
	ms.onUnsubscribeCB = lc
}

// newRequestContext builds the `ctx` object passed as the 2nd argument to
// every tool/resource/prompt JS handler: { requestId, clientInfo: { name,
// version }, progress(progress, total?), log(level, message, data?) }.
// Phase 3 (sample/elicit) hooks in here later, holding onto sess for the SDK
// calls that need it (CreateMessage, Elicit) — none of that exists yet.
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
// tok is the request's progress token (req.Params.GetProgressToken()),
// captured by the caller off-loop before invoking the JS handler — it's nil
// when the client didn't attach one to this call.
//
// This must be called ON the event loop (from a buildArgs callback, which
// callJSHandler always invokes on-loop) since it allocates goja values. The
// progress/log closures built here only do their actual SDK I/O later, off
// the loop, when the script calls them — see jsCtxProgress/jsCtxLog.
func (ms *mcpServer) newRequestContext(vm *goja.Runtime, sess *mcp.ServerSession, tok any) goja.Value {
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

	_ = ctxObj.Set("progress", ms.jsCtxProgress(sess, tok))
	_ = ctxObj.Set("log", ms.jsCtxLog(sess))

	return ctxObj
}

// jsCtxProgress builds ctx.progress(progress: number, total?: number):
// Promise<void> for the request context: sends a notifications/progress
// message to the client, correlated to this request via tok (the progress
// token the client attached to its call, captured by the caller before
// invoking the JS handler). Per the MCP spec, a progress notification only
// makes sense when the request carried a token to correlate it to — if the
// client didn't set one, tok is nil and this resolves immediately without
// calling the SDK (same for a nil sess, which shouldn't happen in practice
// but is guarded defensively).
//
// progress/total are read synchronously here (the function itself runs
// on-loop, like any goja call), but the SDK call happens in a goroutine —
// sess.NotifyProgress does its own network/pipe I/O and must never block
// goja. The spawned goroutine settles the returned Promise back on the loop
// via RunOnLoop, mirroring jsStdio/jsListen's async-settle pattern elsewhere
// in this file (resolve/reject from a goroutine-owned RunOnLoop callback). A
// deferred recover in that callback turns a settle-time panic into a
// rejection instead of crashing the eventloop's job runner (which has no
// recover of its own), matching callJSHandler's precedent in mcp_bridge.go.
func (ms *mcpServer) jsCtxProgress(sess *mcp.ServerSession, tok any) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		p, resolve, reject := ms.vm.NewPromise()

		if tok == nil || sess == nil {
			_ = resolve(goja.Undefined())
			return ms.vm.ToValue(p)
		}

		progress := call.Argument(0).ToFloat()
		var total float64
		if tv := call.Argument(1); tv != nil && !goja.IsUndefined(tv) && !goja.IsNull(tv) {
			total = tv.ToFloat()
		}

		go func() {
			notifyErr := sess.NotifyProgress(context.Background(), &mcp.ProgressNotificationParams{
				ProgressToken: tok,
				Progress:      progress,
				Total:         total,
			})
			ms.loop.RunOnLoop(func(vm *goja.Runtime) {
				defer func() {
					if r := recover(); r != nil {
						_ = reject(vm.NewGoError(fmt.Errorf("mcp: ctx.progress settle panicked: %v", r)))
					}
				}()
				if notifyErr != nil {
					_ = reject(vm.NewGoError(notifyErr))
					return
				}
				_ = resolve(goja.Undefined())
			})
		}()

		return ms.vm.ToValue(p)
	}
}

// jsCtxLog builds ctx.log(level: string, message: string, data?: unknown):
// Promise<void> for the request context: sends a notifications/message log
// entry to the client via sess.Log.
//
// GOTCHA (confirmed by the Task-1 API spike, mcp_spike2_test.go): the go-sdk
// silently no-ops sess.Log until the client has called
// session.SetLoggingLevel — see (*ServerSession).Log in server.go ("The
// spec is unclear, but seems to imply that no log messages are sent until
// the client sets the level"). Callers/tests must call SetLoggingLevel
// first or a log assertion is vacuous (the Promise still resolves — Log
// returns nil in that case — but nothing reaches the client).
//
// mcp.LoggingMessageParams.Data is `any` (documented as "any JSON
// serializable type"), so message and the optional data argument are
// combined into a single object: { message, data } — data is omitted
// entirely when the script didn't pass a third argument, keeping the
// payload minimal when there's nothing beyond the message.
//
// Same off-loop-I/O / on-loop-settle shape as jsCtxProgress: the SDK call
// runs in a goroutine, and the goroutine settles the Promise back on the
// loop via RunOnLoop with a deferred recover guarding the settle callback.
func (ms *mcpServer) jsCtxLog(sess *mcp.ServerSession) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		p, resolve, reject := ms.vm.NewPromise()

		if sess == nil {
			_ = resolve(goja.Undefined())
			return ms.vm.ToValue(p)
		}

		level := call.Argument(0).String()
		message := call.Argument(1).String()
		payload := map[string]any{"message": message}
		if dv := call.Argument(2); dv != nil && !goja.IsUndefined(dv) {
			payload["data"] = dv.Export()
		}

		go func() {
			logErr := sess.Log(context.Background(), &mcp.LoggingMessageParams{
				Level: mcp.LoggingLevel(level),
				Data:  payload,
			})
			ms.loop.RunOnLoop(func(vm *goja.Runtime) {
				defer func() {
					if r := recover(); r != nil {
						_ = reject(vm.NewGoError(fmt.Errorf("mcp: ctx.log settle panicked: %v", r)))
					}
				}()
				if logErr != nil {
					_ = reject(vm.NewGoError(logErr))
					return
				}
				_ = resolve(goja.Undefined())
			})
		}()

		return ms.vm.ToValue(p)
	}
}

// jsTool implements srv.tool({ name, description?, inputSchema, outputSchema?,
// handler }). It registers a real SDK tool whose ToolHandler bridges into the
// JS handler through callJSHandler: the SDK invokes the handler on its own
// goroutine, callJSHandler hops onto the event loop to run the JS function,
// and the result is converted to an *mcp.CallToolResult ON the loop (via
// toToolResult) before it crosses back — so no goja.Value ever escapes the
// loop goroutine.
//
// Registration is allowed at any time, including after a transport has
// started: the SDK's AddTool fires a tools/list_changed notification to
// connected clients when called post-connect, so a script can grow its tool
// set at runtime (e.g. from within another handler). See jsRemoveTool for the
// inverse operation.
func (ms *mcpServer) jsTool(call goja.FunctionCall) goja.Value {
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
		tok := req.Params.GetProgressToken()
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
				return []goja.Value{vm.ToValue(args), ms.newRequestContext(vm, sess, tok)}
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
// Registration is allowed at any time, for the same runtime-add /
// list-changed-notification reason documented on jsTool. See jsRemoveResource
// for the inverse operation.
func (ms *mcpServer) jsResource(call goja.FunctionCall) goja.Value {
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
		tok := req.Params.GetProgressToken()
		requestedURI := req.Params.URI
		out, err := ms.callJSHandler(lc,
			func(vm *goja.Runtime) []goja.Value {
				return []goja.Value{vm.ToValue(requestedURI), ms.newRequestContext(vm, sess, tok)}
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

// jsResourceTemplate implements
// srv.resourceTemplate({ uriTemplate, name, mimeType?, read }). It mirrors
// jsResource exactly, swapping the SDK call (AddResourceTemplate instead of
// AddResource, keyed by an RFC-6570 URITemplate rather than a fixed URI) —
// the ResourceHandler bridge, error-propagation shape, and result converter
// (toReadResourceResult) are identical. The client reads a concrete URI that
// matches the template (e.g. "db:///users/42" against "db:///{table}/{id}");
// that resolved URI — not the template string — is what's passed to the JS
// `read` function and stamped onto the returned ResourceContents.
//
// Registration is allowed at any time, for the same runtime-add /
// list-changed-notification reason documented on jsTool.
func (ms *mcpServer) jsResourceTemplate(call goja.FunctionCall) goja.Value {
	spec, ok := call.Argument(0).Export().(map[string]any)
	if !ok {
		panic(ms.vm.NewTypeError("mcp.resourceTemplate: a spec object is required"))
	}
	uriTemplate, _ := spec["uriTemplate"].(string)
	if uriTemplate == "" {
		panic(ms.vm.NewTypeError("mcp.resourceTemplate: `uriTemplate` is required"))
	}
	name, _ := spec["name"].(string)
	if name == "" {
		panic(ms.vm.NewTypeError("mcp.resourceTemplate: `name` is required"))
	}
	mimeType, _ := spec["mimeType"].(string)

	hv := ms.vm.ToValue(spec["read"])
	fn, isFn := goja.AssertFunction(hv)
	if !isFn {
		panic(ms.vm.NewTypeError("mcp.resourceTemplate: `read` must be a function"))
	}
	lc := scriptengine.NewLoopCallable(ms.loop, fn)

	ms.srv.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: uriTemplate, Name: name, MIMEType: mimeType}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		sess := req.Session
		tok := req.Params.GetProgressToken()
		requestedURI := req.Params.URI
		out, err := ms.callJSHandler(lc,
			func(vm *goja.Runtime) []goja.Value {
				return []goja.Value{vm.ToValue(requestedURI), ms.newRequestContext(vm, sess, tok)}
			},
			func(vm *goja.Runtime, v goja.Value) (any, error) {
				return toReadResourceResult(vm, requestedURI, mimeType, v)
			},
		)
		if err != nil {
			// Same protocol-level error shape as jsResource: there's no
			// isError equivalent for resources/read.
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
// Registration is allowed at any time, for the same runtime-add /
// list-changed-notification reason documented on jsTool. See jsRemovePrompt
// for the inverse operation.
func (ms *mcpServer) jsPrompt(call goja.FunctionCall) goja.Value {
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
		tok := req.Params.GetProgressToken()
		requestedArgs := req.Params.Arguments
		out, err := ms.callJSHandler(lc,
			func(vm *goja.Runtime) []goja.Value {
				return []goja.Value{vm.ToValue(requestedArgs), ms.newRequestContext(vm, sess, tok)}
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

// requireNonEmptyStringArg validates that call's first argument is a present,
// non-null, non-empty string, panicking with a goja TypeError (labelled with
// `who`, e.g. "mcp.removeTool") otherwise. Shared by jsRemoveTool/
// jsRemoveResource/jsRemovePrompt, whose single-string-arg shape doesn't go
// through the map[string]any spec-object validation jsTool/jsResource/
// jsPrompt use.
func (ms *mcpServer) requireNonEmptyStringArg(who string, call goja.FunctionCall) string {
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		panic(ms.vm.NewTypeError(fmt.Sprintf("%s: a name is required", who)))
	}
	name := arg.String()
	if name == "" {
		panic(ms.vm.NewTypeError(fmt.Sprintf("%s: a non-empty name is required", who)))
	}
	return name
}

// jsRemoveTool implements srv.removeTool(name): unregisters a previously
// added tool by name. Like jsTool, this may be called at any time — before or
// after a transport has started — and the SDK's RemoveTools fires a
// tools/list_changed notification to connected clients when called
// post-connect. Removing a name that was never registered (or already
// removed) is a silent no-op, matching (*mcp.Server).RemoveTools' own
// contract.
//
// No event-loop hop is needed around the ms.srv.RemoveTools call itself: the
// go-sdk source (mcp/server.go, changeAndNotify) guards every Add*/Remove*
// mutation with the Server's own internal mutex, so it's safe to call from
// whichever goroutine reaches this line — the main script (already on-loop,
// since goja execution only ever happens on the loop) or a JS handler
// invoked via callJSHandler (also hopped onto the loop by the time it runs
// JS). The mutex is what actually serializes concurrent mutation/notify
// against the SDK's own request-dispatch goroutines, not anything on our side.
func (ms *mcpServer) jsRemoveTool(call goja.FunctionCall) goja.Value {
	name := ms.requireNonEmptyStringArg("mcp.removeTool", call)
	ms.srv.RemoveTools(name)
	return goja.Undefined()
}

// jsRemoveResource implements srv.removeResource(uri): unregisters a
// previously added resource by URI. Same runtime/concurrency contract as
// jsRemoveTool, backed by (*mcp.Server).RemoveResources.
func (ms *mcpServer) jsRemoveResource(call goja.FunctionCall) goja.Value {
	uri := ms.requireNonEmptyStringArg("mcp.removeResource", call)
	ms.srv.RemoveResources(uri)
	return goja.Undefined()
}

// jsRemovePrompt implements srv.removePrompt(name): unregisters a previously
// added prompt by name. Same runtime/concurrency contract as jsRemoveTool,
// backed by (*mcp.Server).RemovePrompts.
func (ms *mcpServer) jsRemovePrompt(call goja.FunctionCall) goja.Value {
	name := ms.requireNonEmptyStringArg("mcp.removePrompt", call)
	ms.srv.RemovePrompts(name)
	return goja.Undefined()
}

// requireFunctionArg validates that call's first argument is a callable JS
// function, panicking with a goja TypeError (labelled with `who`) otherwise.
// Shared by jsOnSubscribe/jsOnUnsubscribe.
func (ms *mcpServer) requireFunctionArg(who string, call goja.FunctionCall) goja.Callable {
	fn, isFn := goja.AssertFunction(call.Argument(0))
	if !isFn {
		panic(ms.vm.NewTypeError(fmt.Sprintf("%s: a function is required", who)))
	}
	return fn
}

// jsOnSubscribe implements srv.onSubscribe(fn): registers a best-effort JS
// hook invoked whenever a client subscribes to a resource (via the
// SubscribeHandler dispatcher set in mcp.serve). fn receives the subscribed
// URI as its sole argument; its return value (and any error/rejection) is
// ignored — see the SubscribeHandler dispatcher's doc comment for why this
// is deliberately fire-and-forget rather than able to fail the subscribe.
//
// Only one onSubscribe callback is held at a time; a later call to
// srv.onSubscribe replaces the earlier registration (last-writer-wins,
// same as this handle's other single-slot registrations).
func (ms *mcpServer) jsOnSubscribe(call goja.FunctionCall) goja.Value {
	fn := ms.requireFunctionArg("mcp.onSubscribe", call)
	ms.setOnSubscribe(scriptengine.NewLoopCallable(ms.loop, fn))
	return goja.Undefined()
}

// jsOnUnsubscribe implements srv.onUnsubscribe(fn): the unsubscribe-side
// mirror of jsOnSubscribe, invoked whenever a client unsubscribes (via the
// UnsubscribeHandler dispatcher set in mcp.serve). Same fire-and-forget,
// last-writer-wins contract.
func (ms *mcpServer) jsOnUnsubscribe(call goja.FunctionCall) goja.Value {
	fn := ms.requireFunctionArg("mcp.onUnsubscribe", call)
	ms.setOnUnsubscribe(scriptengine.NewLoopCallable(ms.loop, fn))
	return goja.Undefined()
}

// jsResourceUpdated implements srv.resourceUpdated(uri): Promise<void>,
// notifying every client currently subscribed to uri that the resource has
// changed. This is the primary way a script signals a resource update — the
// go-sdk's Server.ResourceUpdated does the actual subscriber lookup and
// notification fan-out (see recordSubscribe's doc comment: the SDK tracks
// per-session subscriptions itself, independent of ms's own bookkeeping
// set).
//
// Same off-loop-I/O / on-loop-settle shape as jsCtxProgress/jsCtxLog: the
// SDK call runs in a goroutine (ResourceUpdated does network/pipe I/O and
// must never block goja), and the goroutine settles the returned Promise
// back on the loop via RunOnLoop, with a deferred recover guarding the
// settle callback so a panic there rejects instead of crashing the
// eventloop's job runner.
func (ms *mcpServer) jsResourceUpdated(call goja.FunctionCall) goja.Value {
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		panic(ms.vm.NewTypeError("mcp.resourceUpdated: a uri is required"))
	}
	uri := arg.String()
	if uri == "" {
		panic(ms.vm.NewTypeError("mcp.resourceUpdated: a non-empty uri is required"))
	}

	p, resolve, reject := ms.vm.NewPromise()

	go func() {
		notifyErr := ms.srv.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: uri})
		ms.loop.RunOnLoop(func(vm *goja.Runtime) {
			defer func() {
				if r := recover(); r != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("mcp: resourceUpdated settle panicked: %v", r)))
				}
			}()
			if notifyErr != nil {
				_ = reject(vm.NewGoError(notifyErr))
				return
			}
			_ = resolve(goja.Undefined())
		})
	}()

	return ms.vm.ToValue(p)
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

// jsClose is srv.close(): present on the handle for interface symmetry with
// server.http.listen's close(), but currently an inert no-op — it does not
// stop a running transport. Stopping actually happens per-transport: an HTTP
// listener is stopped via the close() on jsListen's returned handle, and a
// stdio server stops on its own once the peer disconnects (jsStdio's promise
// settles then). Wiring this into an explicit "stop whichever transport is
// running" shutdown is later-phase work.
func (ms *mcpServer) jsClose(call goja.FunctionCall) goja.Value { return goja.Undefined() }
