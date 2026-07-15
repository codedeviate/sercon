package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"sync/atomic"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mcpClient is a live MCP client session (Phase 1: the consume half). It holds
// the event loop alive for the connection's lifetime and releases exactly once
// on close() or transport death.
type mcpClient struct {
	eng     *scriptengine.Engine
	vm      *goja.Runtime
	loop    *eventloop.EventLoop
	sess    *mcp.ClientSession
	release func()
	closed  atomic.Bool
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
func connectWith(eng *scriptengine.Engine, vm *goja.Runtime, loop *eventloop.EventLoop, reason string, mkTransport func(context.Context) (mcp.Transport, error)) goja.Value {
	p, resolve, reject := vm.NewPromise()
	release := eng.HoldRun(reason)
	var released atomic.Bool
	releaseOnce := func() {
		if released.CompareAndSwap(false, true) {
			release()
		}
	}

	go func() {
		ctx := context.Background()
		transport, terr := mkTransport(ctx)
		var sess *mcp.ClientSession
		var cerr error
		if terr != nil {
			cerr = terr
		} else {
			client := mcp.NewClient(&mcp.Implementation{Name: "sercon", Version: scriptengine.Version}, nil)
			sess, cerr = client.Connect(ctx, transport, nil)
		}
		loop.RunOnLoop(func(vm *goja.Runtime) {
			if cerr != nil {
				releaseOnce()
				_ = reject(vm.NewGoError(fmt.Errorf("mcp.connect: %w", cerr)))
				return
			}
			mc := &mcpClient{eng: eng, vm: vm, loop: loop, sess: sess, release: releaseOnce}
			_ = resolve(mc.handle(vm))
		})
	}()

	return vm.ToValue(p)
}

// handle builds the JS session object. Later tasks add tool/resource/prompt
// methods; Phase-1 Task 1 provides serverInfo, capabilities, and close.
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
			for {
				res, err := mc.sess.ListTools(context.Background(), &mcp.ListToolsParams{Cursor: cursor})
				if err != nil {
					return nil, err
				}
				all = append(all, res.Tools...)
				if res.NextCursor == "" {
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
			res, err := mc.sess.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
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
