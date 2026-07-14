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

// TestMCPHTTP drives srv.listen(...) end to end over a real TCP socket: a
// script builds an mcpServer, registers a tool, calls srv.listen({port: 0})
// (port 0 so the OS picks a free port — avoids the flakiness of a fixed
// port), and hands the resulting handle's `url` back to the Go side via a
// test-only namespace function. The Go side then connects the real SDK
// client over mcp.StreamableClientTransport, does the initialize handshake +
// a tools/call, asserts the result, signals the script to proceed, and
// waits for the script's `await h.close()` + Run to complete.
//
// Unlike TestMCPTool (in-memory transport, needs its own HoldRun to keep the
// loop alive across the callback hop), jsListen's own HoldRun("mcp:http")
// already keeps the loop alive for the whole listen→close window, so no
// extra ready()/done holding namespace call is needed here — just a gate
// (waitClose) so the script doesn't call h.close() before the Go side is
// done asserting.
func TestMCPHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	urlCh := make(chan string, 1)
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
			"notifyURL": func(u string) goja.Value {
				urlCh <- u
				return goja.Undefined()
			},
			// waitClose returns a Promise that only resolves once the Go side
			// has finished its assertions and closed `done`. jsListen's own
			// HoldRun("mcp:http") is what keeps the loop alive while this is
			// pending — this function itself doesn't need to hold anything.
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
		_, err := eng.Run(ctx, "http.ts", `
const srv = test.serve();
srv.tool({
	name: "add",
	inputSchema: { type: "object" },
	handler: (args) => String(args.a + args.b),
});
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
	if !strings.HasPrefix(url, "http://127.0.0.1:") || !strings.HasSuffix(url, "/mcp") {
		close(done)
		t.Fatalf("unexpected handle url: %q", url)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "http-test-client", Version: "0.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{Endpoint: url}
	sess, err := client.Connect(ctx, transport, nil)
	if err != nil {
		close(done)
		t.Fatalf("connect (initialize): %v", err)
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "add", Arguments: map[string]any{"a": 2, "b": 3}})
	if err != nil {
		close(done)
		t.Fatalf("call tool add: %v", err)
	}
	if res.IsError {
		close(done)
		t.Fatalf("add: unexpected isError, content=%v", res.Content)
	}
	if len(res.Content) != 1 {
		close(done)
		t.Fatalf("add: want 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "5" {
		close(done)
		t.Fatalf("add: want TextContent \"5\", got %#v", res.Content[0])
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

// TestMCPHTTP_AlreadyStartedThrows asserts the one-transport-per-handle guard:
// calling listen() a second time throws, per the errAlreadyStarted contract
// jsStdio/jsListen share (tool/resource/prompt registration is exempt from
// this guard as of the runtime-mutation task — see errAlreadyStarted's doc
// comment in mcp.go).
func TestMCPHTTP_AlreadyStartedThrows(t *testing.T) {
	out, err := runScript(t, `
		const srv = mcp.serve({ name: "t", version: "1.0.0" });
		const h = await srv.listen({ port: 0 });
		try {
			await srv.listen({ port: 0 });
			runtime.log("FAIL: second listen() did not throw");
		} catch (e) {
			runtime.log("threw:", e.message);
		}
		await h.close();
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "threw:") || strings.Contains(out, "FAIL") {
		t.Fatalf("expected second listen() to throw, got: %q", out)
	}
}

// TestMCPHTTP_MissingPortThrows asserts jsListen validates its config before
// binding anything.
func TestMCPHTTP_MissingPortThrows(t *testing.T) {
	if _, err := runScript(t, `
		const srv = mcp.serve({ name: "t", version: "1.0.0" });
		srv.listen({});
	`); err == nil {
		t.Fatal("expected throw for missing port")
	}
}

// TestMCPHTTP_ListenThenStdioThrows asserts the started-guard is shared
// across transports, not just within the same one: TestMCPHTTP_AlreadyStartedThrows
// already covers a second listen() after the first; this covers the
// cross-transport ordering (listen() then stdio()) the guard is also meant
// to reject. jsStdio checks `ms.started` before doing anything else — in
// particular before touching the stdout-redirect machinery — so this throws
// synchronously and is safe to exercise in-process, unlike TestMCPStdio
// (which needs a real subprocess because a live stdio() swaps fd 1).
func TestMCPHTTP_ListenThenStdioThrows(t *testing.T) {
	out, err := runScript(t, `
		const srv = mcp.serve({ name: "t", version: "1.0.0" });
		const h = await srv.listen({ port: 0 });
		try {
			srv.stdio();
			runtime.log("FAIL: stdio() after listen() did not throw");
		} catch (e) {
			runtime.log("threw:", e.message);
		}
		await h.close();
	`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out, "threw:") || strings.Contains(out, "FAIL") {
		t.Fatalf("expected stdio() after listen() to throw, got: %q", out)
	}
}
