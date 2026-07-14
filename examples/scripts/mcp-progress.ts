// Demonstrates ctx.progress / ctx.log — the two request-context hooks a
// tool handler can use to report incremental status back to the client
// while a long-ish operation runs (see the `ctx` object built by
// newRequestContext in cmd/sercon/mcp_server.go).
//
// LIMITATION, read before copying this pattern: progress and log
// notifications aren't part of the JSON-RPC response to the request that
// triggered them — the go-sdk always sends them to the *standalone* SSE
// stream a client opens with a bare GET on the MCP endpoint (kept open for
// the life of the session) rather than the per-call POST response stream.
// This demo's self-test client, like mcp-server-http.ts's, never opens
// that standalone stream — doing so would mean a second concurrent request
// against the same handle, which is unnecessary complexity for a fast,
// deterministic `make demo` script (a real client, e.g. the SDK's own,
// keeps that stream open and receives these normally). Concretely: this
// harness attaches a progressToken to the tools/call request (so the tool
// handler's ctx.progress() takes the real send path, not a no-op), but
// because nothing has claimed the standalone stream to deliver onto, the
// underlying notification can fail to deliver — so the handler treats
// ctx.progress()/ctx.log() as best-effort telemetry and swallows any
// delivery error, exactly as a fire-and-forget status update should. The
// self-test therefore only asserts the one thing reliably observable over
// a raw client: the tool still runs to completion and returns the right
// result.

function parseSSE(body: string): any {
  const dataLine = body.split("\n").find((l) => l.startsWith("data:"));
  if (!dataLine) throw new Error("no SSE data frame in response: " + body);
  return JSON.parse(dataLine.slice(5).trim());
}

let nextId = 100;
async function rpc(url: string, headers: Record<string, string>, method: string, params?: any): Promise<any> {
  const res = await net.http.request("POST", url, {
    headers,
    body: JSON.stringify({ jsonrpc: "2.0", id: nextId++, method, params }),
  });
  runtime.assert.equal(res.status, 200, `${method} status`);
  return parseSSE(res.body);
}

const port = 39030;
const srv = mcp.serve({ name: "sercon-progress-demo", version: "1.0.0" });

srv.tool({
  name: "count_to",
  description: "Count from 1 to n, reporting progress and a log line at each step.",
  inputSchema: {
    type: "object",
    properties: { n: { type: "number", description: "How high to count" } },
    required: ["n"],
  },
  async handler(args: any, ctx: any) {
    const n = args.n;
    for (let i = 1; i <= n; i++) {
      // Best-effort: see the file header comment for why delivery failures
      // are swallowed rather than allowed to fail the tool call.
      await ctx.progress(i, n).catch(() => {});
      await ctx.log("info", `counted to ${i}`).catch(() => {});
    }
    return `counted to ${n}`;
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
      clientInfo: { name: "sercon-progress-client", version: "1.0.0" },
    },
  }),
});
runtime.assert.equal(initRes.status, 200, "initialize status");
const sessionId = initRes.headers["mcp-session-id"];
runtime.assert.ok(!!sessionId, "Mcp-Session-Id header present");

const sessionHeaders = { ...jsonRPCHeaders, "Mcp-Session-Id": sessionId };

// 2) notifications/initialized
const notifyRes = await net.http.request("POST", h.url, {
  headers: sessionHeaders,
  body: JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }),
});
runtime.assert.equal(notifyRes.status, 202, "notifications/initialized status");

// 3) tools/call — attach a progressToken (as a real client would) so the
// handler's ctx.progress() calls exercise the genuine send path, not the
// tok-is-nil fast path. See the file header for why we don't try to read
// the resulting notifications.
const callMsg = await rpc(h.url, sessionHeaders, "tools/call", {
  name: "count_to",
  arguments: { n: 5 },
  _meta: { progressToken: "count-to-5" },
});
runtime.assert.equal(callMsg.result.content[0].text, "counted to 5", "tool completes and returns despite best-effort progress/log");
runtime.log("count_to(5) ->", callMsg.result.content[0].text);

await h.close();
runtime.log("mcp-progress OK — closed");
