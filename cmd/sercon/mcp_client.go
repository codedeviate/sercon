package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
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
// adds a connection-scoped ctx + death-watcher). It holds the event loop alive
// for the connection's lifetime and releases exactly once on close() or
// transport death.
type mcpClient struct {
	eng     *scriptengine.Engine
	vm      *goja.Runtime
	loop    *eventloop.EventLoop
	sess    *mcp.ClientSession
	ctx     context.Context
	cancel  context.CancelFunc
	release func()
	closed  atomic.Bool
}

// clientOptions builds the SDK ClientOptions. Task 2 wires the six notification
// handlers to on-loop dispatchers; Phase-1 behaviour is nil (no handlers).
func (mc *mcpClient) clientOptions() *mcp.ClientOptions {
	return &mcp.ClientOptions{}
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

	return connectWith(eng, vm, loop, "mcp:client", func(ctx context.Context) (mcp.Transport, error) {
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
	if len(call.Arguments) > 1 {
		if o, ok := call.Arguments[1].(*goja.Object); ok && o != nil {
			headers = stringMapArg(vm, o.Get("headers"))
		}
	}
	return connectWith(eng, vm, loop, "mcp:client", func(_ context.Context) (mcp.Transport, error) {
		hc := &http.Client{Transport: clientHeaderRoundTripper{base: http.DefaultTransport, headers: headers}}
		return &mcp.StreamableClientTransport{Endpoint: rawURL, HTTPClient: hc}, nil
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

func connectWith(eng *scriptengine.Engine, vm *goja.Runtime, loop *eventloop.EventLoop, reason string, mkTransport func(context.Context) (mcp.Transport, error)) goja.Value {
	p, resolve, reject := vm.NewPromise()
	release := eng.HoldRun(reason)
	var released atomic.Bool
	releaseOnce := func() {
		if released.CompareAndSwap(false, true) {
			release()
		}
	}

	// Connection-scoped context: cancelled by close(), the death-watcher, or
	// a failed connect. Every SDK call uses it, so a script timeout / close
	// unblocks in-flight off-loop calls and (for stdio) ties the subprocess
	// lifetime to the connection.
	connCtx, connCancel := context.WithCancel(context.Background())

	// mc is created up front so later tasks' ClientOptions notification
	// dispatchers can close over it; sess is filled after Connect.
	mc := &mcpClient{eng: eng, vm: vm, loop: loop, ctx: connCtx, cancel: connCancel}
	mc.release = releaseOnce

	go func() {
		transport, terr := mkTransport(connCtx)
		var sess *mcp.ClientSession
		var cerr error
		if terr != nil {
			cerr = terr
		} else {
			client := mcp.NewClient(&mcp.Implementation{Name: "sercon", Version: scriptengine.Version}, mc.clientOptions())
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

func optStringArg(vm *goja.Runtime, v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	return v.String()
}
