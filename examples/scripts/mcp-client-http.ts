// mcp-client-http.ts — mcp.connect.http end to end, fully offline.
//
// Starts an in-script MCP server (mcp.serve + srv.listen on a random port),
// connects a client to it with mcp.connect.http, exercises the Phase-1 consume
// surface (serverInfo, listTools, callTool incl. an isError case, listResources
// + readResource, listPrompts + getPrompt, ping), then closes both. Self-testing
// so it runs under `make demo`.

const srv = mcp.serve({ name: "demo-server", version: "1.0.0" });
srv.tool({ name: "add", description: "add two numbers", inputSchema: {
  type: "object", properties: { a: { type: "number" }, b: { type: "number" } }, required: ["a", "b"],
}, handler: (args) => String(args.a + args.b) });
srv.tool({ name: "fail", inputSchema: { type: "object" }, handler: () => { throw new Error("intentional"); } });
srv.resource({ uri: "cfg://app", name: "config", read: () => ({ text: JSON.stringify({ theme: "dark" }) }) });
srv.prompt({ name: "greet", arguments: [{ name: "who", required: true }],
  get: (a) => ({ messages: [{ role: "user", content: { type: "text", text: `Hello, ${a.who}!` } }] }) });

const h = await srv.listen({ port: 0 });
runtime.log(`server listening at ${h.url}`);

const c = await mcp.connect.http(h.url);
runtime.log(`connected to ${c.serverInfo.name} v${c.serverInfo.version}`);
runtime.assert.equal(c.serverInfo.name, "demo-server", "serverInfo.name");

const tools = await c.listTools();
runtime.assert.equal(tools.map(t => t.name).sort().join(","), "add,fail", "tools listed");

const sum = await c.callTool("add", { a: 2, b: 40 });
runtime.assert.equal(sum.isError, false, "add ok");
runtime.assert.equal(sum.content[0].text, "42", "add=42");

const bad = await c.callTool("fail", {});
runtime.assert.equal(bad.isError, true, "fail -> isError (not a throw)");

const doc = await c.readResource("cfg://app");
runtime.assert.ok(doc.contents[0].text.includes("dark"), "resource read");

const p = await c.getPrompt("greet", { who: "world" });
runtime.assert.ok(JSON.stringify(p.messages).includes("Hello, world!"), "prompt rendered");

await c.ping();
runtime.log("mcp-client-http OK — closing");
await c.close();
await h.close();
