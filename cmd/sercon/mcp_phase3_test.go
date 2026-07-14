package main

import (
	"context"
	"strings"
	"testing"

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
