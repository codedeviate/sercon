package main

import (
	"context"
	"reflect"
	"strings"
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
// scheduling jitter — it is not expected to be hit in a passing run. Kept
// generous (matching the 10s/30s deadlines the Phase-3 notification tests
// use) so a loaded CI runner can't trip it: a 3s value flaked on the slowest
// matrix cell (macos/go-stable) under the heavier post-Phase-3 package load.
// Raising it only lengthens the failure path; a passing run returns at once.
const notifyWait = 15 * time.Second

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

// TestMCPTemplate drives srv.resourceTemplate(...) end to end: a script
// registers a resource template (RFC-6570 URI template) via
// srv.resourceTemplate(...) and signals readiness; the Go side connects an
// in-memory client, asserts ListResourceTemplates surfaces the registered
// template (URITemplate + Name), then reads a CONCRETE URI that matches the
// template ("db:///users/42" against "db:///{table}/{id}") and asserts the
// read handler was invoked with that resolved URI (not the template) and its
// {text} return round-trips through toReadResourceResult, mirroring
// TestMCPResource's harness for the plain (non-template) resource.
func TestMCPTemplate(t *testing.T) {
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
				release := eng.HoldRun("test-mcp-template")
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
		_, err := eng.Run(ctx, "template.ts", `
const srv = test.serve();
srv.resourceTemplate({
	uriTemplate: "db:///{table}/{id}",
	name: "row",
	mimeType: "application/json",
	read: (uri) => ({ text: "row-at-" + uri }),
});
test.ready();
`)
		runErr <- err
	}()

	<-ready

	sess, err := connectInMemory(ctx, ms.srv)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	tmplRes, err := sess.ListResourceTemplates(ctx, nil)
	if err != nil {
		close(done)
		t.Fatalf("ListResourceTemplates: %v", err)
	}
	foundTemplate := false
	for _, tmpl := range tmplRes.ResourceTemplates {
		if tmpl.URITemplate == "db:///{table}/{id}" {
			foundTemplate = true
			if tmpl.Name != "row" {
				close(done)
				t.Fatalf("template name = %q, want %q", tmpl.Name, "row")
			}
		}
	}
	if !foundTemplate {
		close(done)
		t.Fatalf("want template %q present, got %#v", "db:///{table}/{id}", tmplRes.ResourceTemplates)
	}

	res, err := sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "db:///users/42"})
	if err != nil {
		close(done)
		t.Fatalf("read db:///users/42: %v", err)
	}
	if len(res.Contents) != 1 {
		close(done)
		t.Fatalf("db:///users/42: want 1 content, got %d", len(res.Contents))
	}
	if c := res.Contents[0]; c.URI != "db:///users/42" || c.MIMEType != "application/json" || c.Text != "row-at-db:///users/42" {
		close(done)
		t.Fatalf("db:///users/42: unexpected contents %#v", c)
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
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

// TestMCPSubscriptions drives Task 5's resource-subscription plumbing end to
// end: a script registers onSubscribe/onUnsubscribe hooks via
// srv.onSubscribe(fn)/srv.onUnsubscribe(fn), then (once the Go side has
// subscribed) calls srv.resourceUpdated(uri). The Go side drives an
// in-memory client through Subscribe → assert onSubscribe fired with that
// uri → (script calls resourceUpdated) → assert the client's
// ResourceUpdatedHandler fires a resources/updated notification for that uri
// → Unsubscribe → assert onUnsubscribe fired.
//
// Same staged-gate shape as TestMCPPhase2Tool: the script can't just call
// srv.onSubscribe/resourceUpdated back-to-back on its own schedule (the Go
// side needs to connect and subscribe in between), so it awaits
// test.waitSubscribed(), a Promise the Go side resolves (by closing a
// channel) once it has subscribed.
//
// test.ready() is called FIRST — immediately after registering the hooks,
// before awaiting anything — establishing a HoldRun that keeps the loop
// alive for the whole test, same as TestMCPProgressAndLog/TestMCPTemplate.
// This matters here specifically: jsResourceUpdated (like jsCtxProgress/
// jsCtxLog) settles its Promise via a bare goroutine + loop.RunOnLoop with
// no HoldRun of its own (per the documented off-loop-I/O/on-loop-settle
// pattern in mcp_server.go) — RunOnLoop does not itself keep jobCount
// nonzero (see engine.go's eventloop contract), so calling test.ready()
// only at the very end (after resourceUpdated) leaves a window with no
// active hold where the loop could observe jobCount==0 and stop consuming
// scheduled jobs entirely, permanently stalling not just resourceUpdated's
// own settle but also the later onUnsubscribe dispatch — an early failed
// attempt at this test hit exactly that hang.
func TestMCPSubscriptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	served := make(chan struct{})
	afterSubscribed := make(chan struct{})
	done := make(chan struct{})

	onSubCh := make(chan string, 4)
	onUnsubCh := make(chan string, 4)

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		waitOn := func(gate <-chan struct{}) goja.Value {
			p, resolve, _ := vm.NewPromise()
			release := eng.HoldRun("test-mcp-subscriptions-wait")
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
				ms = &mcpServer{eng: eng, vm: vm, loop: loop}
				// Wire Subscribe/UnsubscribeHandler the same way mcp.serve
				// (mcp.go) does — required for the SDK to accept
				// resources/subscribe at all (a nil-options *mcp.Server, as
				// the other tests in this file construct, doesn't support
				// subscriptions).
				ms.srv = mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, &mcp.ServerOptions{
					SubscribeHandler: func(_ context.Context, req *mcp.SubscribeRequest) error {
						if cb := ms.getOnSubscribe(); cb != nil {
							uri := req.Params.URI
							_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
								return []goja.Value{vm.ToValue(uri)}, nil
							})
						}
						return nil
					},
					UnsubscribeHandler: func(_ context.Context, req *mcp.UnsubscribeRequest) error {
						if cb := ms.getOnUnsubscribe(); cb != nil {
							uri := req.Params.URI
							_, _ = cb.Call(func(vm *goja.Runtime) ([]goja.Value, error) {
								return []goja.Value{vm.ToValue(uri)}, nil
							})
						}
						return nil
					},
				})
				close(served)
				return ms.handle(vm)
			},
			"onSub":          func(uri string) { onSubCh <- uri },
			"onUnsub":        func(uri string) { onUnsubCh <- uri },
			"waitSubscribed": func() goja.Value { return waitOn(afterSubscribed) },
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-subscriptions-ready")
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

	const uri = "res://thing"

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.Run(ctx, "subscriptions.ts", `
const srv = test.serve();
srv.onSubscribe((uri) => { test.onSub(uri); });
srv.onUnsubscribe((uri) => { test.onUnsub(uri); });
test.ready();
await test.waitSubscribed();
await srv.resourceUpdated("`+uri+`");
`)
		runErr <- err
	}()

	<-served

	updatedCh := make(chan *mcp.ResourceUpdatedNotificationParams, 4)
	sess, err := connectInMemoryWithOptions(ctx, ms.srv, &mcp.ClientOptions{
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			updatedCh <- req.Params
		},
	})
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	if err := sess.Subscribe(ctx, &mcp.SubscribeParams{URI: uri}); err != nil {
		close(done)
		t.Fatalf("subscribe: %v", err)
	}

	select {
	case got := <-onSubCh:
		if got != uri {
			t.Errorf("onSubscribe uri = %q, want %q", got, uri)
		}
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for onSubscribe callback")
	}

	// Let the script call srv.resourceUpdated(uri) now that the client has
	// subscribed.
	close(afterSubscribed)

	select {
	case params := <-updatedCh:
		if params.URI != uri {
			t.Errorf("resources/updated uri = %q, want %q", params.URI, uri)
		}
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for resources/updated notification")
	}

	if err := sess.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: uri}); err != nil {
		close(done)
		t.Fatalf("unsubscribe: %v", err)
	}

	select {
	case got := <-onUnsubCh:
		if got != uri {
			t.Errorf("onUnsubscribe uri = %q, want %q", got, uri)
		}
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for onUnsubscribe callback")
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPPhase2ResourceUpdated_NoTransport_ResolvesWithoutHang is the regression
// test for the reviewed bug in jsResourceUpdated (and, by the same fix, its
// jsCtxProgress/jsCtxLog siblings — see asyncSettle in mcp_server.go):
// srv.resourceUpdated(uri) is callable immediately after mcp.serve(), BEFORE
// (or without) any .stdio()/.listen() transport ever starting.
//
// Before asyncSettle's HoldRun was added, nothing kept the event loop's
// jobCount above zero while ms.srv.ResourceUpdated ran in its goroutine:
// loop.Run could observe jobCount == 0 and return before the goroutine
// reached loop.RunOnLoop, silently dropping the queued settle job (RunOnLoop
// doesn't itself count toward jobCount — see the "Keeping the event loop
// alive across async work" note in CLAUDE.md). The returned Promise then
// never settled, and the script exited 0 without ever running the line
// after the `await` — reproduced by this exact shape prior to the fix.
//
// This uses runScript (mcp_server_test.go), which registers the real `mcp`
// global via registerSurface — not a test-only stand-in — so the assertion
// exercises the production jsResourceUpdated binding. A post-await
// runtime.log call is the sentinel: it only reaches captured stdout if the
// await actually resumed, proving the settle wasn't dropped.
func TestMCPPhase2ResourceUpdated_NoTransport_ResolvesWithoutHang(t *testing.T) {
	out, err := runScript(t, `
		const srv = mcp.serve({ name: "t", version: "1.0.0" });
		await srv.resourceUpdated("res://x");
		runtime.log("resourceUpdated:resolved");
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "resourceUpdated:resolved") {
		t.Fatalf("post-await marker not observed (Promise settle likely dropped); got %q", out)
	}
}

// TestMCPCompletion drives srv.completion(fn) (Task 6) end to end: a script
// registers a prompt and a resource template, then registers a completion
// handler that filters candidate values differently depending on the
// normalized ref it's given. The Go side connects an in-memory client and
// issues completion/complete requests for both a ref/prompt argument and a
// ref/resource (template) argument, asserting the routed/filtered values
// reach the client — i.e. mcpCompletionHandler's ref normalization
// ({type,name,uri}) and toCompleteResult's array/object conversion both
// round-trip correctly.
//
// Uses the same hand-rolled `test` namespace (mirroring mcp.serve's
// construction) as TestMCPSubscriptions/TestMCPPhase2Tool so the Go side can
// capture *mcpServer and thus *mcp.Server directly, and the same
// ready()/done HoldRun gate so the event loop stays alive while the Go side
// issues requests — required here because, unlike jsResourceUpdated's
// no-transport regression case, dispatching to the registered JS handler
// hops back onto the loop via callJSHandler and would hang forever if the
// loop had already exited.
func TestMCPCompletion(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	served := make(chan struct{})
	done := make(chan struct{})

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				// Two-step assignment (ms first, then ms.srv): mcp.serve
				// takes ms.mcpCompletionHandler as a method value, which
				// needs `ms` to already be a valid, addressable pointer —
				// mirrors the same two-step TestMCPSubscriptions uses for
				// Subscribe/UnsubscribeHandler.
				ms = &mcpServer{eng: eng, vm: vm, loop: loop}
				ms.srv = mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, &mcp.ServerOptions{
					CompletionHandler: ms.mcpCompletionHandler,
				})
				close(served)
				return ms.handle(vm)
			},
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-completion-ready")
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
		_, err := eng.Run(ctx, "completion.ts", `
const srv = test.serve();
srv.prompt({
	name: "greet",
	arguments: [{ name: "name" }],
	get: () => ({ messages: [{ role: "user", content: { type: "text", text: "hi" } }] }),
});
srv.resourceTemplate({
	uriTemplate: "db:///{table}/{id}",
	name: "db-row",
	read: () => ({ text: "{}" }),
});

const names = ["alice", "alicia", "bob"];
const tables = ["users", "orders", "user_sessions"];

srv.completion((ref, argName, partial) => {
	if (ref.type === "prompt" && ref.name === "greet" && argName === "name") {
		return { values: names.filter((n) => n.startsWith(partial)), hasMore: false };
	}
	if (ref.type === "resource" && ref.uri === "db:///{table}/{id}" && argName === "table") {
		return tables.filter((t) => t.startsWith(partial));
	}
	return [];
});

test.ready();
`)
		runErr <- err
	}()

	<-served

	sess, err := connectInMemory(ctx, ms.srv)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	promptRes, err := sess.Complete(ctx, &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "greet"},
		Argument: mcp.CompleteParamsArgument{Name: "name", Value: "ali"},
	})
	if err != nil {
		close(done)
		t.Fatalf("Complete (prompt ref): %v", err)
	}
	if want := []string{"alice", "alicia"}; !reflect.DeepEqual(promptRes.Completion.Values, want) {
		t.Errorf("prompt completion values = %#v, want %#v", promptRes.Completion.Values, want)
	}
	if promptRes.Completion.HasMore {
		t.Errorf("prompt completion HasMore = true, want false")
	}

	resourceRes, err := sess.Complete(ctx, &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/resource", URI: "db:///{table}/{id}"},
		Argument: mcp.CompleteParamsArgument{Name: "table", Value: "user"},
	})
	if err != nil {
		close(done)
		t.Fatalf("Complete (resource ref): %v", err)
	}
	if want := []string{"users", "user_sessions"}; !reflect.DeepEqual(resourceRes.Completion.Values, want) {
		t.Errorf("resource completion values = %#v, want %#v", resourceRes.Completion.Values, want)
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPPageSize drives Task 7's `pageSize` config option end to end over a
// real HTTP transport (mirroring TestMCPHTTP's harness): mcp.serve is called
// with pageSize: 2 (the REAL production binding, via runScript/registerSurface
// — unlike TestMCPHTTP/TestMCPPhase2Tool, this test needs the real
// mcp.serve config-parsing path exercised, not a hand-rolled mcp.NewServer,
// since parsing `pageSize` out of the config object is exactly what Task 7
// adds), 5 tools are registered, then srv.listen({port:0}) hands back a URL
// the script forwards to the Go side via a companion `test` namespace (same
// notifyURL/waitClose shape as TestMCPHTTP). The Go side connects a real SDK
// client and walks ListTools page by page via NextCursor (same
// cursor-walking loop mcp_spike2_test.go's "RemoveTools" subtest already
// pinned against the go-sdk's actual pagination contract), asserting each
// page holds at most 2 tools (PageSize actually chunks, not just an
// unenforced hint), more than one page is produced, and all 5 tools are
// recovered in total (the client aggregates correctly across pages).
func TestMCPPageSize(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}

	urlCh := make(chan string, 1)
	done := make(chan struct{})
	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"notifyURL": func(u string) goja.Value {
				urlCh <- u
				return goja.Undefined()
			},
			"waitClose": func() goja.Value {
				p, resolve, _ := vm.NewPromise()
				go func() {
					<-done
					loop.RunOnLoop(func(*goja.Runtime) { _ = resolve(goja.Undefined()) })
				}()
				return vm.ToValue(p)
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.Run(ctx, "pagesize.ts", `
const srv = mcp.serve({ name: "t", version: "1.0.0", pageSize: 2 });
for (let i = 0; i < 5; i++) {
	srv.tool({
		name: "tool" + i,
		inputSchema: { type: "object" },
		handler: () => "ok",
	});
}
const h = await srv.listen({ port: 0 });
test.notifyURL(h.url);
await test.waitClose();
await h.close();
`)
		runErr <- err
	}()

	var url string
	select {
	case url = <-urlCh:
	case <-ctx.Done():
		close(done)
		t.Fatal("timed out waiting for listen() URL")
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "pagesize-test-client", Version: "0.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: url}
	sess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}

	var names []string
	cursor := ""
	pages := 0
	for {
		page, err := sess.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			close(done)
			t.Fatalf("ListTools (page %d): %v", pages+1, err)
		}
		pages++
		if len(page.Tools) > 2 {
			close(done)
			t.Fatalf("page %d: got %d tools, want <= 2 (PageSize)", pages, len(page.Tools))
		}
		for _, tl := range page.Tools {
			names = append(names, tl.Name)
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(names) != 5 {
		close(done)
		t.Fatalf("want 5 tools total across pages, got %d: %v", len(names), names)
	}
	if pages < 3 {
		close(done)
		t.Fatalf("want pagination to actually chunk at PageSize 2 (>=3 pages for 5 tools), got %d pages", pages)
	}

	if err := sess.Close(); err != nil {
		close(done)
		t.Fatalf("session close: %v", err)
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPPageSize_ValidatesArgs asserts mcp.serve rejects a present-but-invalid
// pageSize synchronously (zero, negative, and non-integer all throw a
// TypeError) rather than silently clamping or falling through to the SDK's
// default. Absence of pageSize is exercised implicitly by every other test in
// this package that calls mcp.serve without it.
func TestMCPPageSize_ValidatesArgs(t *testing.T) {
	cases := []string{
		`mcp.serve({ name: "t", version: "1.0.0", pageSize: 0 });`,
		`mcp.serve({ name: "t", version: "1.0.0", pageSize: -1 });`,
		`mcp.serve({ name: "t", version: "1.0.0", pageSize: 1.5 });`,
		`mcp.serve({ name: "t", version: "1.0.0", pageSize: "2" });`,
	}
	for _, script := range cases {
		if _, err := runScript(t, script); err == nil {
			t.Fatalf("expected throw for script %q", script)
		}
	}
}

// TestMCPCompletionUnregistered asserts the no-handler case the brief calls
// out explicitly: a server that never calls srv.completion still answers
// completion/complete with an empty (not an error) result — see
// mcpCompletionHandler's nil-getCompletionCB branch in mcp_server.go.
//
// Unlike TestMCPCompletion, the nil-callback branch returns immediately
// without ever hopping onto the event loop (there's no JS handler to call),
// so this doesn't need a ready()/done keep-alive gate: the script can finish
// and the loop can exit before the Go side even connects, exactly like
// TestMCPPhase2ResourceUpdated_NoTransport_ResolvesWithoutHang's already-
// exited-loop shape — the in-memory transport goroutine (started by
// connectInMemory against the captured *mcp.Server) is independent of the
// script's own event loop lifetime.
func TestMCPCompletionUnregistered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	served := make(chan struct{})

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				ms = &mcpServer{eng: eng, vm: vm, loop: loop}
				ms.srv = mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, &mcp.ServerOptions{
					CompletionHandler: ms.mcpCompletionHandler,
				})
				close(served)
				return ms.handle(vm)
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		// Deliberately never calls srv.completion(...).
		_, err := eng.Run(ctx, "completion-unregistered.ts", `const srv = test.serve();`)
		runErr <- err
	}()

	<-served
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}

	sess, err := connectInMemory(ctx, ms.srv)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	res, err := sess.Complete(ctx, &mcp.CompleteParams{
		Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "whatever"},
		Argument: mcp.CompleteParamsArgument{Name: "name", Value: "a"},
	})
	if err != nil {
		t.Fatalf("Complete: unexpected error for unregistered handler: %v", err)
	}
	if len(res.Completion.Values) != 0 {
		t.Errorf("Completion.Values = %#v, want empty", res.Completion.Values)
	}
	if res.Completion.HasMore {
		t.Errorf("Completion.HasMore = true, want false")
	}
	if res.Completion.Total != 0 {
		t.Errorf("Completion.Total = %d, want 0", res.Completion.Total)
	}
}
