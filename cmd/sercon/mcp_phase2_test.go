package main

import (
	"context"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// notifyWait is the timeout used when blocking on a list-changed notification
// channel in this file's tests. The SDK debounces list-changed notifications
// by 10ms (notificationDelay in mcp/server.go's changeAndNotify) before
// dispatching, so this margin only needs to comfortably clear that plus
// scheduling jitter — it is not expected to be hit in a passing run.
const notifyWait = 3 * time.Second

// connectInMemoryWithOptions is connectInMemory (mcp_server_test.go) plus a
// caller-supplied *mcp.ClientOptions, so this file's tests can register
// ToolListChangedHandler/ResourceListChangedHandler/PromptListChangedHandler.
// A separate function rather than widening connectInMemory's signature: five
// existing call sites in mcp_server_test.go/mcp_http_test.go rely on the
// 2-arg form.
func connectInMemoryWithOptions(ctx context.Context, srv *mcp.Server, opts *mcp.ClientOptions) (*mcp.ClientSession, error) {
	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, opts)
	return client.Connect(ctx, ct, nil)
}

// TestMCPPhase2Tool drives the full runtime add/remove contract for tools:
// serve, connect a client with a tools/list_changed handler wired up, THEN
// (after connect) have the script call srv.tool(...) for a brand-new tool —
// this is the case Phase 1 rejected outright (jsTool's ms.started guard) and
// this task's whole point is to allow. Asserts the client's list_changed
// handler fires and ListTools reflects the addition, then does the mirror
// image for srv.removeTool(name).
//
// The script can't just call srv.tool()/removeTool() back-to-back on its own
// schedule: the Go side needs to connect (which requires `ms.srv`, set
// synchronously by test.serve()) and assert between the add and the remove.
// So the script is split into stages gated by test.waitConnected()/
// test.waitBeforeRemove(), each a Promise the Go side resolves (by closing a
// channel) once it's ready for the script to proceed — same shape as
// mcp_http_test.go's waitClose gate, just two of them here. Because nothing
// else keeps the event loop's jobCount above zero while a gate Promise is
// pending (RunOnLoop doesn't count, per engine.go's eventloop contract), each
// gate wraps its wait in its own eng.HoldRun, mirroring the ready()/done
// pattern TestMCPTool/TestMCPContext already use in mcp_server_test.go.
func TestMCPPhase2Tool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	served := make(chan struct{})
	afterConnect := make(chan struct{})
	afterAddAssert := make(chan struct{})
	done := make(chan struct{})

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		waitOn := func(gate <-chan struct{}) goja.Value {
			p, resolve, _ := vm.NewPromise()
			release := eng.HoldRun("test-mcp-phase2-tool-wait")
			go func() {
				<-gate
				loop.RunOnLoop(func(*goja.Runtime) {
					_ = resolve(goja.Undefined())
					release()
				})
			}()
			return vm.ToValue(p)
		}
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				ms = &mcpServer{
					eng: eng, vm: vm, loop: loop,
					srv: mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, nil),
				}
				close(served)
				return ms.handle(vm)
			},
			"waitConnected":    func() goja.Value { return waitOn(afterConnect) },
			"waitBeforeRemove": func() goja.Value { return waitOn(afterAddAssert) },
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-phase2-tool-ready")
				go func() {
					defer release()
					<-done
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.Run(ctx, "phase2-tool.ts", `
const srv = test.serve();
await test.waitConnected();
srv.tool({
	name: "added",
	inputSchema: { type: "object" },
	handler: () => "ok",
});
await test.waitBeforeRemove();
srv.removeTool("added");
test.ready();
`)
		runErr <- err
	}()

	<-served

	toolChanged := make(chan struct{}, 8)
	sess, err := connectInMemoryWithOptions(ctx, ms.srv, &mcp.ClientOptions{
		ToolListChangedHandler: func(context.Context, *mcp.ToolListChangedRequest) {
			toolChanged <- struct{}{}
		},
	})
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	// Let the script register the new tool now that we're connected.
	close(afterConnect)

	select {
	case <-toolChanged:
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for tools/list_changed after runtime add")
	}

	toolsRes, err := sess.ListTools(ctx, nil)
	if err != nil {
		close(done)
		t.Fatalf("ListTools after add: %v", err)
	}
	foundAdded := false
	for _, tl := range toolsRes.Tools {
		if tl.Name == "added" {
			foundAdded = true
		}
	}
	if !foundAdded {
		close(done)
		t.Fatalf("want tool %q present after runtime add, got %#v", "added", toolsRes.Tools)
	}

	// Let the script remove it.
	close(afterAddAssert)

	select {
	case <-toolChanged:
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for tools/list_changed after runtime remove")
	}

	toolsRes2, err := sess.ListTools(ctx, nil)
	if err != nil {
		close(done)
		t.Fatalf("ListTools after remove: %v", err)
	}
	for _, tl := range toolsRes2.Tools {
		if tl.Name == "added" {
			close(done)
			t.Fatalf("want tool %q gone after removeTool, still present: %#v", "added", toolsRes2.Tools)
		}
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPPhase2ResourceAndPrompt is the lighter spot-check for resources and
// prompts the brief calls for: same runtime add-after-connect /
// remove-after-connect shape as TestMCPPhase2Tool, minus the two-stage
// before/after-assert split — the script adds resource+prompt, then removes
// both, and the Go side asserts a list_changed fires for each kind at least
// once and ListResources/ListPrompts reflect the end state (removed).
// Combined into one test since the resource and prompt bridges are
// structurally identical to the tool one already covered in full above; this
// exists to prove jsResource/jsPrompt/jsRemoveResource/jsRemovePrompt aren't
// copy-paste mistakes, not to re-litigate the notification mechanics.
func TestMCPPhase2ResourceAndPrompt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	served := make(chan struct{})
	afterConnect := make(chan struct{})
	done := make(chan struct{})

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		waitOn := func(gate <-chan struct{}) goja.Value {
			p, resolve, _ := vm.NewPromise()
			release := eng.HoldRun("test-mcp-phase2-rp-wait")
			go func() {
				<-gate
				loop.RunOnLoop(func(*goja.Runtime) {
					_ = resolve(goja.Undefined())
					release()
				})
			}()
			return vm.ToValue(p)
		}
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				ms = &mcpServer{
					eng: eng, vm: vm, loop: loop,
					srv: mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, nil),
				}
				close(served)
				return ms.handle(vm)
			},
			"waitConnected": func() goja.Value { return waitOn(afterConnect) },
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-phase2-rp-ready")
				go func() {
					defer release()
					<-done
				}()
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.Run(ctx, "phase2-resource-prompt.ts", `
const srv = test.serve();
await test.waitConnected();
srv.resource({ uri: "text://added", name: "added", mimeType: "text/plain", read: () => ({ text: "hi" }) });
srv.prompt({ name: "added", get: () => ({ messages: [{ role: "user", content: { type: "text", text: "hi" } }] }) });
srv.removeResource("text://added");
srv.removePrompt("added");
test.ready();
`)
		runErr <- err
	}()

	<-served

	resourceChanged := make(chan struct{}, 8)
	promptChanged := make(chan struct{}, 8)
	sess, err := connectInMemoryWithOptions(ctx, ms.srv, &mcp.ClientOptions{
		ResourceListChangedHandler: func(context.Context, *mcp.ResourceListChangedRequest) {
			resourceChanged <- struct{}{}
		},
		PromptListChangedHandler: func(context.Context, *mcp.PromptListChangedRequest) {
			promptChanged <- struct{}{}
		},
	})
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	close(afterConnect)

	select {
	case <-resourceChanged:
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for resources/list_changed")
	}
	select {
	case <-promptChanged:
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for prompts/list_changed")
	}

	// The add+remove pair happens between the single afterConnect gate and
	// test.ready(), so by the time Run() completes both list_changed events
	// for each kind (add, then remove) have already been debounced/dispatched
	// or are still in flight; draining defensively avoids leaving a buffered
	// send blocked (channels are large enough that it never would) and just
	// confirms the end state below is authoritative regardless of how many
	// notifications coalesced.
	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}

	resRes, err := sess.ListResources(ctx, nil)
	if err != nil {
		t.Fatalf("ListResources: %v", err)
	}
	for _, r := range resRes.Resources {
		if r.URI == "text://added" {
			t.Fatalf("want resource %q gone after removeResource, still present: %#v", "text://added", resRes.Resources)
		}
	}

	promptsRes, err := sess.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatalf("ListPrompts: %v", err)
	}
	for _, p := range promptsRes.Prompts {
		if p.Name == "added" {
			t.Fatalf("want prompt %q gone after removePrompt, still present: %#v", "added", promptsRes.Prompts)
		}
	}
}

// TestMCPPhase2Remove_ValidatesArgs asserts jsRemoveTool/jsRemoveResource/
// jsRemovePrompt all reject a missing/empty name synchronously (a goja
// TypeError, per requireNonEmptyStringArg), rather than silently no-oping or
// panicking some other way. Uses runScript (mcp_server_test.go), same as
// TestMCPServe_BadConfigThrows.
func TestMCPPhase2Remove_ValidatesArgs(t *testing.T) {
	cases := []string{
		`const srv = mcp.serve({ name: "t", version: "1.0.0" }); srv.removeTool("");`,
		`const srv = mcp.serve({ name: "t", version: "1.0.0" }); srv.removeTool();`,
		`const srv = mcp.serve({ name: "t", version: "1.0.0" }); srv.removeResource("");`,
		`const srv = mcp.serve({ name: "t", version: "1.0.0" }); srv.removePrompt("");`,
	}
	for _, script := range cases {
		if _, err := runScript(t, script); err == nil {
			t.Fatalf("expected throw for script %q", script)
		}
	}
}

// TestMCPProgressAndLog drives Task 3's ctx.progress/ctx.log end to end: a
// script registers a tool whose async handler awaits ctx.progress(1, 2) then
// ctx.log("warning", "hi", {a: 1}) before returning. The Go side connects an
// in-memory client with ProgressNotificationHandler + LoggingMessageHandler
// wired up, calls SetLoggingLevel BEFORE invoking the tool (see the GOTCHA
// below), and calls the tool WITH a progress token attached via
// CallToolParams.Meta["progressToken"] (same mechanism mcp_spike2_test.go
// pinned) so the notification can correlate back to this call.
//
// GOTCHA (confirmed by the Task-1 spike, mcp_spike2_test.go): sess.Log
// silently no-ops until the client has called session.SetLoggingLevel — see
// (*ServerSession).Log in the go-sdk's server.go. Skipping SetLoggingLevel
// here would make the log assertion vacuous (ctx.log's Promise still
// resolves — Log returns nil in that case — but nothing would ever reach
// loggingCh, and the test would hang until its timeout instead of failing
// meaningfully). Calling it first is what makes this a real assertion.
func TestMCPProgressAndLog(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	ready := make(chan struct{})
	done := make(chan struct{})

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				ms = &mcpServer{
					eng: eng, vm: vm, loop: loop,
					srv: mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, nil),
				}
				return ms.handle(vm)
			},
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-progress-log")
				go func() {
					defer release()
					<-done
				}()
				close(ready)
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.Run(ctx, "progress-log.ts", `
const srv = test.serve();
srv.tool({
	name: "progressLog",
	inputSchema: { type: "object" },
	handler: async (args, ctx) => {
		await ctx.progress(1, 2);
		await ctx.log("warning", "hi", { a: 1 });
		return "done";
	},
});
test.ready();
`)
		runErr <- err
	}()

	<-ready

	progressCh := make(chan *mcp.ProgressNotificationParams, 4)
	loggingCh := make(chan *mcp.LoggingMessageParams, 4)
	sess, err := connectInMemoryWithOptions(ctx, ms.srv, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			progressCh <- req.Params
		},
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			loggingCh <- req.Params
		},
	})
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	// See the GOTCHA in the doc comment: without this, ctx.log's notification
	// never reaches the client and loggingCh would time out below.
	if err := sess.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		close(done)
		t.Fatalf("SetLoggingLevel: %v", err)
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{
		Name: "progressLog",
		Meta: mcp.Meta{"progressToken": "tok-progress-log"},
	})
	if err != nil {
		close(done)
		t.Fatalf("call progressLog: %v", err)
	}
	if res.IsError {
		close(done)
		t.Fatalf("progressLog reported an error result: %+v", res)
	}

	select {
	case p := <-progressCh:
		if p.ProgressToken != "tok-progress-log" {
			t.Errorf("progress token = %v, want %q", p.ProgressToken, "tok-progress-log")
		}
		if p.Progress != 1 || p.Total != 2 {
			t.Errorf("progress params = %+v, want Progress=1 Total=2", p)
		}
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for progress notification")
	}

	select {
	case l := <-loggingCh:
		if l.Level != "warning" {
			t.Errorf("logging level = %q, want %q", l.Level, "warning")
		}
		data, ok := l.Data.(map[string]any)
		if !ok {
			t.Fatalf("logging data = %#v, want map[string]any", l.Data)
		}
		if data["message"] != "hi" {
			t.Errorf("logging data.message = %v, want %q", data["message"], "hi")
		}
		extra, ok := data["data"].(map[string]any)
		if !ok {
			t.Fatalf("logging data.data = %#v, want map[string]any", data["data"])
		}
		if a, _ := extra["a"].(float64); a != 1 {
			t.Errorf("logging data.data.a = %v, want 1", extra["a"])
		}
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for logging message")
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPProgress_NoTokenResolvesWithoutNotifying asserts ctx.progress
// resolves cleanly (does not hang, does not reject) when the client didn't
// attach a progress token to its call — jsCtxProgress's documented no-op
// path. There is nothing to correlate a progress notification to without a
// token, so this must not call the SDK at all; a connected client with a
// ProgressNotificationHandler confirms nothing arrives.
func TestMCPProgress_NoTokenResolvesWithoutNotifying(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	ready := make(chan struct{})
	done := make(chan struct{})

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				ms = &mcpServer{
					eng: eng, vm: vm, loop: loop,
					srv: mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, nil),
				}
				return ms.handle(vm)
			},
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-progress-no-token")
				go func() {
					defer release()
					<-done
				}()
				close(ready)
				return goja.Undefined()
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.Run(ctx, "progress-no-token.ts", `
const srv = test.serve();
srv.tool({
	name: "progressNoToken",
	inputSchema: { type: "object" },
	handler: async (args, ctx) => {
		await ctx.progress(1, 2);
		return "done";
	},
});
test.ready();
`)
		runErr <- err
	}()

	<-ready

	progressCh := make(chan *mcp.ProgressNotificationParams, 4)
	sess, err := connectInMemoryWithOptions(ctx, ms.srv, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			progressCh <- req.Params
		},
	})
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	// No Meta/progressToken on this call, unlike TestMCPProgressAndLog.
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "progressNoToken"})
	if err != nil {
		close(done)
		t.Fatalf("call progressNoToken: %v", err)
	}
	if res.IsError {
		close(done)
		t.Fatalf("progressNoToken reported an error result (ctx.progress should resolve, not reject, with no token): %+v", res)
	}

	select {
	case p := <-progressCh:
		close(done)
		t.Fatalf("received unexpected progress notification with no token attached: %+v", p)
	case <-time.After(300 * time.Millisecond):
		// expected: nothing arrives
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}
