// mcp-client-http.ts — mcp.connect.http end to end, fully offline.
//
// Starts an in-script MCP server (mcp.serve + srv.listen on a random port),
// connects a client to it with mcp.connect.http, exercises the Phase-1 consume
// surface (serverInfo, listTools, callTool incl. an isError case, listResources
// + readResource, listPrompts + getPrompt, ping) plus a slice of the Phase-2
// reactive surface (subscribe + a server-pushed resourceUpdated notification
// firing onResourceUpdated), then a Phase-3 host round trip: a second client
// connects with onSample/roots so the server's "summarize"/"listRoots" tools
// (ctx.sample/ctx.roots) get answered by the client instead of rejecting, and
// c2.setRoots(...) updates the root set the server sees. Both connections
// close at the end. Self-testing so it runs under `make demo` — both sides are
// sercon, so the round trip is fully hermetic.

const srv = mcp.serve({ name: "demo-server", version: "1.0.0" });
srv.tool({ name: "add", description: "add two numbers", inputSchema: {
  type: "object", properties: { a: { type: "number" }, b: { type: "number" } }, required: ["a", "b"],
}, handler: (args) => String(args.a + args.b) });
srv.tool({ name: "fail", inputSchema: { type: "object" }, handler: () => { throw new Error("intentional"); } });
srv.resource({ uri: "cfg://app", name: "config", read: () => ({ text: JSON.stringify({ theme: "dark" }) }) });
srv.prompt({ name: "greet", arguments: [{ name: "who", required: true }],
  get: (a) => ({ messages: [{ role: "user", content: { type: "text", text: `Hello, ${a.who}!` } }] }) });

// Phase 3: tools that ask the connected client for things — answered below by
// a client that supplies onSample/roots (see host, further down).
srv.tool({ name: "summarize", inputSchema: { type: "object" }, handler: async (_args, ctx) => {
  const r = await ctx.sample({ messages: [{ role: "user", content: { type: "text", text: "hi" } }], maxTokens: 50 });
  return r.content.text;
} });
srv.tool({ name: "listRoots", inputSchema: { type: "object" }, handler: async (_args, ctx) => {
  const roots = await ctx.roots();
  return roots.map((r) => r.uri).sort().join(",");
} });

const h = await srv.listen({ port: 0 });
runtime.log(`server listening at ${h.url}`);

const c = await mcp.connect.http(h.url);
runtime.log(`connected to ${c.serverInfo.name} v${c.serverInfo.version}`);
runtime.assert.equal(c.serverInfo.name, "demo-server", "serverInfo.name");

const tools = await c.listTools();
runtime.assert.equal(tools.map(t => t.name).sort().join(","), "add,fail,listRoots,summarize", "tools listed");

const sum = await c.callTool("add", { a: 2, b: 40 });
runtime.assert.equal(sum.isError, false, "add ok");
runtime.assert.equal(sum.content[0].text, "42", "add=42");

const bad = await c.callTool("fail", {});
runtime.assert.equal(bad.isError, true, "fail -> isError (not a throw)");

const doc = await c.readResource("cfg://app");
runtime.assert.ok(doc.contents[0].text.includes("dark"), "resource read");

// Phase 2: subscribe to the resource, then have the server push an update —
// onResourceUpdated should fire with the subscribed uri.
let updatedURI = "";
c.onResourceUpdated((uri) => { updatedURI = uri; });
await c.subscribe("cfg://app");
await srv.resourceUpdated("cfg://app");
const deadline = Date.now() + 5000;
while (updatedURI === "" && Date.now() < deadline) {
  await new Promise((r) => setTimeout(r, 20));
}
runtime.assert.equal(updatedURI, "cfg://app", "onResourceUpdated fired after subscribe");
await c.unsubscribe("cfg://app");

const p = await c.getPrompt("greet", { who: "world" });
runtime.assert.ok(JSON.stringify(p.messages).includes("Hello, world!"), "prompt rendered");

await c.ping();

// Phase 3: a second connection acts as the "host", answering the server's
// sampling/roots requests via onSample/roots. c (above) supplied neither, so
// the server calling ctx.sample()/ctx.roots() against c's session would
// reject — this is a separate client specifically to demonstrate the
// responder side.
const host = await mcp.connect.http(h.url, {
  onSample: (req) => "SUMMARY:" + req.messages[0].content.text,
  roots: [{ uri: "file:///a" }, { uri: "file:///b" }],
});

const summarized = await host.callTool("summarize", {});
runtime.assert.equal(summarized.isError, false, "summarize ok");
runtime.assert.equal(summarized.content[0].text, "SUMMARY:hi", "onSample answered ctx.sample");

const roots1 = await host.callTool("listRoots", {});
runtime.assert.equal(roots1.content[0].text, "file:///a,file:///b", "initial roots seeded");

host.setRoots([{ uri: "file:///c" }]);
const roots2 = await host.callTool("listRoots", {});
runtime.assert.equal(roots2.content[0].text, "file:///c", "setRoots updated the root set");

runtime.log("mcp-client-http OK — closing");
await host.close();
await c.close();
await h.close();
