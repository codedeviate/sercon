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

function parseSSE(body: string): any {
  const dataLine = body.split("\n").find((l) => l.startsWith("data:"));
  if (!dataLine) throw new Error("no SSE data frame in response: " + body);
  return JSON.parse(dataLine.slice(5).trim());
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

await h.close();
runtime.log("closed");
