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

	// handlerTimeout is the per-call deadline callJSHandler applies to a tool/
	// resource/prompt/completion/verify handler (from mcp.serve's handlerTimeout
	// option; default mcpDefaultHandlerTimeout, 0 disables).
	handlerTimeout time.Duration

	// Resource-subscription state (Task 5). subscribeMu guards both fields
	// below: onSubscribeCB, onUnsubscribeCB. They are touched from both the
	// main script goroutine (on-loop, via jsOnSubscribe/jsOnUnsubscribe
	// registering a callback) and the go-sdk's own request-handling
	// goroutines (via the SubscribeHandler/UnsubscribeHandler dispatchers set
	// in mcp.serve, which read the callbacks) — hence the mutex rather than
	// relying on the event loop's single-threadedness the way most of this
	// file's goja access does.
	//
	// There is deliberately no `subscribed` set here: the go-sdk already
	// tracks per-session subscriptions itself (resourceSubscriptions in
	// server.go) and filters ResourceUpdated's notification fan-out by it,
	// and capability advertisement is gated on SubscribeHandler != nil, not
	// on any bookkeeping of ours — a prior write-only `subscribed` map (and
	// its recordSubscribe/recordUnsubscribe helpers) was removed as dead
	// weight; see the code review that flagged it.
	subscribeMu     sync.Mutex
	onSubscribeCB   *scriptengine.LoopCallable
	onUnsubscribeCB *scriptengine.LoopCallable

	// completionMu guards completionCB (Task 6), the JS handler registered via
	// srv.completion(fn). Same cross-goroutine reason as subscribeMu: it's set
	// on the main script goroutine (jsCompletion, on-loop) and read from the
	// go-sdk's own request-handling goroutines (the CompletionHandler
	// dispatcher set in mcp.serve). A separate mutex rather than reusing
	// subscribeMu since completion is an unrelated feature with its own
	// single field — no reason to serialize the two against each other.
	completionMu sync.Mutex
	completionCB *scriptengine.LoopCallable

	// rootsMu guards onRootsChangedCB (Task 4), the JS handler registered via
	// srv.onRootsChanged(fn). Same cross-goroutine reason as subscribeMu/
	// completionMu: set on the main script goroutine (jsOnRootsChanged,
	// on-loop) but read from the go-sdk's own request-handling goroutines
	// (the RootsListChangedHandler dispatcher set in mcp.serve). A separate
	// mutex rather than reusing subscribeMu/completionMu — unrelated feature
	// with its own single field, no reason to serialize it against them.
	rootsMu          sync.Mutex
	onRootsChangedCB *scriptengine.LoopCallable
}

// getOnSubscribe/getOnUnsubscribe return the currently-registered JS
// callback (nil if none was ever set via jsOnSubscribe/jsOnUnsubscribe),
// mutex-guarded for the same cross-goroutine reason documented on
// mcpServer's subscribeMu field.
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

// getCompletionCB returns the currently-registered srv.completion callback
// (nil if none was ever set via jsCompletion), mutex-guarded for the same
// cross-goroutine reason documented on mcpServer's completionMu field.
func (ms *mcpServer) getCompletionCB() *scriptengine.LoopCallable {
	ms.completionMu.Lock()
	defer ms.completionMu.Unlock()
	return ms.completionCB
}

// setCompletionCB stores the JS callback registered via srv.completion(fn).
// Called on-loop (jsCompletion runs as a goja binding), but still
// mutex-guarded since the SDK's CompletionHandler dispatcher goroutines read
// the same field concurrently.
func (ms *mcpServer) setCompletionCB(lc *scriptengine.LoopCallable) {
	ms.completionMu.Lock()
	defer ms.completionMu.Unlock()
	ms.completionCB = lc
}

// getOnRootsChanged returns the currently-registered srv.onRootsChanged(fn)
// callback (nil if none was ever set via jsOnRootsChanged), mutex-guarded
// for the same cross-goroutine reason documented on mcpServer's rootsMu
// field.
func (ms *mcpServer) getOnRootsChanged() *scriptengine.LoopCallable {
	ms.rootsMu.Lock()
	defer ms.rootsMu.Unlock()
	return ms.onRootsChangedCB
}

// setOnRootsChanged stores the JS callback registered via
// srv.onRootsChanged(fn). Called on-loop (jsOnRootsChanged runs as a goja
// binding), but still mutex-guarded since the SDK's RootsListChangedHandler
// dispatcher goroutine reads the same field concurrently.
func (ms *mcpServer) setOnRootsChanged(lc *scriptengine.LoopCallable) {
	ms.rootsMu.Lock()
	defer ms.rootsMu.Unlock()
	ms.onRootsChangedCB = lc
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
	_ = ctxObj.Set("sample", ms.jsCtxSample(sess))
	_ = ctxObj.Set("elicit", ms.jsCtxElicit(sess))
	_ = ctxObj.Set("roots", ms.jsCtxRoots(sess))

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
// goja. Settling is delegated to asyncSettle (see its doc comment), which
// holds the event loop open for the duration of the call — the fast path
// above (tok == nil || sess == nil) resolves synchronously and skips the
// hold entirely since there's no async tail to protect.
func (ms *mcpServer) jsCtxProgress(sess *mcp.ServerSession, tok any) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if tok == nil || sess == nil {
			p, resolve, _ := ms.vm.NewPromise()
			_ = resolve(goja.Undefined())
			return ms.vm.ToValue(p)
		}

		progress := call.Argument(0).ToFloat()
		var total float64
		if tv := call.Argument(1); tv != nil && !goja.IsUndefined(tv) && !goja.IsNull(tv) {
			total = tv.ToFloat()
		}

		return ms.asyncSettle("mcp:ctx.progress", func() error {
			return sess.NotifyProgress(context.Background(), &mcp.ProgressNotificationParams{
				ProgressToken: tok,
				Progress:      progress,
				Total:         total,
			})
		})
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
// runs in a goroutine and settling is delegated to asyncSettle, which holds
// the event loop open for the duration of the call. The fast path above
// (sess == nil) resolves synchronously and skips the hold entirely since
// there's no async tail to protect.
func (ms *mcpServer) jsCtxLog(sess *mcp.ServerSession) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if sess == nil {
			p, resolve, _ := ms.vm.NewPromise()
			_ = resolve(goja.Undefined())
			return ms.vm.ToValue(p)
		}

		level := call.Argument(0).String()
		message := call.Argument(1).String()
		payload := map[string]any{"message": message}
		if dv := call.Argument(2); dv != nil && !goja.IsUndefined(dv) {
			payload["data"] = dv.Export()
		}

		return ms.asyncSettle("mcp:ctx.log", func() error {
			return sess.Log(context.Background(), &mcp.LoggingMessageParams{
				Level: mcp.LoggingLevel(level),
				Data:  payload,
			})
		})
	}
}

// jsCtxSample builds ctx.sample(opts): Promise<{content, model, stopReason,
// role}> for the request context — the Phase-3 exemplar this task exists to
// establish: a mid-handler server->client call asking the client to run a
// completion through its own LLM (the MCP "sampling" capability), via
// sess.CreateMessage.
//
// opts is parsed on-loop (this function itself runs on-loop, like any goja
// call, before any SDK I/O happens) into a *mcp.CreateMessageParams:
//   - messages (required, non-empty array of {role, content}) ->
//     []*mcp.SamplingMessage, each Content built via toContentItem — the
//     same JS-content-object -> mcp.Content dispatch toContentList/
//     toGetPromptResult already use (mcp_content.go), not duplicated here.
//   - maxTokens/systemPrompt/temperature/stopSequences/includeContext map
//     straight onto the matching CreateMessageParams field.
//   - modelPreferences is a partial pass-through: only the three numeric
//     priorities (costPriority/intelligencePriority/speedPriority) are
//     wired onto *mcp.ModelPreferences. Hints ([]*mcp.ModelHint, matching
//     models by name) is deliberately omitted for now — no test or example
//     needs it yet, and the brief allows dropping an "awkward" field rather
//     than half-implementing it. Revisit if a script needs to steer model
//     choice by name/family.
//
// Capability check: unlike Elicit (which the go-sdk itself gates on
// InitializeParams().Capabilities.Elicitation before ever sending anything,
// see (*ServerSession).Elicit in server.go), CreateMessage has NO such
// guard — a client with no CreateMessageHandler/CreateMessageWithToolsHandler
// registered simply answers the wire round trip with a raw jsonrpc "client
// does not support CreateMessage" error (confirmed against go-sdk@v1.6.1's
// (*Client).createMessage source). Rather than pattern-matching that string,
// this mirrors the SDK's own Elicit precedent and checks the negotiated
// ClientCapabilities.Sampling up front instead: the client package sets it
// automatically (to a non-nil &SamplingCapabilities{}) exactly when
// CreateMessageHandler/CreateMessageWithToolsHandler is non-nil (see
// (*Client) construction in client.go), so absence of that field is a
// reliable, pre-flight signal — and this rejects with a clear
// "mcp: client does not support sampling" message before ever calling the
// SDK, rather than surfacing the SDK's own less-obvious wording. A nil sess
// (shouldn't happen in practice, guarded defensively like
// jsCtxProgress/jsCtxLog) takes the same synchronous-reject fast path.
//
// The actual CreateMessage call is delegated to asyncSettleResult: it's a
// server->client round trip (blocking network/pipe I/O) that must run off
// the loop, exactly like jsCtxProgress/jsCtxLog's SDK calls via asyncSettle —
// the difference is a result value crosses back (the client's sampled
// message), not just an error.
func (ms *mcpServer) jsCtxSample(sess *mcp.ServerSession) func(goja.FunctionCall) goja.Value {
	rejectSync := func(err error) goja.Value {
		p, _, reject := ms.vm.NewPromise()
		_ = reject(ms.vm.NewGoError(err))
		return ms.vm.ToValue(p)
	}

	asFloat := func(who string, v any) float64 {
		switch n := v.(type) {
		case int64:
			return float64(n)
		case float64:
			return n
		default:
			panic(ms.vm.NewTypeError(fmt.Sprintf("mcp: ctx.sample: `%s` must be a number", who)))
		}
	}

	return func(call goja.FunctionCall) goja.Value {
		opts, ok := call.Argument(0).Export().(map[string]any)
		if !ok {
			panic(ms.vm.NewTypeError("mcp: ctx.sample: an options object is required"))
		}

		rawMessages, ok := opts["messages"].([]any)
		if !ok || len(rawMessages) == 0 {
			panic(ms.vm.NewTypeError("mcp: ctx.sample: `messages` must be a non-empty array"))
		}

		messages := make([]*mcp.SamplingMessage, 0, len(rawMessages))
		for i, item := range rawMessages {
			mm, ok := item.(map[string]any)
			if !ok {
				panic(ms.vm.NewTypeError(fmt.Sprintf("mcp: ctx.sample: messages[%d] must be an object", i)))
			}
			role, _ := mm["role"].(string)
			if role == "" {
				panic(ms.vm.NewTypeError(fmt.Sprintf("mcp: ctx.sample: messages[%d].role is required", i)))
			}
			cm, ok := mm["content"].(map[string]any)
			if !ok {
				panic(ms.vm.NewTypeError(fmt.Sprintf("mcp: ctx.sample: messages[%d].content must be an object", i)))
			}
			content, err := toContentItem(cm)
			if err != nil {
				panic(ms.vm.NewTypeError(fmt.Sprintf("mcp: ctx.sample: messages[%d].content: %s", i, err.Error())))
			}
			messages = append(messages, &mcp.SamplingMessage{Role: mcp.Role(role), Content: content})
		}

		params := &mcp.CreateMessageParams{Messages: messages}

		if v, has := opts["maxTokens"]; has {
			params.MaxTokens = int64(asFloat("maxTokens", v))
		}
		if v, has := opts["systemPrompt"]; has {
			s, ok := v.(string)
			if !ok {
				panic(ms.vm.NewTypeError("mcp: ctx.sample: `systemPrompt` must be a string"))
			}
			params.SystemPrompt = s
		}
		if v, has := opts["temperature"]; has {
			params.Temperature = asFloat("temperature", v)
		}
		if v, has := opts["includeContext"]; has {
			s, ok := v.(string)
			if !ok {
				panic(ms.vm.NewTypeError("mcp: ctx.sample: `includeContext` must be a string"))
			}
			params.IncludeContext = s
		}
		if v, has := opts["stopSequences"]; has {
			list, ok := v.([]any)
			if !ok {
				panic(ms.vm.NewTypeError("mcp: ctx.sample: `stopSequences` must be an array of strings"))
			}
			seqs := make([]string, 0, len(list))
			for i, s := range list {
				str, ok := s.(string)
				if !ok {
					panic(ms.vm.NewTypeError(fmt.Sprintf("mcp: ctx.sample: stopSequences[%d] must be a string", i)))
				}
				seqs = append(seqs, str)
			}
			params.StopSequences = seqs
		}
		if v, has := opts["modelPreferences"]; has {
			mp, ok := v.(map[string]any)
			if !ok {
				panic(ms.vm.NewTypeError("mcp: ctx.sample: `modelPreferences` must be an object"))
			}
			prefs := &mcp.ModelPreferences{}
			if raw, has := mp["costPriority"]; has {
				prefs.CostPriority = asFloat("modelPreferences.costPriority", raw)
			}
			if raw, has := mp["intelligencePriority"]; has {
				prefs.IntelligencePriority = asFloat("modelPreferences.intelligencePriority", raw)
			}
			if raw, has := mp["speedPriority"]; has {
				prefs.SpeedPriority = asFloat("modelPreferences.speedPriority", raw)
			}
			params.ModelPreferences = prefs
		}

		if sess == nil {
			return rejectSync(errors.New("mcp: ctx.sample: no client session"))
		}
		if ip := sess.InitializeParams(); ip == nil || ip.Capabilities == nil || ip.Capabilities.Sampling == nil {
			return rejectSync(errors.New("mcp: client does not support sampling"))
		}

		return ms.asyncSettleResult(ms.vm, "mcp:ctx.sample", func() (any, error) {
			return sess.CreateMessage(context.Background(), params)
		})
	}
}

// jsCtxElicit builds ctx.elicit(opts): Promise<{action, content?}> for the
// request context — ctx.sample's sibling exemplar: a mid-handler
// server->client call asking the user (via the client) to confirm an
// action or fill in a small form (the MCP "elicitation" capability), via
// sess.Elicit.
//
// opts is parsed on-loop (this function itself runs on-loop, like any goja
// call, before any SDK I/O happens) into a *mcp.ElicitParams:
//   - message (required, non-empty string) -> Message.
//   - schema (an object) -> RequestedSchema, passed through as-is (the SDK
//     marshals it as-is on the wire, same as jsTool's InputSchema/
//     OutputSchema handling).
//   - mode (optional string) -> Mode ("form"/"url"; left empty lets the SDK
//     infer it from the other fields, per (*ServerSession).Elicit).
//
// Capability check: unlike CreateMessage (which has no SDK-side guard, see
// jsCtxSample's doc comment), (*ServerSession).Elicit in server.go DOES
// gate itself on InitializeParams().Capabilities.Elicitation being non-nil,
// rejecting with the plain "client does not support elicitation" if not.
// This still duplicates that check up front — mirroring jsCtxSample's own
// precedent for a consistent, prefixed "mcp: ..." error surfaced to the
// script before any SDK call is attempted, rather than relying on the SDK's
// (here, adequate but unprefixed) wording. A nil sess (shouldn't happen in
// practice, guarded defensively like jsCtxProgress/jsCtxLog/jsCtxSample)
// takes the same synchronous-reject fast path.
//
// The actual Elicit call is delegated to asyncSettleResult: it's a
// server->client round trip (blocking network/pipe I/O) that must run off
// the loop, exactly like jsCtxSample's CreateMessage call — the result
// (*mcp.ElicitResult, holding Action and Content) crosses back through the
// same toPlain conversion asyncSettleResult already performs.
func (ms *mcpServer) jsCtxElicit(sess *mcp.ServerSession) func(goja.FunctionCall) goja.Value {
	rejectSync := func(err error) goja.Value {
		p, _, reject := ms.vm.NewPromise()
		_ = reject(ms.vm.NewGoError(err))
		return ms.vm.ToValue(p)
	}

	return func(call goja.FunctionCall) goja.Value {
		opts, ok := call.Argument(0).Export().(map[string]any)
		if !ok {
			panic(ms.vm.NewTypeError("mcp: ctx.elicit: an options object is required"))
		}

		message, ok := opts["message"].(string)
		if !ok || message == "" {
			panic(ms.vm.NewTypeError("mcp: ctx.elicit: `message` is required"))
		}

		params := &mcp.ElicitParams{Message: message}

		if v, has := opts["schema"]; has {
			schema, ok := v.(map[string]any)
			if !ok {
				panic(ms.vm.NewTypeError("mcp: ctx.elicit: `schema` must be an object"))
			}
			params.RequestedSchema = schema
		}
		if v, has := opts["mode"]; has {
			s, ok := v.(string)
			if !ok {
				panic(ms.vm.NewTypeError("mcp: ctx.elicit: `mode` must be a string"))
			}
			params.Mode = s
		}

		if sess == nil {
			return rejectSync(errors.New("mcp: ctx.elicit: no client session"))
		}
		if ip := sess.InitializeParams(); ip == nil || ip.Capabilities == nil || ip.Capabilities.Elicitation == nil {
			return rejectSync(errors.New("mcp: client does not support elicitation"))
		}

		return ms.asyncSettleResult(ms.vm, "mcp:ctx.elicit", func() (any, error) {
			return sess.Elicit(context.Background(), params)
		})
	}
}

// jsCtxRoots builds ctx.roots(): Promise<Array<{uri, name?}>> for the
// request context — ctx.sample/ctx.elicit's sibling exemplar, but a
// no-argument server->client round trip: it asks the client for its current
// list of filesystem roots (the MCP "roots" capability) via sess.ListRoots.
//
// Capability check: like Elicit (and unlike CreateMessage, see jsCtxSample's
// doc comment), (*ServerSession).ListRoots has no SDK-side capability guard
// of its own — it just sends the roots/list request and lets the wire round
// trip fail if the client doesn't implement it. Rather than surface that
// less-obvious failure, this checks the negotiated
// ClientCapabilities.RootsV2 up front, mirroring jsCtxSample/jsCtxElicit's
// own precedent. RootsV2 (not the deprecated value-typed Roots field) is the
// right field to nil-check: Roots is a plain struct (not a pointer), so it
// can never distinguish "absent" from "present but false", whereas RootsV2
// is a pointer the SDK only sets when the client actually advertises the
// capability — see ClientCapabilities' doc comment (protocol.go) and
// (*Client).capabilities in the go-sdk, which defaults RootsV2 to
// &RootCapabilities{ListChanged:true} unless the client explicitly opts out
// via ClientOptions.Capabilities. A nil sess (shouldn't happen in practice,
// guarded defensively like jsCtxSample/jsCtxElicit) takes the same
// synchronous-reject fast path.
//
// The actual ListRoots call is delegated to asyncSettleResult: it's a
// server->client round trip (blocking network/pipe I/O) that must run off
// the loop, exactly like jsCtxSample/jsCtxElicit — the result ([]*mcp.Root)
// crosses back through the same toPlain conversion asyncSettleResult
// already performs, yielding a plain [{uri, name}, ...] array in JS (name is
// "" when the client didn't set one, since mcp.Root.Name has no pointer to
// distinguish absent from empty).
func (ms *mcpServer) jsCtxRoots(sess *mcp.ServerSession) func(goja.FunctionCall) goja.Value {
	rejectSync := func(err error) goja.Value {
		p, _, reject := ms.vm.NewPromise()
		_ = reject(ms.vm.NewGoError(err))
		return ms.vm.ToValue(p)
	}

	return func(call goja.FunctionCall) goja.Value {
		if sess == nil {
			return rejectSync(errors.New("mcp: ctx.roots: no client session"))
		}
		if ip := sess.InitializeParams(); ip == nil || ip.Capabilities == nil || ip.Capabilities.RootsV2 == nil {
			return rejectSync(errors.New("mcp: client does not support roots"))
		}

		return ms.asyncSettleResult(ms.vm, "mcp:ctx.roots", func() (any, error) {
			r, err := sess.ListRoots(context.Background(), nil)
			if err != nil {
				return nil, err
			}
			return r.Roots, nil
		})
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

	ms.srv.AddTool(tool, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
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
		out, err := ms.callJSHandler(ctx, lc,
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

	ms.srv.AddResource(&mcp.Resource{URI: uri, Name: name, MIMEType: mimeType}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		sess := req.Session
		tok := req.Params.GetProgressToken()
		requestedURI := req.Params.URI
		out, err := ms.callJSHandler(ctx, lc,
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

	ms.srv.AddResourceTemplate(&mcp.ResourceTemplate{URITemplate: uriTemplate, Name: name, MIMEType: mimeType}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		sess := req.Session
		tok := req.Params.GetProgressToken()
		requestedURI := req.Params.URI
		out, err := ms.callJSHandler(ctx, lc,
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

	ms.srv.AddPrompt(&mcp.Prompt{Name: name, Description: desc, Arguments: args}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		sess := req.Session
		tok := req.Params.GetProgressToken()
		requestedArgs := req.Params.Arguments
		out, err := ms.callJSHandler(ctx, lc,
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

// jsOnRootsChanged implements srv.onRootsChanged(fn): registers a
// best-effort JS hook invoked whenever the client's filesystem-roots list
// changes (via the RootsListChangedHandler dispatcher set in mcp.serve). fn
// receives the fresh roots array (the same [{uri, name?}] shape ctx.roots()
// resolves to) as its sole argument; its return value (and any
// error/rejection) is ignored, and the go-sdk's own handler signature
// (func(context.Context, *RootsListChangedRequest), no return value) has no
// way to fail the notification anyway — see mcpRootsListChangedHandler's doc
// comment.
//
// Only one onRootsChanged callback is held at a time; a later call to
// srv.onRootsChanged replaces the earlier registration (last-writer-wins,
// same as this handle's other single-slot registrations like onSubscribe/
// completion).
func (ms *mcpServer) jsOnRootsChanged(call goja.FunctionCall) goja.Value {
	fn := ms.requireFunctionArg("mcp.onRootsChanged", call)
	ms.setOnRootsChanged(scriptengine.NewLoopCallable(ms.loop, fn))
	return goja.Undefined()
}

// jsCompletion implements srv.completion(fn): registers the JS handler
// invoked for a client's "completion/complete" request (argument
// autocompletion for a prompt argument or a resource-template URI variable).
// fn receives (ref, argName, partial): ref is a normalized
// { type: "prompt"|"resource", name, uri } object (name is set for
// type:"prompt", uri for type:"resource" — see the CompletionHandler
// dispatcher in mcp.go, which builds it from the SDK's
// *mcp.CompleteReference), argName is the argument being completed, and
// partial is the text typed so far. fn may return a string[] or an object
// { values?, total?, hasMore? } (converted by toCompleteResult), a Promise
// of either, or nothing/null/undefined (treated as no matches).
//
// Only one completion callback is held at a time; a later call to
// srv.completion replaces the earlier registration (last-writer-wins, same
// as this handle's other single-slot registrations like onSubscribe).
func (ms *mcpServer) jsCompletion(call goja.FunctionCall) goja.Value {
	fn := ms.requireFunctionArg("mcp.completion", call)
	ms.setCompletionCB(scriptengine.NewLoopCallable(ms.loop, fn))
	return goja.Undefined()
}

// mcpCompletionHandler is the ServerOptions.CompletionHandler wired up in
// mcp.serve (mcp.go): it's what the go-sdk invokes for every
// "completion/complete" request, on one of its own request-handling
// goroutines (never the loop) — the same shape SubscribeHandler/
// UnsubscribeHandler use in mcp.go, just returning a real result instead of
// a fire-and-forget nil.
//
// If no JS handler was ever registered (srv.completion was never called),
// this returns an empty, non-error *mcp.CompleteResult{} — "no
// suggestions", not a protocol failure, since completion is inherently
// optional/best-effort from a client's point of view (many servers won't
// implement it for every ref). Otherwise it dispatches through
// callJSHandler exactly like jsTool/jsResource/jsPrompt: buildArgs runs
// on-loop and constructs (ref, argName, partial) — ref is a normalized
// { type: "prompt"|"resource", name, uri } object built from the SDK's
// *mcp.CompleteReference (Type is either "ref/prompt" or "ref/resource" on
// the wire; both name and uri are set on the JS object regardless of type,
// since setting the unused one is harmless and saves the JS side an
// extra branch) — and convert (toCompleteResult) runs on-loop too, turning
// the handler's return into an *mcp.CompleteResult before it crosses back
// to this goroutine.
//
// Unlike jsTool (whose handler errors become an isError result, since a
// tool call is meant to report failures to the model) a broken/throwing
// completion handler is surfaced as a real protocol-level error here,
// mirroring jsResource/jsPrompt's choice: there's no isError-equivalent
// shape for completion/complete, and a script bug in an autocomplete
// handler is more useful visible to the client (and to whoever's debugging
// the server) than silently downgraded to "no suggestions" — which would
// make the bug indistinguishable from a handler that legitimately has
// nothing to suggest.
func (ms *mcpServer) mcpCompletionHandler(ctx context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
	cb := ms.getCompletionCB()
	if cb == nil {
		return &mcp.CompleteResult{}, nil
	}

	ref := req.Params.Ref
	argName := req.Params.Argument.Name
	partial := req.Params.Argument.Value

	out, err := ms.callJSHandler(ctx, cb,
		func(vm *goja.Runtime) []goja.Value {
			refType := "resource"
			var name, uri string
			if ref != nil {
				if ref.Type == "ref/prompt" {
					refType = "prompt"
				}
				name, uri = ref.Name, ref.URI
			}
			refObj := vm.NewObject()
			_ = refObj.Set("type", refType)
			_ = refObj.Set("name", name)
			_ = refObj.Set("uri", uri)
			return []goja.Value{refObj, vm.ToValue(argName), vm.ToValue(partial)}
		},
		func(vm *goja.Runtime, v goja.Value) (any, error) {
			return toCompleteResult(vm, v)
		},
	)
	if err != nil {
		return nil, err
	}
	// convert always yields *mcp.CompleteResult on the success path; the
	// comma-ok guard keeps a future convert change from panicking here.
	result, ok := out.(*mcp.CompleteResult)
	if !ok {
		return nil, errors.New("mcp: internal completion result conversion failed")
	}
	return result, nil
}

// mcpRootsListChangedHandler is the ServerOptions.RootsListChangedHandler
// wired up in mcp.serve (mcp.go): it's what the go-sdk invokes whenever the
// client sends a notifications/roots/list_changed notification, on one of
// its own request-handling goroutines (never the loop) — the same
// never-on-loop shape as SubscribeHandler/UnsubscribeHandler/
// CompletionHandler. Unlike those, though, the SDK's own signature here
// returns nothing at all (func(context.Context, *RootsListChangedRequest)):
// this notification is truly fire-and-forget from the SDK's point of view,
// there is no result to report back even on the wire.
//
// If no JS handler was ever registered (srv.onRootsChanged was never
// called), this is a silent no-op — same best-effort-hook contract as
// onSubscribe/onUnsubscribe when unset.
//
// Otherwise it fetches the fresh roots list off-loop via
// req.Session.ListRoots: this handler already runs off-loop (on an SDK
// goroutine, per the go-sdk's dispatch), so the blocking round trip is safe
// to make directly here — unlike ctx.roots()/ctx.sample()/ctx.elicit(),
// there is no goja Promise to settle and therefore no need for
// asyncSettleResult's HoldRun+RunOnLoop dance. Once the fresh list is in
// hand, the JS callback is invoked via LoopCallable.Call (which itself
// schedules the call onto the loop), passing the roots — converted to plain
// via toPlain inside buildArgs, on-loop, mirroring
// mcpCompletionHandler's on-loop conversion pattern — as the sole argument.
// A ListRoots failure (e.g. the client errors on this reverse round trip)
// is swallowed: same as the rest of this handler family, a failed
// best-effort notification has nowhere useful to report to.
func (ms *mcpServer) mcpRootsListChangedHandler(ctx context.Context, req *mcp.RootsListChangedRequest) {
	cb := ms.getOnRootsChanged()
	if cb == nil {
		return
	}
	sess := req.Session
	if sess == nil {
		return
	}
	res, err := sess.ListRoots(ctx, nil)
	if err != nil {
		return
	}
	roots := res.Roots
	_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
		plain, err := toPlain(roots)
		if err != nil {
			return nil, err
		}
		return []goja.Value{vm.ToValue(plain)}, nil
	})
}

// jsResourceUpdated implements srv.resourceUpdated(uri): Promise<void>,
// notifying every client currently subscribed to uri that the resource has
// changed. This is the primary way a script signals a resource update — the
// go-sdk's Server.ResourceUpdated does the actual subscriber lookup and
// notification fan-out (the SDK tracks per-session subscriptions itself,
// independent of anything on our side — see mcpServer's subscribeMu doc
// comment).
//
// resourceUpdated is callable immediately after mcp.serve(), before (or
// without) any .stdio()/.listen() transport ever starting — there is no
// requirement that a transport be active. That matters for how this settles:
// see asyncSettle's doc comment for why the call is wrapped in a HoldRun.
func (ms *mcpServer) jsResourceUpdated(call goja.FunctionCall) goja.Value {
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		panic(ms.vm.NewTypeError("mcp.resourceUpdated: a uri is required"))
	}
	uri := arg.String()
	if uri == "" {
		panic(ms.vm.NewTypeError("mcp.resourceUpdated: a non-empty uri is required"))
	}

	return ms.asyncSettle("mcp:resourceUpdated", func() error {
		return ms.srv.ResourceUpdated(context.Background(), &mcp.ResourceUpdatedNotificationParams{URI: uri})
	})
}

// asyncSettle is the shared shape behind jsResourceUpdated/jsCtxProgress/
// jsCtxLog: it builds a Promise, runs work off the event loop in a
// goroutine (work does blocking SDK I/O — network/pipe writes — and must
// never run on-loop), and settles the Promise back on the loop via
// RunOnLoop once work returns.
//
// Crucially, it holds the loop open (eng.HoldRun) for the duration of the
// call. Without the hold, a caller with no other outstanding work keeping
// the loop's jobCount above zero — e.g. srv.resourceUpdated() called at
// top level right after mcp.serve(), before any transport has started —
// can lose the race: loop.Run sees jobCount hit zero and returns before the
// goroutine reaches RunOnLoop, and eventloop.EventLoop silently drops a
// RunOnLoop job queued after the loop has stopped (RunOnLoop, unlike
// setTimeout/setInterval/setImmediate, never counted toward jobCount in the
// first place — see the "Keeping the event loop alive across async work"
// note in CLAUDE.md). The Promise then never settles and the script exits
// 0 without ever resuming past the `await`. Reviewed-and-fixed: this was
// exactly reproducible for jsResourceUpdated before this hold was added.
//
// The hold is released exactly once, inside the on-loop settle callback,
// on both the success and error path (the single deferred func below runs
// release() unconditionally, then checks recover() so a panic during
// resolve/reject still rejects instead of crashing the eventloop's job
// runner, mirroring the recover-then-reject precedent already used
// elsewhere in this file).
func (ms *mcpServer) asyncSettle(reason string, work func() error) goja.Value {
	p, resolve, reject := ms.vm.NewPromise()
	release := ms.eng.HoldRun(reason)

	go func() {
		workErr := work()
		ms.loop.RunOnLoop(func(vm *goja.Runtime) {
			defer func() {
				release()
				if r := recover(); r != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("mcp: %s settle panicked: %v", reason, r)))
				}
			}()
			if workErr != nil {
				_ = reject(vm.NewGoError(workErr))
				return
			}
			_ = resolve(goja.Undefined())
		})
	}()

	return ms.vm.ToValue(p)
}

// asyncSettleResult runs `work` off the event loop, then settles a JS Promise
// on-loop with toPlain(result). It holds loop.Run alive (HoldRun) for the
// whole round-trip and releases exactly once. Shared by the MCP server
// (server->client sends: CreateMessage today, Elicit/ListRoots reuse it in
// Tasks 3-4) and the MCP client (client->server calls). `val` is exported
// through toPlain (cloud_google_storage.go, a JSON marshal/unmarshal round
// trip) before crossing to goja, mirroring how the cloud namespace already
// surfaces SDK response structs as plain objects — no goja.Value is built
// off-loop, and no SDK result struct needs a bespoke converter the way
// toToolResult/toReadResourceResult/toGetPromptResult do for JS-authored
// shapes.
//
// See asyncSettle's doc comment for why the HoldRun is mandatory (without
// it, a caller with no other outstanding work can let loop.Run exit before
// the goroutine's RunOnLoop reaches the loop, per the eventloop's jobCount
// contract) and for the exactly-once release + defer recover() shape, both
// reproduced here unchanged.
func asyncSettleResult(eng *scriptengine.Engine, loop *eventloop.EventLoop, vm *goja.Runtime, reason string, work func() (any, error)) goja.Value {
	p, resolve, reject := vm.NewPromise()
	release := eng.HoldRun(reason)

	go func() {
		val, workErr := work()
		loop.RunOnLoop(func(vm *goja.Runtime) {
			defer func() {
				release()
				if r := recover(); r != nil {
					_ = reject(vm.NewGoError(fmt.Errorf("mcp: %s settle panicked: %v", reason, r)))
				}
			}()
			if workErr != nil {
				_ = reject(vm.NewGoError(workErr))
				return
			}
			plain, err := toPlain(val)
			if err != nil {
				_ = reject(vm.NewGoError(err))
				return
			}
			_ = resolve(vm.ToValue(plain))
		})
	}()

	return vm.ToValue(p)
}

func (ms *mcpServer) asyncSettleResult(vm *goja.Runtime, reason string, work func() (any, error)) goja.Value {
	return asyncSettleResult(ms.eng, ms.loop, vm, reason, work)
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

	// Parse the optional OAuth `auth` block before binding, so a malformed
	// config throws synchronously (no half-bound listener). nil == the
	// unauthenticated Phase-1 path. See mcp_auth.go.
	authCfg := ms.parseMCPAuth(optsObj)

	getServer := func(*http.Request) *mcp.Server { return ms.srv }
	streamableHandler := mcp.NewStreamableHTTPHandler(getServer, nil)
	mux := http.NewServeMux()

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
	baseURL := fmt.Sprintf("http://%s", net.JoinHostPort(urlHost, strconv.Itoa(actualPort)))
	url := baseURL + path

	// Mount the handler(s). With auth, wrap the streamable handler in the
	// bearer-token middleware and expose protected-resource metadata on the
	// same mux (deferred until here so the metadata URL carries the real bound
	// port); without auth, mount the streamable handler bare (Phase-1 path).
	if authCfg != nil {
		authCfg.finalizeMCPAuthMeta(baseURL)
		metadataURL := baseURL + mcpProtectedResourcePath
		applyMCPAuth(mux, streamableHandler, path, authCfg, metadataURL, ms.tokenVerifier(authCfg))
	} else {
		mux.Handle(path, streamableHandler)
	}

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
