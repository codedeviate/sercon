package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
