// Demonstrates mcp.serve()'s Streamable HTTP transport — srv.listen(...).
// Unlike srv.stdio() (single peer over stdin/stdout, one process per
// client), listen() serves the MCP server over a plain TCP/HTTP endpoint
// that any number of Streamable-HTTP clients can connect to.
//
// This script is its own client: it speaks the wire protocol directly via
// net.http.request rather than pulling in an MCP client library, so the demo
// stays dependency-free and runnable under `make demo`. That means doing the
// handshake sercon's own net.http binding has to do by hand:
//   - POST with `Content-Type: application/json` and an `Accept` header
//     naming BOTH `application/json` and `text/event-stream` (the Streamable
//     HTTP spec requires the client to advertise it can take either).
//   - The server (registered here with default options, i.e. SSE responses,
//     not the JSONResponse opt-in) always answers a call with a
//     `text/event-stream` body — one `data: <json-rpc message>` frame — even
//     though nothing here asks for a long-lived stream. parseSSE below pulls
//     the JSON-RPC message out of that frame.
//   - The `initialize` response carries the session id in the
//     `Mcp-Session-Id` response header; every request after that must send
//     it back on `Mcp-Session-Id`.
//   - Per spec, after `initialize` the client sends the `notifications/
//     initialized` notification (no `id`, so the server replies 202 with an
//     empty body) before making any other call.
//
// This exercises the real over-the-wire framing end to end (initialize +
// notifications/initialized + tools/call), which is why it isn't the lighter
// "just POST initialize and close" fallback the task brief allows for — the
// SSE-vs-JSON response format turned out to be a one-line parse, not a
// blocker. cmd/sercon/mcp_http_test.go additionally drives the same handle
// with the real SDK client (mcp.StreamableClientTransport) as the
// authoritative round-trip.
//
// Also exercises two Phase-2 additions end to end, deliberately via plain
// request/response (not server->client notifications, which would need this
// hand-rolled client to also read the standalone SSE stream — real MCP
// clients like the SDK's own do that, but it's unnecessary complexity for a
// fast, deterministic `make demo` script):
//   - Runtime mutation: `srv.tool(...)` is called again AFTER `listen()` has
//     already started (registering a second tool, "multiply"), then
//     `srv.removeTool("multiply")` retracts it — both allowed at any time
//     since Phase 2 (each fires a `tools/list_changed` notification to
//     connected clients, which this script doesn't listen for, but a real
//     client would). `tools/list` before/after confirms the tool set
//     actually changed both times.
//   - Resource templates: `srv.resourceTemplate(...)` registers an RFC 6570
//     URI-templated resource family; `resources/templates/list` confirms it's
//     advertised, and `resources/read` against a concrete URI confirms the
//     `read` handler receives the resolved URI (not the template string).

function parseSSE(body: string): any {
  const dataLine = body.split("\n").find((l) => l.startsWith("data:"));
  if (!dataLine) throw new Error("no SSE data frame in response: " + body);
  return JSON.parse(dataLine.slice(5).trim());
}

// rpc issues one JSON-RPC call (always with an id, so it always gets a
// result frame back) against the already-initialized session and returns the
// parsed message — a small helper for the Phase-2 scenarios below, which
// issue several more calls than the handshake walkthrough above.
let nextId = 100;
async function rpc(url: string, headers: Record<string, string>, method: string, params?: any): Promise<any> {
  const res = await net.http.request("POST", url, {
    headers,
    body: JSON.stringify({ jsonrpc: "2.0", id: nextId++, method, params }),
  });
  runtime.assert.equal(res.status, 200, `${method} status`);
  return parseSSE(res.body);
}

const port = 38084;

const srv = mcp.serve({ name: "sercon-http-demo", version: "1.0.0" });

srv.tool({
  name: "add",
  description: "add two numbers",
  inputSchema: {
    type: "object",
    properties: { a: { type: "number" }, b: { type: "number" } },
    required: ["a", "b"],
  },
  async handler(args: any) {
    return String(args.a + args.b);
  },
});

srv.resourceTemplate({
  uriTemplate: "demo:///{table}/{id}",
  name: "row",
  mimeType: "application/json",
  read: (uri: string) => ({ text: JSON.stringify({ uri }) }),
});

const h = await srv.listen({ port });
runtime.log("listening at", h.url);

const jsonRPCHeaders = {
  "Content-Type": "application/json",
  Accept: "application/json, text/event-stream",
};

// 1) initialize
const initRes = await net.http.request("POST", h.url, {
  headers: jsonRPCHeaders,
  body: JSON.stringify({
    jsonrpc: "2.0",
    id: 1,
    method: "initialize",
    params: {
      protocolVersion: "2025-06-18",
      capabilities: {},
      clientInfo: { name: "sercon-demo-client", version: "1.0.0" },
    },
  }),
});
runtime.assert.equal(initRes.status, 200, "initialize status");
const sessionId = initRes.headers["mcp-session-id"];
runtime.assert.ok(!!sessionId, "Mcp-Session-Id header present");
const initMsg = parseSSE(initRes.body);
runtime.assert.equal(initMsg.result.serverInfo.name, "sercon-http-demo", "serverInfo.name");
runtime.log("initialized, session:", sessionId);

const sessionHeaders = { ...jsonRPCHeaders, "Mcp-Session-Id": sessionId };

// 2) notifications/initialized (no id -> 202 Accepted, empty body)
const notifyRes = await net.http.request("POST", h.url, {
  headers: sessionHeaders,
  body: JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }),
});
runtime.assert.equal(notifyRes.status, 202, "notifications/initialized status");

// 3) tools/call
const callRes = await net.http.request("POST", h.url, {
  headers: sessionHeaders,
  body: JSON.stringify({
    jsonrpc: "2.0",
    id: 2,
    method: "tools/call",
    params: { name: "add", arguments: { a: 2, b: 3 } },
  }),
});
runtime.assert.equal(callRes.status, 200, "tools/call status");
const callMsg = parseSSE(callRes.body);
runtime.assert.equal(callMsg.result.content[0].text, "5", "add result");
runtime.log("tools/call add(2, 3) ->", callMsg.result.content[0].text);

// 4) Phase-2: resource template — list, then read a concrete URI matching
// the "demo:///{table}/{id}" pattern registered above.
const templatesMsg = await rpc(h.url, sessionHeaders, "resources/templates/list");
const templateNames = templatesMsg.result.resourceTemplates.map((t: any) => t.name);
runtime.assert.ok(templateNames.includes("row"), "resource template 'row' listed");

const readMsg = await rpc(h.url, sessionHeaders, "resources/read", { uri: "demo:///widgets/7" });
const readContents = JSON.parse(readMsg.result.contents[0].text);
runtime.assert.equal(readContents.uri, "demo:///widgets/7", "resourceTemplate read receives the resolved URI");
runtime.log("resources/read demo:///widgets/7 ->", readMsg.result.contents[0].text);

// 5) Phase-2: runtime tool mutation — register "multiply" AFTER listen()
// has already started, confirm it shows up in tools/list and is callable,
// then removeTool() it and confirm it's gone again.
srv.tool({
  name: "multiply",
  description: "multiply two numbers",
  inputSchema: {
    type: "object",
    properties: { a: { type: "number" }, b: { type: "number" } },
    required: ["a", "b"],
  },
  handler: (args: any) => String(args.a * args.b),
});

const afterAddMsg = await rpc(h.url, sessionHeaders, "tools/list");
const afterAddNames = afterAddMsg.result.tools.map((t: any) => t.name);
runtime.assert.ok(afterAddNames.includes("multiply"), "'multiply' registered after listen() is listed");

const multiplyMsg = await rpc(h.url, sessionHeaders, "tools/call", { name: "multiply", arguments: { a: 4, b: 5 } });
runtime.assert.equal(multiplyMsg.result.content[0].text, "20", "multiply result");
runtime.log("tools/call multiply(4, 5) ->", multiplyMsg.result.content[0].text);

srv.removeTool("multiply");
const afterRemoveMsg = await rpc(h.url, sessionHeaders, "tools/list");
const afterRemoveNames = afterRemoveMsg.result.tools.map((t: any) => t.name);
runtime.assert.ok(!afterRemoveNames.includes("multiply"), "'multiply' no longer listed after removeTool()");
runtime.log("removeTool('multiply') -> tools/list:", afterRemoveNames.join(", "));

await h.close();
runtime.log("closed");
