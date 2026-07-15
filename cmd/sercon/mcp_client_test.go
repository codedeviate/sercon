package main

import (
	"context"
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
