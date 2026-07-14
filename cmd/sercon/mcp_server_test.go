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

// runScript builds an engine with registerSurface applied, runs script, and
// returns whatever the script wrote via runtime.log (captured from stdout)
// alongside the Run error. Mirrors the engine-construction pattern used in
// server_http_test.go and the stdout-capture pattern in run_output_test.go;
// no helper by this exact name existed yet in cmd/sercon, so it's added here.
func runScript(t *testing.T, script string) (string, error) {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot:     t.TempDir(),
		DisableConsole: true,
		Timeout:        5 * time.Second,
	})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var runErr error
	out := captureStdout(t, func() {
		_, runErr = eng.Run(context.Background(), "test.ts", script)
	})
	return out, runErr
}

func TestMCPServe_RegistersAndValidates(t *testing.T) {
	out, err := runScript(t, `
		const srv = mcp.serve({ name: "t", version: "1.0.0" });
		runtime.log(typeof srv.tool, typeof srv.resource, typeof srv.prompt, typeof srv.stdio, typeof srv.listen, typeof srv.close);
	`)
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	if !strings.Contains(out, "function function function function function function") {
		t.Fatalf("handle missing methods: %q", out)
	}
}

func TestMCPServe_BadConfigThrows(t *testing.T) {
	if _, err := runScript(t, `mcp.serve({ version: "1.0.0" });`); err == nil {
		t.Fatal("expected throw for missing name")
	}
	if _, err := runScript(t, `mcp.serve("nope");`); err == nil {
		t.Fatal("expected throw for non-object config")
	}
}

// connectInMemory wires an in-memory MCP client to srv and returns the
// connected session. Test-only glue (deliberately a free function, not a
// method on the production mcpServer) for driving a registered tool without a
// real stdio/listen transport — reused by later transport tasks.
func connectInMemory(ctx context.Context, srv *mcp.Server) (*mcp.ClientSession, error) {
	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	return client.Connect(ctx, ct, nil)
}

// TestMCPTool drives JS-defined tools end to end: a script builds an mcpServer,
// registers three tools via srv.tool(...), and signals readiness; the Go side
// then connects an in-memory client and calls each tool, asserting the bridge
// (callJSHandler + toToolResult) round-trips async returns, {content:[…]}
// passthrough, and thrown-handler isError results.
//
// The `test` namespace here replicates mcp.serve's ~3-line construction so the
// Go side can capture the *mcpServer (and thus its *mcp.Server); mcp.serve's
// config parsing is covered separately by TestMCPServe_*. Everything under
// srv.tool — jsTool, the SDK ToolHandler, callJSHandler's on-loop conversion,
// and toToolResult — is the real production code path.
func TestMCPTool(t *testing.T) {
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
			// ready() holds the loop alive (so the SDK goroutine's callJSHandler
			// can hop onto it) until the Go side finishes and closes done.
			"ready": func() goja.Value {
				release := eng.HoldRun("test-mcp-tool")
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
		_, err := eng.Run(ctx, "tool.ts", `
const srv = test.serve();
srv.tool({ name: "async_str",   inputSchema: { type: "object" }, handler: async (args) => "hello-" + args.who });
srv.tool({ name: "passthrough", inputSchema: { type: "object" }, handler: (args) => ({ content: [{ type: "text", text: "pt-" + args.who }] }) });
srv.tool({ name: "boom",        inputSchema: { type: "object" }, handler: () => { throw new Error("kaboom"); } });
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

	// (a) async handler returning a string -> single text content.
	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "async_str", Arguments: map[string]any{"who": "world"}})
	if err != nil {
		close(done)
		t.Fatalf("call async_str: %v", err)
	}
	if res.IsError {
		close(done)
		t.Fatalf("async_str: unexpected isError, content=%v", res.Content)
	}
	if len(res.Content) != 1 {
		close(done)
		t.Fatalf("async_str: want 1 content item, got %d", len(res.Content))
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); !ok || tc.Text != "hello-world" {
		close(done)
		t.Fatalf("async_str: want text 'hello-world', got %#v", res.Content[0])
	}

	// (b) handler returning { content: [...] } -> passthrough.
	res, err = sess.CallTool(ctx, &mcp.CallToolParams{Name: "passthrough", Arguments: map[string]any{"who": "there"}})
	if err != nil {
		close(done)
		t.Fatalf("call passthrough: %v", err)
	}
	if res.IsError || len(res.Content) != 1 {
		close(done)
		t.Fatalf("passthrough: want one non-error content item, got isError=%v content=%v", res.IsError, res.Content)
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); !ok || tc.Text != "pt-there" {
		close(done)
		t.Fatalf("passthrough: want text 'pt-there', got %#v", res.Content[0])
	}

	// (c) handler that throws -> isError result carrying the message.
	res, err = sess.CallTool(ctx, &mcp.CallToolParams{Name: "boom", Arguments: map[string]any{}})
	if err != nil {
		close(done)
		t.Fatalf("call boom: %v", err)
	}
	if !res.IsError {
		close(done)
		t.Fatalf("boom: want isError result, got %#v", res)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || !strings.Contains(tc.Text, "kaboom") {
		close(done)
		t.Fatalf("boom: want error text containing 'kaboom', got %#v", res.Content[0])
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPResource drives JS-defined resources end to end: a script builds an
// mcpServer, registers a text resource and a blob resource via
// srv.resource(...), and signals readiness; the Go side then connects an
// in-memory client and reads each resource, asserting the bridge
// (callJSHandler + toReadResourceResult) round-trips a {text} result and a
// {blob} result (base64 string decoded to bytes).
//
// Mirrors TestMCPTool's harness (the `test` namespace replicates mcp.serve's
// construction so the Go side can capture the *mcpServer); everything under
// srv.resource — jsResource, the SDK ResourceHandler, callJSHandler's on-loop
// conversion, and toReadResourceResult — is the real production code path.
func TestMCPResource(t *testing.T) {
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
				release := eng.HoldRun("test-mcp-resource")
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
		_, err := eng.Run(ctx, "resource.ts", `
const srv = test.serve();
srv.resource({ uri: "text://greeting", name: "greeting", mimeType: "text/plain", read: (uri) => ({ text: "hello-" + uri }) });
srv.resource({ uri: "blob://data", name: "data", mimeType: "application/octet-stream", read: (uri) => ({ blob: "aGVsbG8=" }) });
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

	// (a) read handler returning {text} -> one ResourceContents with Text set.
	res, err := sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "text://greeting"})
	if err != nil {
		close(done)
		t.Fatalf("read text://greeting: %v", err)
	}
	if len(res.Contents) != 1 {
		close(done)
		t.Fatalf("text://greeting: want 1 content, got %d", len(res.Contents))
	}
	if c := res.Contents[0]; c.URI != "text://greeting" || c.MIMEType != "text/plain" || c.Text != "hello-text://greeting" {
		close(done)
		t.Fatalf("text://greeting: unexpected contents %#v", c)
	}

	// (b) read handler returning {blob} -> one ResourceContents with the
	// base64 string decoded to raw bytes.
	res, err = sess.ReadResource(ctx, &mcp.ReadResourceParams{URI: "blob://data"})
	if err != nil {
		close(done)
		t.Fatalf("read blob://data: %v", err)
	}
	if len(res.Contents) != 1 {
		close(done)
		t.Fatalf("blob://data: want 1 content, got %d", len(res.Contents))
	}
	if c := res.Contents[0]; c.URI != "blob://data" || c.MIMEType != "application/octet-stream" || string(c.Blob) != "hello" {
		close(done)
		t.Fatalf("blob://data: unexpected contents %#v", c)
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}
