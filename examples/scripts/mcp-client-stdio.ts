// mcp-client-stdio.ts — mcp.connect.stdio: launch an MCP server subprocess and
// drive it over stdio. Here the server is another sercon running the
// mcp-server-stdio.ts fixture, so this file is a two-process demo.
//
// NOT a `make demo` script: it spawns a subprocess and needs the sercon binary
// on PATH (or an explicit path). Driven by cmd/sercon/mcp_client_test.go
// (TestMCPClientStdio), which passes the freshly built binary path via argv.

const bin = runtime.env.get("SERCON_BIN") ?? "sercon";
const server = runtime.env.get("MCP_SERVER_SCRIPT") ?? "examples/scripts/mcp-server-stdio.ts";

const c = await mcp.connect.stdio({ command: [bin, server] });
runtime.log(`connected to ${c.serverInfo.name}`);
const r = await c.callTool("add", { a: 2, b: 3 });
runtime.assert.equal(r.content[0].text, "5", "add over stdio = 5");
runtime.log("mcp-client-stdio OK — closing");
await c.close();
