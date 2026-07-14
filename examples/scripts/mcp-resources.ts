// Demonstrates the MCP resources surface: two static srv.resource()
// registrations plus one srv.resourceTemplate() (an RFC 6570 URI template),
// self-tested via resources/list, resources/templates/list, and
// resources/read against both a static URI and a concrete templated URI.
//
// Same self-test harness as mcp-server-http.ts: this script is its own
// client, speaking raw JSON-RPC over net.http.request (initialize +
// notifications/initialized + the resources/* calls), then closes.

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

const port = 39020;
const srv = mcp.serve({ name: "sercon-resources-demo", version: "1.0.0" });

// Two static resources — fixed URIs, always the same content.
srv.resource({
  uri: "config://app/settings",
  name: "app-settings",
  mimeType: "application/json",
  read: () => ({ text: JSON.stringify({ theme: "dark", maxItems: 50 }) }),
});

srv.resource({
  uri: "docs://readme",
  name: "readme",
  mimeType: "text/plain",
  read: () => ({ text: "This is the sercon MCP resources demo." }),
});

// A resource template — the client reads any concrete URI matching the
// pattern; the resolved URI (not the template string) is what the `read`
// function receives, so a single registration serves a whole family of
// resources (here, one per user id).
const USERS: Record<string, { name: string }> = {
  "1": { name: "Alice" },
  "2": { name: "Bob" },
};

srv.resourceTemplate({
  uriTemplate: "users:///{id}",
  name: "user",
  mimeType: "application/json",
  read: (uri: string) => {
    const id = uri.split("/").pop()!;
    const user = USERS[id];
    if (!user) throw new Error(`no such user: ${id}`);
    return { text: JSON.stringify({ id, ...user }) };
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
      clientInfo: { name: "sercon-resources-client", version: "1.0.0" },
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

// 3) resources/list — both static resources are advertised.
const listMsg = await rpc(h.url, sessionHeaders, "resources/list");
const uris = listMsg.result.resources.map((r: any) => r.uri).sort();
runtime.assert.equal(uris.join(","), ["config://app/settings", "docs://readme"].join(","), "both static resources listed");

// 4) resources/templates/list — the template is advertised separately.
const templatesMsg = await rpc(h.url, sessionHeaders, "resources/templates/list");
const templateNames = templatesMsg.result.resourceTemplates.map((t: any) => t.name);
runtime.assert.ok(templateNames.includes("user"), "resource template 'user' listed");

// 5) resources/read — a static resource.
const settingsMsg = await rpc(h.url, sessionHeaders, "resources/read", { uri: "config://app/settings" });
const settings = JSON.parse(settingsMsg.result.contents[0].text);
runtime.assert.equal(settings.theme, "dark", "static resource content read back");
runtime.log("config://app/settings ->", settingsMsg.result.contents[0].text);

// 6) resources/read — a concrete URI matching the template.
const userMsg = await rpc(h.url, sessionHeaders, "resources/read", { uri: "users:///2" });
const user = JSON.parse(userMsg.result.contents[0].text);
runtime.assert.equal(user.name, "Bob", "resourceTemplate resolves the concrete URI's variable");
runtime.log("users:///2 ->", userMsg.result.contents[0].text);

await h.close();
runtime.log("mcp-resources OK — closed");
