package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// newSampleTestServer wires the same ~3-line `test.serve()`/`test.ready()`
// namespace TestMCPTool (mcp_server_test.go) uses, so the Go side can capture
// the *mcpServer (and thus its *mcp.Server) while a script registers a tool
// via srv.tool(...). Factored out here since both TestMCPSample and
// TestMCPSample_CapabilityAbsent need the identical harness, differing only
// in the connected client's options and the assertions made afterward.
func newSampleTestServer(t *testing.T, script string) (eng *scriptengine.Engine, ms **mcpServer, ready, done chan struct{}, runErr chan error) {
	t.Helper()
	eng = scriptengine.New(scriptengine.Options{DisableConsole: true})
	ms = new(*mcpServer)
	ready = make(chan struct{})
	done = make(chan struct{})
	runErr = make(chan error, 1)

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				*ms = &mcpServer{
					eng: eng, vm: vm, loop: loop,
					srv: mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, nil),
				}
				return (*ms).handle(vm)
			},
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-sample")
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

	go func() {
		ctx := context.Background()
		_, err := eng.Run(ctx, "sample.ts", script)
		runErr <- err
	}()

	return eng, ms, ready, done, runErr
}

// TestMCPSample drives ctx.sample end to end against an in-memory client
// that advertises the sampling capability (via CreateMessageHandler — see
// client.go: "Setting CreateMessageHandler to a non-nil value automatically
// causes the client to advertise the sampling capability"). The script's
// tool handler awaits ctx.sample({messages, maxTokens}) and returns
// r.content.text, proving the full round trip: opts parsing ->
// *mcp.CreateMessageParams -> sess.CreateMessage (held off-loop via
// asyncSettleResult) -> toPlain conversion -> back into JS as a plain
// object.
func TestMCPSample(t *testing.T) {
	_, ms, ready, done, runErr := newSampleTestServer(t, `
const srv = test.serve();
srv.tool({
	name: "ask",
	inputSchema: { type: "object" },
	handler: async (args, ctx) => {
		const r = await ctx.sample({
			messages: [{ role: "user", content: { type: "text", text: "what is 6*7?" } }],
			maxTokens: 100,
		});
		return JSON.stringify({ text: r.content.text, model: r.model, stopReason: r.stopReason, role: r.role });
	},
});
test.ready();
`)

	ctx := context.Background()
	<-ready

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = (*ms).srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content:    &mcp.TextContent{Text: "SUMMARY"},
				Model:      "m",
				StopReason: "endTurn",
				Role:       "assistant",
			}, nil
		},
	})
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "ask"})
	if err != nil {
		close(done)
		t.Fatalf("call ask: %v", err)
	}
	if res.IsError {
		close(done)
		t.Fatalf("ask reported an error result: %+v", res)
	}
	if len(res.Content) != 1 {
		close(done)
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		close(done)
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}
	if !strings.Contains(tc.Text, `"text":"SUMMARY"`) {
		t.Errorf("ask result = %q, want it to contain %q", tc.Text, `"text":"SUMMARY"`)
	}
	if !strings.Contains(tc.Text, `"model":"m"`) {
		t.Errorf("ask result = %q, want it to contain %q", tc.Text, `"model":"m"`)
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPSample_CapabilityAbsent connects an in-memory client with NO
// CreateMessageHandler (and no CreateMessageWithToolsHandler) — the client
// package only sets ClientCapabilities.Sampling when one of those is
// non-nil (client.go's NewClient), so this client negotiates without the
// sampling capability. ctx.sample must reject before ever attempting the SDK
// round trip (see jsCtxSample's capability-check doc comment); the script's
// tool handler catches the rejection and returns the error message as an
// isError tool result, and the assertion below checks it mentions
// "sampling" — not the SDK's own, less obvious wording for an unsupported
// method.
func TestMCPSample_CapabilityAbsent(t *testing.T) {
	_, ms, ready, done, runErr := newSampleTestServer(t, `
const srv = test.serve();
srv.tool({
	name: "ask",
	inputSchema: { type: "object" },
	handler: async (args, ctx) => {
		try {
			await ctx.sample({
				messages: [{ role: "user", content: { type: "text", text: "hi" } }],
			});
			return "unexpectedly resolved";
		} catch (e) {
			throw new Error("sample rejected: " + e.message);
		}
	},
});
test.ready();
`)

	ctx := context.Background()
	<-ready

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = (*ms).srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "ask"})
	if err != nil {
		close(done)
		t.Fatalf("call ask: %v", err)
	}
	if !res.IsError {
		close(done)
		t.Fatalf("ask: want isError result (sampling unsupported), got %+v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		close(done)
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}
	if !strings.Contains(tc.Text, "sampling") {
		t.Errorf("error text = %q, want it to contain %q", tc.Text, "sampling")
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPElicit drives ctx.elicit end to end against an in-memory client
// that advertises the elicitation capability (via ElicitationHandler — see
// client.go: "Setting ElicitationHandler to a non-nil value automatically
// causes the client to advertise" the elicitation capability). The script's
// tool handler awaits ctx.elicit({message, schema}) and returns
// JSON.stringify({action, confirm: content.confirm}), proving the full
// round trip: opts parsing -> *mcp.ElicitParams -> sess.Elicit (held
// off-loop via asyncSettleResult) -> toPlain conversion -> back into JS as
// a plain object, mirroring TestMCPSample.
func TestMCPElicit(t *testing.T) {
	_, ms, ready, done, runErr := newSampleTestServer(t, `
const srv = test.serve();
srv.tool({
	name: "confirm",
	inputSchema: { type: "object" },
	handler: async (args, ctx) => {
		const e = await ctx.elicit({
			message: "Confirm?",
			schema: { type: "object", properties: { confirm: { type: "boolean" } } },
		});
		return JSON.stringify({ action: e.action, confirm: e.content.confirm });
	},
});
test.ready();
`)

	ctx := context.Background()
	<-ready

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = (*ms).srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{
				Action:  "accept",
				Content: map[string]any{"confirm": true},
			}, nil
		},
	})
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "confirm"})
	if err != nil {
		close(done)
		t.Fatalf("call confirm: %v", err)
	}
	if res.IsError {
		close(done)
		t.Fatalf("confirm reported an error result: %+v", res)
	}
	if len(res.Content) != 1 {
		close(done)
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		close(done)
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}
	if !strings.Contains(tc.Text, `"action":"accept"`) {
		t.Errorf("confirm result = %q, want it to contain %q", tc.Text, `"action":"accept"`)
	}
	if !strings.Contains(tc.Text, `"confirm":true`) {
		t.Errorf("confirm result = %q, want it to contain %q", tc.Text, `"confirm":true`)
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPRoots drives ctx.roots end to end against an in-memory client that
// carries two filesystem roots. Unlike TestMCPSample/TestMCPElicit, no
// special client handler is needed to advertise the roots capability — the
// go-sdk client package advertises `roots` (via a non-nil
// ClientCapabilities.RootsV2) by default, for every client, unless the
// caller explicitly opts out via ClientOptions.Capabilities (see
// TestMCPRoots_CapabilityAbsent for that path and jsCtxRoots' doc comment
// for why RootsV2, not the deprecated value-typed Roots field, is what gets
// checked).
//
// client.AddRoots is called BEFORE Connect: it only stores the roots and
// notifies already-connected sessions (changeAndNotify no-ops the notify
// side with zero sessions), so pre-seeding roots this way is the simplest
// way to have them present for the server's very first ListRoots call,
// without racing a separate list-changed round trip.
//
// ListRoots' result order is explicitly documented as non-deterministic
// (client.go's listRoots collects from a set), so the script sorts the
// returned uris before returning them, and the assertion checks the sorted
// JSON array rather than a fixed positional index — proving the full round
// trip: sess.ListRoots (held off-loop via asyncSettleResult) -> toPlain
// conversion -> back into JS as a plain [{uri, name}, ...] array, mirroring
// TestMCPSample/TestMCPElicit.
func TestMCPRoots(t *testing.T) {
	_, ms, ready, done, runErr := newSampleTestServer(t, `
const srv = test.serve();
srv.tool({
	name: "listRoots",
	inputSchema: { type: "object" },
	handler: async (args, ctx) => {
		const roots = await ctx.roots();
		const uris = roots.map((r) => r.uri).sort();
		return JSON.stringify(uris);
	},
});
test.ready();
`)

	ctx := context.Background()
	<-ready

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = (*ms).srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	client.AddRoots(&mcp.Root{URI: "file:///b"}, &mcp.Root{URI: "file:///a"})
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "listRoots"})
	if err != nil {
		close(done)
		t.Fatalf("call listRoots: %v", err)
	}
	if res.IsError {
		close(done)
		t.Fatalf("listRoots reported an error result: %+v", res)
	}
	if len(res.Content) != 1 {
		close(done)
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		close(done)
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}
	want := `["file:///a","file:///b"]`
	if tc.Text != want {
		t.Errorf("listRoots result = %q, want %q", tc.Text, want)
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPRoots_CapabilityAbsent connects an in-memory client that explicitly
// disables the roots capability via ClientOptions.Capabilities — per the
// go-sdk's documented workaround for #607 (see ClientOptions.Capabilities'
// doc comment in client.go), "To disable the roots capability, use
// &ClientCapabilities{}" (leaving RootsV2 nil rather than the SDK's default
// &RootCapabilities{ListChanged:true}). ctx.roots must reject before ever
// attempting the SDK round trip (see jsCtxRoots' capability-check doc
// comment); the script's tool handler catches the rejection and returns the
// error message as an isError tool result, and the assertion below checks it
// mentions "roots" — mirroring TestMCPSample_CapabilityAbsent/
// TestMCPElicit_CapabilityAbsent.
func TestMCPRoots_CapabilityAbsent(t *testing.T) {
	_, ms, ready, done, runErr := newSampleTestServer(t, `
const srv = test.serve();
srv.tool({
	name: "listRoots",
	inputSchema: { type: "object" },
	handler: async (args, ctx) => {
		try {
			await ctx.roots();
			return "unexpectedly resolved";
		} catch (e) {
			throw new Error("roots rejected: " + e.message);
		}
	},
});
test.ready();
`)

	ctx := context.Background()
	<-ready

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = (*ms).srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		Capabilities: &mcp.ClientCapabilities{},
	})
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "listRoots"})
	if err != nil {
		close(done)
		t.Fatalf("call listRoots: %v", err)
	}
	if !res.IsError {
		close(done)
		t.Fatalf("listRoots: want isError result (roots unsupported), got %+v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		close(done)
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}
	if !strings.Contains(tc.Text, "roots") {
		t.Errorf("error text = %q, want it to contain %q", tc.Text, "roots")
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPOnRootsChanged drives srv.onRootsChanged end to end: a script
// registers the hook, then (once the Go side has connected) the in-memory
// client's AddRoots triggers a notifications/roots/list_changed
// notification, which mcp.serve's RootsListChangedHandler wiring
// (mcpRootsListChangedHandler) turns into a fresh req.Session.ListRoots
// round trip followed by an invocation of the registered JS callback with
// the resulting roots. Mirrors TestMCPSubscriptions' shape, but simpler: no
// staged gate is needed since onRootsChanged has no subsequent script action
// to sequence against (unlike resourceUpdated, which needs the client to
// have subscribed first) — the script just registers the hook and holds the
// loop open via test.ready() while the Go side drives the notification.
//
// Unlike newSampleTestServer's harness (used by TestMCPRoots above), which
// constructs its *mcp.Server with nil ServerOptions, this test needs
// RootsListChangedHandler wired — so it builds its own test.serve() (same
// shape as TestMCPSubscriptions' custom harness) rather than reusing
// newSampleTestServer.
//
// The callback's roots argument crosses from Go to JS through
// vm.ToValue(plain) (see mcpRootsListChangedHandler) and back to Go through
// JSON.stringify + a string-typed test binding, sidestepping any question of
// whether goja's reflection-based argument conversion round-trips a
// []any/map[string]any parameter type cleanly for a plain test-only Go
// function value — the same JSON.stringify-and-compare approach
// TestMCPSample/TestMCPElicit already use for their result values.
//
// Run with -race: onRootsChangedCB is set on the main script goroutine
// (jsOnRootsChanged) and read from the go-sdk's own notification-dispatch
// goroutine (mcpRootsListChangedHandler, invoked via ServerOptions.
// RootsListChangedHandler) — exactly the cross-goroutine field access
// mcpServer's rootsMu exists to guard.
func TestMCPOnRootsChanged(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	served := make(chan struct{})
	done := make(chan struct{})
	rootsCh := make(chan string, 4)

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				ms = &mcpServer{eng: eng, vm: vm, loop: loop}
				ms.srv = mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, &mcp.ServerOptions{
					RootsListChangedHandler: ms.mcpRootsListChangedHandler,
				})
				close(served)
				return ms.handle(vm)
			},
			"onRootsChanged": func(rootsJSON string) { rootsCh <- rootsJSON },
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-roots-changed-ready")
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
		_, err := eng.Run(ctx, "roots-changed.ts", `
const srv = test.serve();
srv.onRootsChanged((roots) => { test.onRootsChanged(JSON.stringify(roots)); });
test.ready();
`)
		runErr <- err
	}()

	<-served

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = ms.srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	client.AddRoots(&mcp.Root{URI: "file:///changed"})

	select {
	case got := <-rootsCh:
		if !strings.Contains(got, "file:///changed") {
			t.Errorf("onRootsChanged roots = %q, want it to contain %q", got, "file:///changed")
		}
	case <-time.After(notifyWait):
		close(done)
		t.Fatal("timed out waiting for onRootsChanged callback")
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPElicit_CapabilityAbsent connects an in-memory client with NO
// ElicitationHandler — the client package only sets
// ClientCapabilities.Elicitation when one is non-nil (client.go's
// NewClient), so this client negotiates without the elicitation
// capability. ctx.elicit must reject before ever attempting the SDK round
// trip (see jsCtxElicit's capability-check doc comment); the script's tool
// handler catches the rejection and returns the error message as an
// isError tool result, and the assertion below checks it mentions
// "elicitation" — mirroring TestMCPSample_CapabilityAbsent.
func TestMCPElicit_CapabilityAbsent(t *testing.T) {
	_, ms, ready, done, runErr := newSampleTestServer(t, `
const srv = test.serve();
srv.tool({
	name: "confirm",
	inputSchema: { type: "object" },
	handler: async (args, ctx) => {
		try {
			await ctx.elicit({
				message: "Confirm?",
				schema: { type: "object", properties: { confirm: { type: "boolean" } } },
			});
			return "unexpectedly resolved";
		} catch (e) {
			throw new Error("elicit rejected: " + e.message);
		}
	},
});
test.ready();
`)

	ctx := context.Background()
	<-ready

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = (*ms).srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect: %v", err)
	}
	defer sess.Close()

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "confirm"})
	if err != nil {
		close(done)
		t.Fatalf("call confirm: %v", err)
	}
	if !res.IsError {
		close(done)
		t.Fatalf("confirm: want isError result (elicitation unsupported), got %+v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		close(done)
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}
	if !strings.Contains(tc.Text, "elicitation") {
		t.Errorf("error text = %q, want it to contain %q", tc.Text, "elicitation")
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}
