// Demonstrates the "MCP tool as an API bridge" pattern: a tool whose handler
// forwards the call to an external HTTP API and hands the result back to the
// model. To keep this hermetic and runnable offline under `make demo`, the
// "external" API is a tiny fake upstream started with server.http in this
// same script rather than a real public endpoint — the bridging code (an
// MCP tool handler calling out over net.http) is identical either way; only
// the URL would change to point at a real service.
//
// Same self-test harness as mcp-server-http.ts / mcp-toolbox.ts: this script
// is its own client, speaking raw JSON-RPC over net.http.request.

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

// === The fake upstream: a tiny "weather API" with one canned endpoint ===

const upstreamPort = 39011;
const WEATHER: Record<string, { tempC: number; conditions: string }> = {
  stockholm: { tempC: 14, conditions: "overcast" },
  "san francisco": { tempC: 18, conditions: "foggy" },
};

const upstream = await server.http.listen({
  port: upstreamPort,
  routes: {
    "GET /weather": (req: any, res: any) => {
      const city = String(req.query.city?.[0] ?? "").toLowerCase();
      const data = WEATHER[city];
      if (!data) return res.status(404).json({ error: "unknown city" });
      return res.json({ city, ...data });
    },
  },
});

// === The MCP server: one tool that bridges to the upstream over net.http ===

const port = 39010;
const srv = mcp.serve({ name: "sercon-bridge-demo", version: "1.0.0" });

srv.tool({
  name: "get_weather",
  description: "Look up current weather for a city via the (fake, local) upstream weather API.",
  inputSchema: {
    type: "object",
    properties: { city: { type: "string", description: "City name" } },
    required: ["city"],
  },
  async handler(args: any) {
    // net.http.request (not .get) — .get's result is the plain
    // { status, body, bodyBytes } shape with no `ok`; .request's richer
    // { status, ok, headers, body, url } is what lets the bridge tell a
    // successful response from a failed one.
    const res = await net.http.request("GET", `http://127.0.0.1:${upstreamPort}/weather?city=${encodeURIComponent(args.city)}`);
    if (!res.ok) {
      return { content: [{ type: "text", text: `upstream error: ${res.status}` }], isError: true };
    }
    return res.body; // upstream already returns JSON; pass it through as-is
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
      clientInfo: { name: "sercon-bridge-client", version: "1.0.0" },
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

// 3) tools/call — bridge through to the fake upstream and back.
const okMsg = await rpc(h.url, sessionHeaders, "tools/call", { name: "get_weather", arguments: { city: "Stockholm" } });
const weather = JSON.parse(okMsg.result.content[0].text);
runtime.assert.equal(weather.city, "stockholm", "bridged city echoed back");
runtime.assert.equal(weather.tempC, 14, "bridged tempC from the fake upstream");
runtime.log("get_weather(Stockholm) ->", JSON.stringify(weather));

// An unknown city surfaces the upstream's 404 as a tool-level isError, not a
// protocol error — the bridge translates transport failures into MCP's own
// error shape rather than letting them propagate raw.
const missMsg = await rpc(h.url, sessionHeaders, "tools/call", { name: "get_weather", arguments: { city: "Nowhere" } });
runtime.assert.ok(missMsg.result.isError, "unknown city surfaces as isError");
runtime.log("get_weather(Nowhere) -> isError:", missMsg.result.isError, missMsg.result.content[0].text);

await h.close();
await upstream.close();
runtime.log("mcp-bridge OK — closed");
