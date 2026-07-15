package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// newClientTestServer starts a Go-built MCP server on one end of an in-memory
// transport pair and hands the OTHER end to the script via a test namespace
// function `test.transport()` that the script passes to an internal connect.
// Because mcp.connect.{stdio,http} only accept a command/url (not a raw
// transport), Phase-1 functional tests use the HTTP path (TestMCPClientHTTP,
// step 6). This test instead validates the handle wiring at the Go level.
func TestMCPClientConnectHTTP_Handshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}

	// A server + client in ONE script over HTTP: serve().listen({port:0}),
	// then connect.http(h.url), read serverInfo, close both.
	_, err := eng.Run(ctx, "client.ts", `
const srv = mcp.serve({ name: "fixture", version: "9.9.9" });
srv.tool({ name: "ping", inputSchema: { type: "object" }, handler: () => "pong" });
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url);
runtime.assert.equal(c.serverInfo.name, "fixture", "serverInfo.name");
runtime.assert.equal(c.serverInfo.version, "9.9.9", "serverInfo.version");
runtime.assert.ok(c.capabilities, "capabilities present");
await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientConnect_ValidatesArgs asserts bad connect args throw.
func TestMCPClientConnect_ValidatesArgs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		`await mcp.connect.stdio({ command: [] });`, // empty command
		`await mcp.connect.stdio({});`,              // missing command
		`await mcp.connect.http("not-a-url");`,      // bad url
		`await mcp.connect.http("ftp://x/y");`,      // wrong scheme
	}
	for _, c := range cases {
		_, err := eng.Run(ctx, "bad.ts", c)
		if err == nil {
			t.Errorf("expected throw for %q, got nil", c)
		}
	}
}

// TestMCPClientTools exercises listTools (auto-pagination happy path via a
// two-tool server) and callTool, including the isError-not-thrown contract
// for a tool that throws inside its handler.
func TestMCPClientTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "tools.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.tool({ name: "add", inputSchema: { type: "object" }, handler: (a) => String(a.x + a.y) });
srv.tool({ name: "boom", inputSchema: { type: "object" }, handler: () => { throw new Error("nope"); } });
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url);

const tools = await c.listTools();
const names = tools.map(t => t.name).sort();
runtime.assert.equal(JSON.stringify(names), JSON.stringify(["add","boom"]), "tool names");

const ok = await c.callTool("add", { x: 2, y: 3 });
runtime.assert.equal(ok.isError, false, "add not error");
runtime.assert.equal(ok.content[0].text, "5", "add result");

const bad = await c.callTool("boom", {});
runtime.assert.equal(bad.isError, true, "boom isError");

await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientResourcesPromptsPing exercises listResources,
// listResourceTemplates, readResource, listPrompts, getPrompt, and ping
// against a dogfood server exposing one of each.
func TestMCPClientResourcesPromptsPing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "rpp.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.resource({ uri: "cfg://app", name: "cfg", read: () => ({ text: "hello" }) });
srv.resourceTemplate({ uriTemplate: "u:///{id}", name: "u", read: (uri) => ({ text: uri }) });
srv.prompt({ name: "greet", arguments: [{ name: "who" }], get: (a) => ({ messages: [{ role: "user", content: { type: "text", text: "hi " + a.who } }] }) });
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url);

const rs = await c.listResources();
runtime.assert.equal(rs[0].uri, "cfg://app", "resource uri");
const tpls = await c.listResourceTemplates();
runtime.assert.equal(tpls[0].uriTemplate, "u:///{id}", "template");
const doc = await c.readResource("cfg://app");
runtime.assert.equal(doc.contents[0].text, "hello", "read text");

const ps = await c.listPrompts();
runtime.assert.equal(ps[0].name, "greet", "prompt name");
// The argument was declared without a required flag, so it must round-trip as
// the boolean false (not undefined) — the promptView fix for PromptArgument's
// omitempty json tag. Locks in the fix's own justification.
runtime.assert.equal(ps[0].arguments[0].required, false, "arg.required defaults to false, not undefined");
const p = await c.getPrompt("greet", { who: "Ada" });
runtime.assert.ok(JSON.stringify(p.messages).includes("hi Ada"), "prompt rendered");

await c.ping();  // throws if server gone
await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientStdio is the end-to-end gate for mcp.connect.stdio(): it builds
// the sercon binary, then runs the mcp-client-stdio.ts example (itself a
// two-process demo) against that binary, pointing it at the
// mcp-server-stdio.ts fixture via env vars. A clean exit proves the client
// spawned the server subprocess, completed the handshake, and got "5" back
// for add(2, 3) over stdio.
func TestMCPClientStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio MCP server is unix-only")
	}

	bin := filepath.Join(t.TempDir(), "sercon-mcp-client-test")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(build.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}

	clientScript, err := filepath.Abs(filepath.Join("..", "..", "examples", "scripts", "mcp-client-stdio.ts"))
	if err != nil {
		t.Fatal(err)
	}
	serverScript, err := filepath.Abs(filepath.Join("..", "..", "examples", "scripts", "mcp-server-stdio.ts"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, clientScript)
	cmd.Env = append(cmd.Environ(), "SERCON_BIN="+bin, "MCP_SERVER_SCRIPT="+serverScript)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("client-stdio run failed: %v\n%s", err, out)
	}
}

// TestWatchSessionDeath asserts the contract our code owns: when a client
// session ends, watchSessionDeath cancels the connection context and releases
// the loop hold. It uses an in-memory transport pair and closes the SERVER
// session to trigger an abrupt death — deterministic, unlike a graceful HTTP
// shutdown (which the SDK's client keeps alive; see watchSessionDeath's doc
// comment for that limitation). Wait() returns within microseconds of the
// server close here.
func TestWatchSessionDeath(t *testing.T) {
	ctx := context.Background()
	ct, st := mcp.NewInMemoryTransports()
	srv := mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, nil)
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "c", Version: "1.0.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	released := make(chan struct{})
	canceled := make(chan struct{})
	var relOnce, canOnce sync.Once
	go watchSessionDeath(cs,
		func() { canOnce.Do(func() { close(canceled) }) },
		func() { relOnce.Do(func() { close(released) }) })

	// Abruptly kill the server side; the client's Wait() must return, firing
	// both the cancel and the release.
	if err := ss.Close(); err != nil {
		t.Fatalf("server close: %v", err)
	}

	for _, c := range []struct {
		name string
		ch   chan struct{}
	}{{"release", released}, {"cancel", canceled}} {
		select {
		case <-c.ch:
		case <-time.After(5 * time.Second):
			t.Fatalf("watchSessionDeath did not fire %s within 5s of server death", c.name)
		}
	}
}

// TestMCPClientOnToolsChanged exercises c.onToolsChanged(fn): a server-side
// tool addition after connect fires notifications/tools/list_changed, which
// the client dispatches on-loop to the registered JS callback.
func TestMCPClientOnToolsChanged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "notif.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.tool({ name: "a", inputSchema: { type: "object" }, handler: () => "a" });
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url);

let fired = 0;
c.onToolsChanged(() => { fired++; });

// Trigger a list_changed by adding a tool after connect.
srv.tool({ name: "b", inputSchema: { type: "object" }, handler: () => "b" });

// Poll until the notification is delivered (server debounces ~10ms).
const deadline = Date.now() + 5000;
while (fired === 0 && Date.now() < deadline) { await new Promise(r => setTimeout(r, 20)); }
runtime.assert.ok(fired >= 1, "onToolsChanged fired");

await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientSubscribeAndComplete drives Task 3's four reactive methods:
// subscribe (a server-pushed resourceUpdated notification fires
// onResourceUpdated, wired in Task 2), unsubscribe, and a complete()
// round-trip against a server srv.completion(fn) handler. The server's
// CompletionHandler (mcpCompletionHandler, mcp_server.go) dispatches purely
// on ref.Type without requiring a matching srv.prompt/resourceTemplate to
// exist, so the { type: "prompt", name: "greet" } ref below needs no
// registered prompt fixture.
func TestMCPClientSubscribeAndComplete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "sub.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.resource({ uri: "cfg://app", name: "cfg", read: () => ({ text: "v" }) });
srv.completion((ref, argName, partial) => argName === "who" ? ["alice","alan"].filter(v => v.startsWith(partial)) : []);
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url);

let updated = "";
c.onResourceUpdated((uri) => { updated = uri; });
await c.subscribe("cfg://app");
await srv.resourceUpdated("cfg://app");           // server pushes the update
const dl = Date.now() + 5000;
while (updated === "" && Date.now() < dl) { await new Promise(r => setTimeout(r, 20)); }
runtime.assert.equal(updated, "cfg://app", "onResourceUpdated fired with uri");
await c.unsubscribe("cfg://app");

const comp = await c.complete({ type: "prompt", name: "greet" }, "who", "ala");
runtime.assert.equal(JSON.stringify(comp.values), JSON.stringify(["alan"]), "completion filtered");

await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientCompleteValidatesRef asserts complete() throws a clean, catchable
// error for a bad ref — a missing `type` (regression: reading it via
// .Get("type").String() nil-dereffed and crashed the runtime with an uncatchable
// SIGSEGV) and an unknown `type`. Must be a caught JS error, not a process crash.
func TestMCPClientCompleteValidatesRef(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "badref.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url);
let threwMissing = false, threwUnknown = false;
try { await c.complete({}, "who", "a"); } catch (e) { threwMissing = true; }
try { await c.complete({ type: "nope" }, "who", "a"); } catch (e) { threwUnknown = true; }
runtime.assert.ok(threwMissing, "complete with missing ref.type throws (not a crash)");
runtime.assert.ok(threwUnknown, "complete with unknown ref.type throws");
await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientSetLoggingLevel exercises c.setLoggingLevel end to end: the
// server's ctx.log is a no-op until the client opts in with a level, so this
// asserts that after setLoggingLevel("info") a server tool's ctx.log is
// delivered to the client's onLoggingMessage callback with the right payload.
// (Locks in the setLoggingLevel binding, which otherwise had no coverage.)
func TestMCPClientSetLoggingLevel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "log.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.tool({ name: "chatty", inputSchema: { type: "object" }, handler: async (a, ctx) => {
  await ctx.log("info", "hello from tool", { n: 7 });
  return "done";
}});
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url);

let got = null;
c.onLoggingMessage((m) => { got = m; });
await c.setLoggingLevel("info");   // without this, the server's ctx.log is a no-op
await c.callTool("chatty", {});

const dl = Date.now() + 5000;
while (got === null && Date.now() < dl) { await new Promise(r => setTimeout(r, 20)); }
runtime.assert.ok(got !== null, "onLoggingMessage fired after setLoggingLevel");
runtime.assert.equal(got.level, "info", "log level");

await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientHostSampleElicit dogfoods MCP Phase 3's host responders: the
// client answers the server's sampling/createMessage (ctx.sample) and
// elicitation/create (ctx.elicit) requests via onSample/onElicit passed to
// mcp.connect.http. Asserts the tool results reflect the client's answers,
// exercising both the CreateMessageResult and ElicitResult conversion paths.
func TestMCPClientHostSampleElicit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "host.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.tool({ name: "summarize", inputSchema: { type: "object" }, handler: async (a, ctx) => {
  const r = await ctx.sample({ messages: [{ role: "user", content: { type: "text", text: "hi" } }], maxTokens: 50 });
  return r.content.text;
}});
srv.tool({ name: "confirm", inputSchema: { type: "object" }, handler: async (a, ctx) => {
  const e = await ctx.elicit({ message: "ok?", schema: { type: "object", properties: { yes: { type: "boolean" } } } });
  return JSON.stringify({ action: e.action, yes: e.content && e.content.yes });
}});
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url, {
  onSample: (req) => "SUMMARY:" + req.messages[0].content.text,
  onElicit: (req) => ({ action: "accept", content: { yes: true } }),
});
const s = await c.callTool("summarize", {});
runtime.assert.equal(s.content[0].text, "SUMMARY:hi", "onSample answered");
const e = await c.callTool("confirm", {});
runtime.assert.ok(e.content[0].text.includes('"action":"accept"'), "onElicit accept");
runtime.assert.ok(e.content[0].text.includes('"yes":true'), "onElicit content");
await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientHostRoots dogfoods MCP Phase 3's roots wiring: the client
// seeds its filesystem/URI roots via the `roots` connect option (AddRoots
// before Connect), and the server sees them via ctx.roots() (roots/list).
// Then c.setRoots(...) swaps the set at runtime (RemoveRoots + AddRoots),
// and a second ctx.roots() call on the server observes the update.
func TestMCPClientHostRoots(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "roots.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.tool({ name: "listRoots", inputSchema: { type: "object" }, handler: async (a, ctx) => {
  const rs = await ctx.roots();
  return rs.map(r => r.uri).sort().join(",");
}});
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url, { roots: [{ uri: "file:///a" }, { uri: "file:///b" }] });
const r1 = await c.callTool("listRoots", {});
runtime.assert.equal(r1.content[0].text, "file:///a,file:///b", "initial roots");
c.setRoots([{ uri: "file:///c" }]);
const r2 = await c.callTool("listRoots", {});
runtime.assert.equal(r2.content[0].text, "file:///c", "updated roots");
await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientSSE is the Phase-4 gate for mcp.connect.sse(): a client
// connects over the legacy (2024-11-05) SSE transport and round-trips
// listTools/callTool.
//
// Dogfood path (Task 1 brief, Step 1): sercon's own server (srv.listen,
// server_http.go) mounts ONLY mcp.NewStreamableHTTPHandler — grepping
// cmd/sercon/ turns up no use of the SDK's mcp.NewSSEHandler, so there is no
// SSE-compatible endpoint on the sercon side to dogfood against. Instead this
// test builds a Go mcp.Server directly (mirroring TestMCPSDKSpike's
// AddTool/CallToolRequest pattern) and serves it over the SDK's
// mcp.NewSSEHandler + httptest.NewServer, then drives mcp.connect.sse from a
// sercon script against that URL.
func TestMCPClientSSE(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	srv := mcp.NewServer(&mcp.Implementation{Name: "sse-fixture", Version: "1.2.3"}, nil)
	srv.AddTool(&mcp.Tool{
		Name: "add",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"x": map[string]any{"type": "number"},
				"y": map[string]any{"type": "number"},
			},
			"required": []any{"x", "y"},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
		}
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		sum := fmt.Sprintf("%v", args.X+args.Y)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: sum}}}, nil
	})

	sseHandler := mcp.NewSSEHandler(func(*http.Request) *mcp.Server { return srv }, nil)
	ts := httptest.NewServer(sseHandler)
	defer ts.Close()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`
const c = await mcp.connect.sse(%q);
runtime.assert.equal(c.serverInfo.name, "sse-fixture", "serverInfo.name");
runtime.assert.equal(c.serverInfo.version, "1.2.3", "serverInfo.version");
const tools = await c.listTools();
runtime.assert.equal(tools.map(t => t.name).join(","), "add", "tool names");
const res = await c.callTool("add", { x: 2, y: 3 });
runtime.assert.equal(res.isError, false, "add not error");
runtime.assert.equal(res.content[0].text, "5", "add result");
await c.close();
`, ts.URL)
	if _, err := eng.Run(ctx, "sse.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientSSE_ValidatesArgs asserts mcp.connect.sse rejects a
// non-absolute / non-http(s) URL the same way mcp.connect.http does.
func TestMCPClientSSE_ValidatesArgs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		`await mcp.connect.sse("not-a-url");`,
		`await mcp.connect.sse("ftp://x/y");`,
	}
	for _, c := range cases {
		if _, err := eng.Run(ctx, "bad-sse.ts", c); err == nil {
			t.Errorf("expected throw for %q, got nil", c)
		}
	}
}

// TestMCPClientMaxRetries is the Phase-4 gate for connect.http's maxRetries
// option: it proves { maxRetries: 0 } is accepted and plumbed through to
// StreamableClientTransport without breaking a normal, successful session —
// deeper reconnect-retry behaviour on transport failure is SDK-owned and not
// re-tested here.
func TestMCPClientMaxRetries(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "maxretries.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.tool({ name: "add", inputSchema: { type: "object" }, handler: (a) => String(a.x + a.y) });
const h = await srv.listen({ port: 0 });
const c = await mcp.connect.http(h.url, { maxRetries: 0 });

const tools = await c.listTools();
runtime.assert.equal(tools.map(t => t.name).join(","), "add", "tool names");
const res = await c.callTool("add", { x: 2, y: 3 });
runtime.assert.equal(res.isError, false, "add not error");
runtime.assert.equal(res.content[0].text, "5", "add result");

await c.close();
await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientOAuth is the Phase-4 dogfood gate for connect.http's
// auth.getToken option: it drives a client against the server's OWN OAuth
// resource-server middleware (srv.listen({ auth: { verify, resourceMetadata
// } }), server Phase 3 — see mcp_auth_test.go). A client whose getToken
// returns the accepted token connects and calls a tool successfully; a
// client whose getToken returns a rejected token fails (the resulting 401 on
// the Streamable HTTP initialize request surfaces as a thrown error, since
// the auth middleware guards the whole endpoint, not just tool calls).
func TestMCPClientOAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "oauth.ts", `
const srv = mcp.serve({ name: "f", version: "1.0.0" });
srv.tool({ name: "add", inputSchema: { type: "object" }, handler: (a) => String(a.x + a.y) });
const h = await srv.listen({
	port: 0,
	auth: {
		verify: (token) => token === "good" ? { subject: "u1" } : null,
		resourceMetadata: { authorizationServers: ["https://auth.example.com"] },
	},
});

const good = await mcp.connect.http(h.url, { auth: { getToken: () => "good" } });
const res2 = await good.callTool("add", { x: 2, y: 3 });
runtime.assert.equal(res2.isError, false, "add not error");
runtime.assert.equal(res2.content[0].text, "5", "add result");
await good.close();

let badErr = null;
try {
	const bad = await mcp.connect.http(h.url, { auth: { getToken: () => "bad" } });
	await bad.callTool("add", { x: 1, y: 1 });
	await bad.close();
} catch (e) {
	badErr = e;
}
runtime.assert.ok(badErr, "bad token should throw somewhere in connect+callTool");

await h.close();
`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPClientOAuth_RequiresGetToken asserts that an `auth` object present
// but missing a `getToken` function throws a clear TypeError rather than
// silently connecting unauthenticated.
func TestMCPClientOAuth_RequiresGetToken(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(ctx, "oauth-bad.ts", `
await mcp.connect.http("http://127.0.0.1:1/mcp", { auth: {} });
`)
	if err == nil {
		t.Fatal("expected throw for auth without getToken")
	}
}
