package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"sync"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mcpClientMaxListPages bounds the four list* auto-pagination loops: a server
// that never returns an empty cursor can't loop forever.
const mcpClientMaxListPages = 1000

// mcpClient is a live MCP client session (Phase 1: the consume half; Phase 2
// adds a connection-scoped ctx + death-watcher, plus the six notification
// callback slots below). It holds the event loop alive for the connection's
// lifetime and releases exactly once on close() or transport death.
//
// The six onXxxCB fields are guarded by cbMu: written on-loop by the script
// goroutine (via c.onToolsChanged/etc. in handle) and read on arbitrary SDK
// goroutines by the clientOptions() dispatchers below. Same pattern as
// mcpServer's onSubscribeCB/onUnsubscribeCB in mcp_server.go.
type mcpClient struct {
	eng      *scriptengine.Engine
	vm       *goja.Runtime
	loop     *eventloop.EventLoop
	sess     *mcp.ClientSession
	ctx      context.Context
	cancel   context.CancelFunc
	release  func()
	closed   atomic.Bool
	host     *mcpHostConfig
	client   *mcp.Client
	rootURIs []string

	cbMu                 sync.Mutex
	onToolsChangedCB     *scriptengine.LoopCallable
	onResourcesChangedCB *scriptengine.LoopCallable
	onPromptsChangedCB   *scriptengine.LoopCallable
	onResourceUpdatedCB  *scriptengine.LoopCallable
	onLoggingMessageCB   *scriptengine.LoopCallable
	onProgressCB         *scriptengine.LoopCallable
}

// mcpHostConfig holds the client-side "host responder" configuration passed
// to mcp.connect.{stdio,http}'s opts object: onSample/onElicit answer the
// server's sampling/createMessage and elicitation/create requests, and roots
// seeds the client's filesystem-roots list (mcp.Root, sent via
// Client.AddRoots before Connect — see connectWith). All three are optional;
// a nil/zero field means the corresponding capability is not advertised (see
// clientOptions' nil-gated wiring) or, for roots, that none are seeded.
type mcpHostConfig struct {
	onSample *scriptengine.LoopCallable
	onElicit *scriptengine.LoopCallable
	roots    []*mcp.Root
}

// parseHostConfig parses onSample/onElicit/roots from a connect-opts object
// (already validated as non-nil by the caller when present) into an
// *mcpHostConfig. Called on-loop, before any SDK I/O — vm/loop are only used
// to validate the two function-typed fields and bind them to LoopCallables.
// A nil optsObj (connectHTTP's opts argument is optional) yields a
// zero-value *mcpHostConfig, i.e. no host responders and no seeded roots.
func parseHostConfig(vm *goja.Runtime, loop *eventloop.EventLoop, optsObj *goja.Object) *mcpHostConfig {
	if optsObj == nil {
		return &mcpHostConfig{}
	}
	hc := &mcpHostConfig{}
	if v := optsObj.Get("onSample"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		if fn, ok := goja.AssertFunction(v); ok {
			hc.onSample = scriptengine.NewLoopCallable(loop, fn)
		} else {
			panic(vm.NewTypeError("mcp.connect: onSample must be a function"))
		}
	}
	if v := optsObj.Get("onElicit"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		if fn, ok := goja.AssertFunction(v); ok {
			hc.onElicit = scriptengine.NewLoopCallable(loop, fn)
		} else {
			panic(vm.NewTypeError("mcp.connect: onElicit must be a function"))
		}
	}
	if v := optsObj.Get("roots"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		hc.roots = parseRoots(vm, v)
	}
	return hc
}

// parseRoots converts a JS array of {uri, name?} into []*mcp.Root. Each
// element must be an object with a non-empty string `uri`; `name` is
// optional. Runs on-loop (called from parseHostConfig).
func parseRoots(vm *goja.Runtime, v goja.Value) []*mcp.Root {
	items, ok := v.Export().([]any)
	if !ok {
		panic(vm.NewTypeError("mcp.connect: roots must be an array of { uri, name? }"))
	}
	roots := make([]*mcp.Root, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			panic(vm.NewTypeError(fmt.Sprintf("mcp.connect: roots[%d] must be an object", i)))
		}
		uri, _ := m["uri"].(string)
		if uri == "" {
			panic(vm.NewTypeError(fmt.Sprintf("mcp.connect: roots[%d].uri is required", i)))
		}
		name, _ := m["name"].(string)
		roots = append(roots, &mcp.Root{URI: uri, Name: name})
	}
	return roots
}

// getCB returns the callback selected by pick under cbMu — a small generic
// getter shared by all six clientOptions() dispatchers below.
func (mc *mcpClient) getCB(pick func() *scriptengine.LoopCallable) *scriptengine.LoopCallable {
	mc.cbMu.Lock()
	defer mc.cbMu.Unlock()
	return pick()
}

// clientOptions builds the SDK ClientOptions. All six notification handlers
// are wired unconditionally at connect time; each dispatcher reads its
// callback slot under cbMu and, if a script has registered one via
// c.onXxx(fn), invokes it on-loop via LoopCallable.Call. Native→goja
// conversion happens inside the buildArgs closure passed to Call, which runs
// on the loop — these handlers themselves run on SDK-internal goroutines and
// must never touch goja directly.
func (mc *mcpClient) clientOptions() *mcp.ClientOptions {
	opts := &mcp.ClientOptions{
		ToolListChangedHandler: func(_ context.Context, _ *mcp.ToolListChangedRequest) {
			cb := mc.getCB(func() *scriptengine.LoopCallable { return mc.onToolsChangedCB })
			if cb == nil {
				return
			}
			_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) { return nil, nil })
		},
		ResourceListChangedHandler: func(_ context.Context, _ *mcp.ResourceListChangedRequest) {
			cb := mc.getCB(func() *scriptengine.LoopCallable { return mc.onResourcesChangedCB })
			if cb == nil {
				return
			}
			_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) { return nil, nil })
		},
		PromptListChangedHandler: func(_ context.Context, _ *mcp.PromptListChangedRequest) {
			cb := mc.getCB(func() *scriptengine.LoopCallable { return mc.onPromptsChangedCB })
			if cb == nil {
				return
			}
			_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) { return nil, nil })
		},
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			cb := mc.getCB(func() *scriptengine.LoopCallable { return mc.onResourceUpdatedCB })
			if cb == nil {
				return
			}
			uri := req.Params.URI
			_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
				return []goja.Value{vm.ToValue(uri)}, nil
			})
		},
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			cb := mc.getCB(func() *scriptengine.LoopCallable { return mc.onLoggingMessageCB })
			if cb == nil {
				return
			}
			params := req.Params
			_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
				plain, err := toPlain(params) // { level, logger, data }
				if err != nil {
					return nil, err
				}
				return []goja.Value{vm.ToValue(plain)}, nil
			})
		},
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			cb := mc.getCB(func() *scriptengine.LoopCallable { return mc.onProgressCB })
			if cb == nil {
				return
			}
			params := req.Params
			_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
				plain, err := toPlain(params) // { progressToken, progress, total, message }
				if err != nil {
					return nil, err
				}
				return []goja.Value{vm.ToValue(plain)}, nil
			})
		},
	}

	// Host responders (onSample/onElicit) are wired ONLY when the script
	// provided one, unlike the six notification handlers above (which are
	// always wired and dispatch as no-ops when no callback is registered).
	// That distinction matters here: setting CreateMessageHandler/
	// ElicitationHandler to non-nil is what causes the SDK client to
	// advertise the corresponding capability (Sampling/Elicitation) during
	// initialize — see ClientOptions' doc comments in go-sdk@v1.6.1's
	// client.go. A client with no onSample must NOT claim it supports
	// sampling, so these two are gated on mc.host being non-nil AND the
	// specific responder being set, whereas the six notification handlers
	// carry no such capability-advertisement side effect and are safe to
	// register unconditionally.
	//
	// Unlike the six notification dispatchers (which read their callback
	// slot under cbMu because a script can register them any time after
	// connect via c.onXxx(fn)), onSample/onElicit are fixed at connect time
	// from the opts object — there is no c.onSample(fn) setter — so no
	// locking is needed here; mc.host is written once in connectWith before
	// this method is ever called.
	if mc.host != nil && mc.host.onSample != nil {
		opts.CreateMessageHandler = func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			out, err := callJSHandler(mc.loop, mc.host.onSample,
				func(vm *goja.Runtime) []goja.Value {
					plain, _ := toPlain(req.Params) // { messages, maxTokens, systemPrompt?, ... }
					return []goja.Value{vm.ToValue(plain)}
				},
				func(vm *goja.Runtime, v goja.Value) (any, error) { return toCreateMessageResult(vm, v) })
			if err != nil {
				return nil, err
			}
			res, ok := out.(*mcp.CreateMessageResult)
			if !ok {
				return nil, fmt.Errorf("mcp: onSample: internal conversion returned %T, want *mcp.CreateMessageResult", out)
			}
			return res, nil
		}
	}
	if mc.host != nil && mc.host.onElicit != nil {
		opts.ElicitationHandler = func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			out, err := callJSHandler(mc.loop, mc.host.onElicit,
				func(vm *goja.Runtime) []goja.Value {
					plain, _ := toPlain(req.Params) // { message, requestedSchema, ... }
					return []goja.Value{vm.ToValue(plain)}
				},
				func(vm *goja.Runtime, v goja.Value) (any, error) { return toElicitResult(vm, v) })
			if err != nil {
				return nil, err
			}
			res, ok := out.(*mcp.ElicitResult)
			if !ok {
				return nil, fmt.Errorf("mcp: onElicit: internal conversion returned %T, want *mcp.ElicitResult", out)
			}
			return res, nil
		}
	}

	return opts
}

// callToolResultView mirrors mcp.CallToolResult for the JS-facing shape,
// except IsError has no `omitempty` tag: the SDK's own type omits IsError
// from JSON entirely on a successful (false) call, which would surface as
// `undefined` rather than `false` after crossing through toPlain's JSON
// round trip. See the isError.omitempty note in callTool below.
type callToolResultView struct {
	Content           []mcp.Content `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError"`
}

// promptArgumentView mirrors mcp.PromptArgument for the JS-facing shape,
// except Required has no `omitempty` tag: the SDK's own type omits Required
// from JSON when false (the common, unset-by-default case for an optional
// argument), which would surface as `undefined` rather than `false` after
// toPlain's JSON round trip — the same isError.omitempty footgun as
// callToolResultView above.
type promptArgumentView struct {
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required"`
}

// promptView mirrors mcp.Prompt, reshaping Arguments through
// promptArgumentView so Required survives the round trip described above.
type promptView struct {
	Name        string               `json:"name"`
	Title       string               `json:"title,omitempty"`
	Description string               `json:"description,omitempty"`
	Icons       []mcp.Icon           `json:"icons,omitempty"`
	Arguments   []promptArgumentView `json:"arguments,omitempty"`
}

// toPromptViews reshapes a page of *mcp.Prompt into promptView, applying the
// Required.omitempty fix described on promptArgumentView.
func toPromptViews(prompts []*mcp.Prompt) []promptView {
	views := make([]promptView, 0, len(prompts))
	for _, p := range prompts {
		v := promptView{Name: p.Name, Title: p.Title, Description: p.Description, Icons: p.Icons}
		for _, a := range p.Arguments {
			v.Arguments = append(v.Arguments, promptArgumentView{
				Name:        a.Name,
				Title:       a.Title,
				Description: a.Description,
				Required:    a.Required,
			})
		}
		views = append(views, v)
	}
	return views
}

// clientHeaderRoundTripper injects fixed headers + a sercon User-Agent.
type clientHeaderRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (rt clientHeaderRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	r2 := r.Clone(r.Context())
	if r2.Header.Get("User-Agent") == "" {
		r2.Header.Set("User-Agent", "sercon-mcp/"+scriptengine.Version)
	}
	for k, v := range rt.headers {
		r2.Header.Set(k, v)
	}
	return rt.base.RoundTrip(r2)
}

// mcpConnectNamespace builds the mcp.connect object registered on the mcp global.
func mcpConnectNamespace(eng *scriptengine.Engine, vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"stdio": func(call goja.FunctionCall) goja.Value {
			return connectStdio(eng, vm, loop, call)
		},
		"http": func(call goja.FunctionCall) goja.Value {
			return connectHTTP(eng, vm, loop, call)
		},
		"sse": func(call goja.FunctionCall) goja.Value {
			return connectSSE(eng, vm, loop, call)
		},
	}
}

func connectStdio(eng *scriptengine.Engine, vm *goja.Runtime, loop *eventloop.EventLoop, call goja.FunctionCall) goja.Value {
	optsObj := requireObjectArg(vm, call, 0, "mcp.connect.stdio(opts)")
	cmdVal := optsObj.Get("command")
	command := stringSliceArg(vm, cmdVal)
	if len(command) == 0 {
		panic(vm.NewTypeError("mcp.connect.stdio: command must be a non-empty string array"))
	}
	env := stringMapArg(vm, optsObj.Get("env")) // map[string]string, nil if absent
	cwd := optStringArg(vm, optsObj.Get("cwd"))
	hostCfg := parseHostConfig(vm, loop, optsObj)

	return connectWith(eng, vm, loop, "mcp:client", hostCfg, func(ctx context.Context) (mcp.Transport, error) {
		cmd := exec.CommandContext(ctx, command[0], command[1:]...)
		if cwd != "" {
			cmd.Dir = cwd
		}
		if len(env) > 0 {
			cmd.Env = cmd.Environ()
			for k, v := range env {
				cmd.Env = append(cmd.Env, k+"="+v)
			}
		}
		// Child stderr is captured by CommandTransport-adjacent plumbing; leave
		// cmd.Stderr nil so the SDK's default (inherit) surfaces server logs to
		// sercon's stderr for debugging.
		return &mcp.CommandTransport{Command: cmd}, nil
	})
}

func connectHTTP(eng *scriptengine.Engine, vm *goja.Runtime, loop *eventloop.EventLoop, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		panic(vm.NewTypeError("mcp.connect.http(url, opts?): url is required"))
	}
	rawURL := call.Arguments[0].String()
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		panic(vm.NewTypeError("mcp.connect.http: url must be an absolute http(s) URL"))
	}
	var headers map[string]string
	var optsObj *goja.Object
	if len(call.Arguments) > 1 {
		if o, ok := call.Arguments[1].(*goja.Object); ok && o != nil {
			optsObj = o
			headers = stringMapArg(vm, o.Get("headers"))
		}
	}
	// maxRetries is StreamableClientTransport-only (mcp.SSEClientTransport has
	// no such field — see connectSSE), so it's parsed here rather than inside
	// the shared parseHostConfig helper. Parsed before mkTransport runs
	// (mkTransport executes off-loop; optsObj.Get must happen on-loop).
	maxRetries, hasMaxRetries := optIntArg(vm, optsObj, "maxRetries", "mcp.connect.http")
	oauthHandler := parseOAuthConfig(vm, loop, optsObj)
	hostCfg := parseHostConfig(vm, loop, optsObj)
	return connectWith(eng, vm, loop, "mcp:client", hostCfg, func(_ context.Context) (mcp.Transport, error) {
		httpClient := &http.Client{Transport: clientHeaderRoundTripper{base: http.DefaultTransport, headers: headers}}
		transport := &mcp.StreamableClientTransport{Endpoint: rawURL, HTTPClient: httpClient}
		if hasMaxRetries {
			// The go-sdk treats MaxRetries == 0 as "use the default (5)" and only
			// a NEGATIVE value as "disable". Map any non-positive script value to
			// -1 so the documented contract — 0 or negative disables reconnection
			// entirely — actually holds.
			if maxRetries <= 0 {
				transport.MaxRetries = -1
			} else {
				transport.MaxRetries = maxRetries
			}
		}
		if oauthHandler != nil {
			transport.OAuthHandler = oauthHandler
		}
		return transport, nil
	})
}

// parseOAuthConfig parses the optional `auth` object off a connect.http opts
// argument into a jsOAuthHandler. Returns nil when `auth` is absent (the
// unauthenticated path, unchanged from Phase 1-3). An `auth` object present
// but missing a `getToken` function throws a TypeError naming the supported
// shape, rather than silently connecting unauthenticated — the only
// supported shape today is `{ getToken: () => token }` (bearer-token OAuth
// client), mirroring parseMCPAuth's synchronous, pre-bind validation style on
// the server side (mcp_auth.go). Runs on-loop, before mkTransport executes
// off-loop — optsObj.Get must happen here, not inside the transport closure.
func parseOAuthConfig(vm *goja.Runtime, loop *eventloop.EventLoop, optsObj *goja.Object) *jsOAuthHandler {
	if optsObj == nil {
		return nil
	}
	av := optsObj.Get("auth")
	if av == nil || goja.IsUndefined(av) || goja.IsNull(av) {
		return nil
	}
	authObj, ok := av.(*goja.Object)
	if !ok {
		panic(vm.NewTypeError("mcp.connect.http: auth must be an object { getToken: () => token }"))
	}
	fn, ok := goja.AssertFunction(authObj.Get("getToken"))
	if !ok {
		panic(vm.NewTypeError("mcp.connect.http: auth.getToken must be a function () => token"))
	}
	return &jsOAuthHandler{loop: loop, getToken: scriptengine.NewLoopCallable(loop, fn)}
}

// connectSSE implements mcp.connect.sse(url, opts?): the legacy (2024-11-05)
// SSE transport. Mirrors connectHTTP's URL validation, headers, and host
// responders (onSample/onElicit/roots — transport-agnostic, so they work
// over SSE too); the only difference is the transport type. Unlike
// connectHTTP, there is no maxRetries here: mcp.SSEClientTransport has only
// {Endpoint, HTTPClient} — no retry knob to plumb (see go-sdk@v1.6.1's
// mcp/streamable.go, which defines MaxRetries solely on
// StreamableClientTransport).
func connectSSE(eng *scriptengine.Engine, vm *goja.Runtime, loop *eventloop.EventLoop, call goja.FunctionCall) goja.Value {
	if len(call.Arguments) == 0 {
		panic(vm.NewTypeError("mcp.connect.sse(url, opts?): url is required"))
	}
	rawURL := call.Arguments[0].String()
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		panic(vm.NewTypeError("mcp.connect.sse: url must be an absolute http(s) URL"))
	}
	var headers map[string]string
	var optsObj *goja.Object
	if len(call.Arguments) > 1 {
		if o, ok := call.Arguments[1].(*goja.Object); ok && o != nil {
			optsObj = o
			headers = stringMapArg(vm, o.Get("headers"))
		}
	}
	hostCfg := parseHostConfig(vm, loop, optsObj)
	return connectWith(eng, vm, loop, "mcp:client", hostCfg, func(_ context.Context) (mcp.Transport, error) {
		httpClient := &http.Client{Transport: clientHeaderRoundTripper{base: http.DefaultTransport, headers: headers}}
		return &mcp.SSEClientTransport{Endpoint: rawURL, HTTPClient: httpClient}, nil
	})
}

// connectWith constructs the transport, connects the SDK client off-loop, and
// resolves a handle on-loop. Holds the loop for the connection's lifetime.
// watchSessionDeath blocks on sess.Wait() — which returns when the session
// ends by ANY means (our own Close, a stdio server subprocess exiting, or an
// abrupt transport/TCP failure) — then cancels the connection context and
// releases the loop hold. This is what lets a connection whose peer has died
// stop pinning loop.Run instead of holding it until the Run-end drain.
//
// LIMITATION (Streamable HTTP + graceful server shutdown): if the peer HTTP
// server shuts down *gracefully* (Go http.Server.Shutdown, e.g. sercon's own
// srv.close()), it does not force-close the client's long-lived SSE stream and
// the SDK client keeps the session alive, so sess.Wait() does NOT return and
// this watcher does not fire — an idle client to a gracefully-stopped HTTP
// server still relies on the Run-end drain to release the hold. Prompt release
// works for stdio (subprocess exit → pipe EOF) and abrupt HTTP failures. See
// MANUAL §5.15.3.
func watchSessionDeath(sess *mcp.ClientSession, cancel context.CancelFunc, releaseOnce func()) {
	_ = sess.Wait()
	cancel()
	releaseOnce()
}

func connectWith(eng *scriptengine.Engine, vm *goja.Runtime, loop *eventloop.EventLoop, reason string, hostCfg *mcpHostConfig, mkTransport func(context.Context) (mcp.Transport, error)) goja.Value {
	p, resolve, reject := vm.NewPromise()
	release := eng.HoldRun(reason)
	var released atomic.Bool
	releaseOnce := func() {
		if released.CompareAndSwap(false, true) {
			release()
		}
	}

	// Connection-scoped context, used by every SDK call. It is cancelled only by
	// close(), the death-watcher (on session end), or a failed connect — so
	// close() unblocks in-flight off-loop calls and (for stdio) ties the
	// subprocess lifetime to the connection. NOTE: it is rooted at
	// context.Background(), NOT the Run context, so a script *timeout* / Run-end
	// does NOT cancel it or unblock an in-flight SDK call — the engine's
	// vm.Interrupt+loop.Terminate stop the script, and the leaked off-loop
	// goroutine + subprocess are reaped on process exit (fine under the
	// single-shot CLI model). Rooting this at the Run context is a tracked
	// follow-up (see OUT-OF-SCOPE).
	connCtx, connCancel := context.WithCancel(context.Background())

	// mc is created up front so later tasks' ClientOptions notification
	// dispatchers can close over it; sess is filled after Connect. mc.host is
	// set before clientOptions() is ever called (both here and inside the
	// goroutine below) since clientOptions gates CreateMessageHandler/
	// ElicitationHandler on mc.host being non-nil.
	mc := &mcpClient{eng: eng, vm: vm, loop: loop, ctx: connCtx, cancel: connCancel, host: hostCfg}
	mc.release = releaseOnce

	go func() {
		transport, terr := mkTransport(connCtx)
		var sess *mcp.ClientSession
		var cerr error
		if terr != nil {
			cerr = terr
		} else {
			client := mcp.NewClient(&mcp.Implementation{Name: "sercon", Version: scriptengine.Version}, mc.clientOptions())
			mc.client = client
			if mc.host != nil && len(mc.host.roots) > 0 {
				client.AddRoots(mc.host.roots...)
				for _, r := range mc.host.roots {
					mc.rootURIs = append(mc.rootURIs, r.URI)
				}
			}
			sess, cerr = client.Connect(connCtx, transport, nil)
		}
		loop.RunOnLoop(func(vm *goja.Runtime) {
			if cerr != nil {
				connCancel()
				releaseOnce()
				_ = reject(vm.NewGoError(fmt.Errorf("mcp.connect: %w", cerr)))
				return
			}
			mc.sess = sess
			go watchSessionDeath(sess, connCancel, releaseOnce)
			_ = resolve(mc.handle(vm))
		})
	}()

	return vm.ToValue(p)
}

// handle builds the JS session object. Phase-1 provides serverInfo,
// capabilities, close, listTools, callTool, listResources,
// listResourceTemplates, readResource, listPrompts, getPrompt, and ping.
func (mc *mcpClient) handle(vm *goja.Runtime) *goja.Object {
	obj := vm.NewObject()

	info := mc.sess.InitializeResult()
	if info != nil {
		if plain, err := toPlain(info.ServerInfo); err == nil {
			_ = obj.Set("serverInfo", vm.ToValue(plain))
		}
		if plain, err := toPlain(info.Capabilities); err == nil {
			_ = obj.Set("capabilities", vm.ToValue(plain))
		}
	}

	_ = obj.Set("close", func(goja.FunctionCall) goja.Value {
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:close", func() (any, error) {
			if mc.closed.CompareAndSwap(false, true) {
				err := mc.sess.Close()
				mc.cancel()
				mc.release()
				return nil, err
			}
			return nil, nil
		})
	})

	_ = obj.Set("listTools", func(goja.FunctionCall) goja.Value {
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:listTools", func() (any, error) {
			var all []*mcp.Tool
			var cursor string
			for pages := 0; ; pages++ {
				res, err := mc.sess.ListTools(mc.ctx, &mcp.ListToolsParams{Cursor: cursor})
				if err != nil {
					return nil, err
				}
				all = append(all, res.Tools...)
				if res.NextCursor == "" {
					break
				}
				if pages+1 >= mcpClientMaxListPages {
					log.Printf("mcp.connect: listTools stopped at %d pages (server keeps returning a cursor); results truncated", mcpClientMaxListPages)
					break
				}
				cursor = res.NextCursor
			}
			return all, nil
		})
	})

	_ = obj.Set("callTool", func(call goja.FunctionCall) goja.Value {
		name := requireStringArg(vm, call, 0, "callTool(name, args?)")
		var args map[string]any
		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) && !goja.IsNull(call.Arguments[1]) {
			if m, ok := call.Arguments[1].Export().(map[string]any); ok {
				args = m
			}
		}
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:callTool", func() (any, error) {
			res, err := mc.sess.CallTool(mc.ctx, &mcp.CallToolParams{Name: name, Arguments: args})
			if err != nil {
				return nil, err
			}
			// CallToolResult.IsError carries `json:"isError,omitempty"`, so a
			// direct marshal of res drops a successful (false) call entirely —
			// toPlain's JSON round trip would then surface `isError` as
			// `undefined` in JS instead of `false`. Re-shape through a view
			// struct without omitempty on IsError so the field is always
			// present.
			return callToolResultView{
				Content:           res.Content,
				StructuredContent: res.StructuredContent,
				IsError:           res.IsError,
			}, nil
		})
	})

	_ = obj.Set("listResources", func(goja.FunctionCall) goja.Value {
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:listResources", func() (any, error) {
			var all []*mcp.Resource
			var cursor string
			for pages := 0; ; pages++ {
				res, err := mc.sess.ListResources(mc.ctx, &mcp.ListResourcesParams{Cursor: cursor})
				if err != nil {
					return nil, err
				}
				all = append(all, res.Resources...)
				if res.NextCursor == "" {
					break
				}
				if pages+1 >= mcpClientMaxListPages {
					log.Printf("mcp.connect: listResources stopped at %d pages (server keeps returning a cursor); results truncated", mcpClientMaxListPages)
					break
				}
				cursor = res.NextCursor
			}
			return all, nil
		})
	})

	_ = obj.Set("listResourceTemplates", func(goja.FunctionCall) goja.Value {
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:listResourceTemplates", func() (any, error) {
			var all []*mcp.ResourceTemplate
			var cursor string
			for pages := 0; ; pages++ {
				res, err := mc.sess.ListResourceTemplates(mc.ctx, &mcp.ListResourceTemplatesParams{Cursor: cursor})
				if err != nil {
					return nil, err
				}
				all = append(all, res.ResourceTemplates...)
				if res.NextCursor == "" {
					break
				}
				if pages+1 >= mcpClientMaxListPages {
					log.Printf("mcp.connect: listResourceTemplates stopped at %d pages (server keeps returning a cursor); results truncated", mcpClientMaxListPages)
					break
				}
				cursor = res.NextCursor
			}
			return all, nil
		})
	})

	_ = obj.Set("readResource", func(call goja.FunctionCall) goja.Value {
		uri := requireStringArg(vm, call, 0, "readResource(uri)")
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:readResource", func() (any, error) {
			return mc.sess.ReadResource(mc.ctx, &mcp.ReadResourceParams{URI: uri})
		})
	})

	_ = obj.Set("listPrompts", func(goja.FunctionCall) goja.Value {
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:listPrompts", func() (any, error) {
			var all []*mcp.Prompt
			var cursor string
			for pages := 0; ; pages++ {
				res, err := mc.sess.ListPrompts(mc.ctx, &mcp.ListPromptsParams{Cursor: cursor})
				if err != nil {
					return nil, err
				}
				all = append(all, res.Prompts...)
				if res.NextCursor == "" {
					break
				}
				if pages+1 >= mcpClientMaxListPages {
					log.Printf("mcp.connect: listPrompts stopped at %d pages (server keeps returning a cursor); results truncated", mcpClientMaxListPages)
					break
				}
				cursor = res.NextCursor
			}
			return toPromptViews(all), nil
		})
	})

	_ = obj.Set("getPrompt", func(call goja.FunctionCall) goja.Value {
		name := requireStringArg(vm, call, 0, "getPrompt(name, args?)")
		var args map[string]string
		if len(call.Arguments) > 1 {
			args = stringMapArg(vm, call.Arguments[1])
		}
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:getPrompt", func() (any, error) {
			return mc.sess.GetPrompt(mc.ctx, &mcp.GetPromptParams{Name: name, Arguments: args})
		})
	})

	_ = obj.Set("ping", func(goja.FunctionCall) goja.Value {
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:ping", func() (any, error) {
			return nil, mc.sess.Ping(mc.ctx, nil)
		})
	})

	// Task 3: subscribe/unsubscribe/setLoggingLevel/complete. All route
	// through asyncSettleResult using mc.ctx like every other call above;
	// none of these are tool calls, so a protocol failure throws rather than
	// surfacing as an isError-shaped result.
	_ = obj.Set("subscribe", func(call goja.FunctionCall) goja.Value {
		uri := requireStringArg(vm, call, 0, "subscribe(uri)")
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:subscribe", func() (any, error) {
			return nil, mc.sess.Subscribe(mc.ctx, &mcp.SubscribeParams{URI: uri})
		})
	})

	_ = obj.Set("unsubscribe", func(call goja.FunctionCall) goja.Value {
		uri := requireStringArg(vm, call, 0, "unsubscribe(uri)")
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:unsubscribe", func() (any, error) {
			return nil, mc.sess.Unsubscribe(mc.ctx, &mcp.UnsubscribeParams{URI: uri})
		})
	})

	_ = obj.Set("setLoggingLevel", func(call goja.FunctionCall) goja.Value {
		level := requireStringArg(vm, call, 0, "setLoggingLevel(level)")
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:setLoggingLevel", func() (any, error) {
			return nil, mc.sess.SetLoggingLevel(mc.ctx, &mcp.SetLoggingLevelParams{Level: mcp.LoggingLevel(level)})
		})
	})

	// complete(ref, argName, partial): ref is a JS object { type: "prompt"|
	// "resource", name?, uri? }. The SDK's *mcp.CompleteReference must be
	// built here, on-loop, before the work closure below runs off-loop —
	// goja values (refObj) can't cross that boundary. Mirrors the server
	// side's mcpCompletionHandler (mcp_server.go), which builds the JS-facing
	// { type, name, uri } object from the same *mcp.CompleteReference shape
	// in the other direction.
	//
	// The result is returned as the raw CompleteResult.Completion
	// (mcp.CompletionResultDetails{Values,Total,HasMore}) rather than nested
	// under a `completion` key, so the script gets { values, total, hasMore }
	// directly — symmetric with what a script passes back from
	// srv.completion(fn) on the server side (see toCompleteResult in
	// mcp_content.go), just without the wrapper.
	_ = obj.Set("complete", func(call goja.FunctionCall) goja.Value {
		refObj := requireObjectArg(vm, call, 0, "complete(ref, argName, partial)")
		argName := requireStringArg(vm, call, 1, "complete(ref, argName, partial)")
		var partial string
		if len(call.Arguments) > 2 {
			partial = call.Arguments[2].String()
		}
		// optStringArg is nil-safe: a missing `type` yields "" → the default
		// case throws cleanly. Calling .String() directly on a missing property
		// would nil-deref (goja returns a Go nil for an absent key) and crash
		// the runtime with an uncatchable SIGSEGV.
		refType := optStringArg(vm, refObj.Get("type"))
		ref := &mcp.CompleteReference{}
		switch refType {
		case "prompt":
			ref.Type = "ref/prompt"
			ref.Name = optStringArg(vm, refObj.Get("name"))
		case "resource":
			ref.Type = "ref/resource"
			ref.URI = optStringArg(vm, refObj.Get("uri"))
		default:
			panic(vm.NewTypeError("complete: ref.type must be \"prompt\" or \"resource\""))
		}
		return asyncSettleResult(mc.eng, mc.loop, vm, "mcp:client:complete", func() (any, error) {
			res, err := mc.sess.Complete(mc.ctx, &mcp.CompleteParams{
				Ref:      ref,
				Argument: mcp.CompleteParamsArgument{Name: argName, Value: partial},
			})
			if err != nil {
				return nil, err
			}
			return res.Completion, nil
		})
	})

	// Task 2: the six server-push notification callbacks. Each setter
	// validates the fn argument, wraps it in a LoopCallable bound to this
	// connection's loop, and stores it under cbMu (last-writer-wins, same as
	// mcpServer's onSubscribe/onUnsubscribe single-slot registrations).
	_ = obj.Set("onToolsChanged", func(call goja.FunctionCall) goja.Value {
		fn := requireFunctionArg(vm, call, 0, "onToolsChanged(fn)")
		lc := scriptengine.NewLoopCallable(mc.loop, fn)
		mc.cbMu.Lock()
		mc.onToolsChangedCB = lc
		mc.cbMu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("onResourcesChanged", func(call goja.FunctionCall) goja.Value {
		fn := requireFunctionArg(vm, call, 0, "onResourcesChanged(fn)")
		lc := scriptengine.NewLoopCallable(mc.loop, fn)
		mc.cbMu.Lock()
		mc.onResourcesChangedCB = lc
		mc.cbMu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("onPromptsChanged", func(call goja.FunctionCall) goja.Value {
		fn := requireFunctionArg(vm, call, 0, "onPromptsChanged(fn)")
		lc := scriptengine.NewLoopCallable(mc.loop, fn)
		mc.cbMu.Lock()
		mc.onPromptsChangedCB = lc
		mc.cbMu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("onResourceUpdated", func(call goja.FunctionCall) goja.Value {
		fn := requireFunctionArg(vm, call, 0, "onResourceUpdated(fn)")
		lc := scriptengine.NewLoopCallable(mc.loop, fn)
		mc.cbMu.Lock()
		mc.onResourceUpdatedCB = lc
		mc.cbMu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("onLoggingMessage", func(call goja.FunctionCall) goja.Value {
		fn := requireFunctionArg(vm, call, 0, "onLoggingMessage(fn)")
		lc := scriptengine.NewLoopCallable(mc.loop, fn)
		mc.cbMu.Lock()
		mc.onLoggingMessageCB = lc
		mc.cbMu.Unlock()
		return goja.Undefined()
	})

	_ = obj.Set("onProgress", func(call goja.FunctionCall) goja.Value {
		fn := requireFunctionArg(vm, call, 0, "onProgress(fn)")
		lc := scriptengine.NewLoopCallable(mc.loop, fn)
		mc.cbMu.Lock()
		mc.onProgressCB = lc
		mc.cbMu.Unlock()
		return goja.Undefined()
	})

	// setRoots(roots): replaces the client's advertised filesystem/URI roots
	// at runtime. Unlike every other handle method, this returns
	// goja.Undefined() directly rather than a Promise: AddRoots/RemoveRoots
	// are pure in-memory bookkeeping on the SDK's *mcp.Client (they queue a
	// roots/list_changed notification but do no I/O themselves), so there is
	// nothing to await. setRoots itself runs on-loop (it's a plain handle
	// method, invoked synchronously from the script goroutine), so mutating
	// mc.rootURIs here is race-free with the off-loop write to it at connect
	// (connectWith's goroutine writes it, then only ever hands off to the
	// loop before this method becomes callable).
	_ = obj.Set("setRoots", func(call goja.FunctionCall) goja.Value {
		roots := parseRoots(vm, call.Argument(0))
		if len(mc.rootURIs) > 0 {
			mc.client.RemoveRoots(mc.rootURIs...)
		}
		mc.client.AddRoots(roots...)
		mc.rootURIs = mc.rootURIs[:0]
		for _, r := range roots {
			mc.rootURIs = append(mc.rootURIs, r.URI)
		}
		return goja.Undefined()
	})

	return obj
}

// requireObjectArg returns call.Arguments[i] as *goja.Object or throws.
func requireObjectArg(vm *goja.Runtime, call goja.FunctionCall, i int, who string) *goja.Object {
	if len(call.Arguments) <= i {
		panic(vm.NewTypeError(who + ": missing options object"))
	}
	o, ok := call.Arguments[i].(*goja.Object)
	if !ok || o == nil {
		panic(vm.NewTypeError(who + ": options must be an object"))
	}
	return o
}

// stringMapArg coerces a goja value into map[string]string (nil if absent).
func stringMapArg(vm *goja.Runtime, v goja.Value) map[string]string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	m, ok := v.Export().(map[string]any)
	if !ok {
		panic(vm.NewTypeError("mcp.connect: expected a string map"))
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		if s, ok := val.(string); ok {
			out[k] = s
		} else {
			out[k] = fmt.Sprintf("%v", val)
		}
	}
	return out
}

// requireStringArg returns call.Arguments[i] as a string or throws.
func requireStringArg(vm *goja.Runtime, call goja.FunctionCall, i int, who string) string {
	if len(call.Arguments) <= i || goja.IsUndefined(call.Arguments[i]) || goja.IsNull(call.Arguments[i]) {
		panic(vm.NewTypeError(who + ": missing argument"))
	}
	return call.Arguments[i].String()
}

// requireFunctionArg returns call.Arguments[i] as a goja.Callable or throws.
// Mirrors mcpServer.requireFunctionArg (mcp_server.go), which is a method on
// *mcpServer and so isn't directly reusable here; kept as a package-level
// function since mcpClient's other arg helpers (requireStringArg etc.) follow
// the same free-function shape.
func requireFunctionArg(vm *goja.Runtime, call goja.FunctionCall, i int, who string) goja.Callable {
	if len(call.Arguments) <= i {
		panic(vm.NewTypeError(who + ": a function is required"))
	}
	fn, isFn := goja.AssertFunction(call.Arguments[i])
	if !isFn {
		panic(vm.NewTypeError(who + ": a function is required"))
	}
	return fn
}

func optStringArg(vm *goja.Runtime, v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}

// optIntArg reads an optional numeric field named key off optsObj (nil-safe:
// a nil optsObj or a missing/undefined/null field returns ok=false, leaving
// the caller to keep its own default). Accepts int64 (goja's export for a JS
// integer literal) or float64 (a non-integer JS literal); any other export
// type throws a TypeError — consistent with this file's other opt/require
// helpers (requireObjectArg, stringMapArg) which reject a wrong-typed field
// rather than silently defaulting.
func optIntArg(vm *goja.Runtime, optsObj *goja.Object, key, who string) (int, bool) {
	if optsObj == nil {
		return 0, false
	}
	v := optsObj.Get(key)
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return 0, false
	}
	switch n := v.Export().(type) {
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		panic(vm.NewTypeError(who + ": " + key + " must be a number"))
	}
}

// toCreateMessageResult converts an onSample handler's JS return value into
// the native *mcp.CreateMessageResult the SDK's CreateMessageHandler must
// return. Runs on-loop (called from callJSHandler's convert callback — see
// clientOptions' CreateMessageHandler wiring above). Two shapes are accepted:
//
//   - a plain string: the common case, wrapped as text content with sercon's
//     default model/role/stopReason;
//   - an object { content: {type, text} | string, model?, stopReason?, role? }
//     for scripts that want to control those fields explicitly.
func toCreateMessageResult(vm *goja.Runtime, v goja.Value) (any, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, fmt.Errorf("onSample must return a string or a result object")
	}
	if s, ok := v.Export().(string); ok {
		return &mcp.CreateMessageResult{Content: &mcp.TextContent{Text: s}, Model: "sercon", Role: "assistant", StopReason: "endTurn"}, nil
	}
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil, fmt.Errorf("onSample must return a string or an object")
	}
	res := &mcp.CreateMessageResult{Model: "sercon", Role: "assistant"}
	if m := optStringArg(vm, obj.Get("model")); m != "" {
		res.Model = m
	}
	if sr := optStringArg(vm, obj.Get("stopReason")); sr != "" {
		res.StopReason = sr
	}
	if r := optStringArg(vm, obj.Get("role")); r != "" {
		res.Role = mcp.Role(r)
	}
	cv := obj.Get("content")
	text := ""
	if co, ok := cv.(*goja.Object); ok {
		text = optStringArg(vm, co.Get("text"))
	} else if s := optStringArg(vm, cv); s != "" {
		text = s
	}
	res.Content = &mcp.TextContent{Text: text}
	return res, nil
}

// toElicitResult converts an onElicit handler's JS return value
// ({ action: "accept"|"decline"|"cancel", content? }) into the native
// *mcp.ElicitResult the SDK's ElicitationHandler must return. Runs on-loop
// (called from callJSHandler's convert callback — see clientOptions'
// ElicitationHandler wiring above).
func toElicitResult(vm *goja.Runtime, v goja.Value) (any, error) {
	obj, ok := v.(*goja.Object)
	if !ok {
		return nil, fmt.Errorf("onElicit must return an object { action, content? }")
	}
	action := optStringArg(vm, obj.Get("action"))
	if action == "" {
		return nil, fmt.Errorf("onElicit result.action is required")
	}
	res := &mcp.ElicitResult{Action: action}
	if cv := obj.Get("content"); cv != nil && !goja.IsUndefined(cv) && !goja.IsNull(cv) {
		if m, ok := cv.Export().(map[string]any); ok {
			res.Content = m
		}
	}
	return res, nil
}
