// The headline "advanced" MCP example: sercon's own reserved-global surface
// (net.http, db.sqlite, image, crypto, fs) wrapped as five MCP tools — the
// pitch being "sercon = instant LLM toolbox": drop one binary in front of an
// LLM client and it can already fetch headers, query a database, inspect an
// image, hash text, and read files, with zero extra dependencies.
//
// Like mcp-server-http.ts, this script is its own client: it speaks the wire
// protocol directly via net.http.request (initialize + notifications/
// initialized + tools/call), so the demo stays dependency-free and runnable
// under `make demo`. See that file's header comment for the framing details
// (SSE response parsing, Mcp-Session-Id) — not repeated here.
//
// Everything the tools touch is set up hermetically in-process: an
// in-memory SQLite catalog, a temp directory with a text file and a sample
// PNG, and a tiny local server.http target for the HTTP tool to hit — no
// outbound network, no on-disk state left behind after the run.

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

// === Set up the things the tools below wrap ===

// db.sqlite: a tiny in-memory product catalog.
const conn = await db.sqlite.open(":memory:");
await conn.exec("CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT, price REAL)");
await conn.exec("INSERT INTO products (name, price) VALUES (?, ?)", "widget", 9.99);
await conn.exec("INSERT INTO products (name, price) VALUES (?, ?)", "gadget", 19.99);

// fs: a scratch directory with a text file and a sample image, cleaned up
// at the end of the script.
const dir = "mcp-toolbox-demo";
await fs.remove(dir); // clean slate (no error if absent)
await fs.mkdir(dir);
await fs.writeText(`${dir}/note.txt`, "sercon can read this file for an LLM.");

// A real 16x16 PNG (RGB), 89 bytes — same fixture examples/scripts/image.ts uses.
const PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,16,0,0,0,16,8,2,0,0,0,144,145,104,54,0,0,0,32,73,68,65,84,120,156,98,97,96,104,16,96,96,32,30,177,128,8,82,192,168,134,81,13,67,71,3,32,0,0,255,255,39,58,2,161,212,168,74,89,0,0,0,0,73,69,78,68,174,66,96,130]);
await fs.writeBytes(`${dir}/sample.png`, PNG);

// net.http: a tiny local target server for the http_headers tool to hit, so
// the demo never reaches outside the process (hermetic, offline-safe).
const targetPort = 39001;
const target = await server.http.listen({
  port: targetPort,
  routes: {
    "GET /": (_req: any, res: any) => res.header("X-Sercon-Toolbox", "1").text("hello from the toolbox target"),
  },
});

// === The MCP server: five tools, each a thin wrapper over one sercon global ===

const port = 39000;
const srv = mcp.serve({ name: "sercon-toolbox", version: "1.0.0" });

srv.tool({
  name: "http_headers",
  description: "Fetch a URL and report its HTTP status and response headers (wraps net.http.request).",
  inputSchema: {
    type: "object",
    properties: { url: { type: "string", description: "URL to fetch" } },
    required: ["url"],
  },
  async handler(args: any) {
    const res = await net.http.request("GET", args.url);
    return JSON.stringify({ status: res.status, headers: res.headers });
  },
});

srv.tool({
  name: "sqlite_query",
  description: "Run a SQL query against the toolbox's in-memory product catalog and return the rows as JSON (wraps db.sqlite).",
  inputSchema: {
    type: "object",
    properties: { sql: { type: "string", description: "SQL SELECT statement" } },
    required: ["sql"],
  },
  async handler(args: any) {
    const rows = await conn.query(args.sql);
    return JSON.stringify(rows);
  },
});

srv.tool({
  name: "image_dims",
  description: "Report the pixel dimensions and format of an image file on disk (wraps fs + image).",
  inputSchema: {
    type: "object",
    properties: { path: { type: "string", description: "Path to an image file" } },
    required: ["path"],
  },
  async handler(args: any) {
    const bytes = await fs.readBytes(args.path);
    const im = image.decode(bytes);
    return JSON.stringify({ width: im.width, height: im.height, format: im.format });
  },
});

srv.tool({
  name: "sha256",
  description: "Compute the SHA-256 hex digest of a UTF-8 string (wraps crypto.hash).",
  inputSchema: {
    type: "object",
    properties: { text: { type: "string" } },
    required: ["text"],
  },
  handler: (args: any) => crypto.hash.sha256(args.text),
});

srv.tool({
  name: "read_text",
  description: "Read a UTF-8 text file from disk and return its contents (wraps fs.readText).",
  inputSchema: {
    type: "object",
    properties: { path: { type: "string" } },
    required: ["path"],
  },
  handler: (args: any) => fs.readText(args.path),
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
      clientInfo: { name: "sercon-toolbox-client", version: "1.0.0" },
    },
  }),
});
runtime.assert.equal(initRes.status, 200, "initialize status");
const sessionId = initRes.headers["mcp-session-id"];
runtime.assert.ok(!!sessionId, "Mcp-Session-Id header present");
runtime.log("initialized, session:", sessionId);

const sessionHeaders = { ...jsonRPCHeaders, "Mcp-Session-Id": sessionId };

// 2) notifications/initialized
const notifyRes = await net.http.request("POST", h.url, {
  headers: sessionHeaders,
  body: JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized" }),
});
runtime.assert.equal(notifyRes.status, 202, "notifications/initialized status");

// 3) tools/list — all five tools are registered and advertised.
const toolsMsg = await rpc(h.url, sessionHeaders, "tools/list");
const toolNames = toolsMsg.result.tools.map((t: any) => t.name).sort();
runtime.assert.equal(
  toolNames.join(","),
  ["http_headers", "image_dims", "read_text", "sha256", "sqlite_query"].join(","),
  "all five toolbox tools listed",
);

// 4) tools/call — exercise every tool at least once.
const headersMsg = await rpc(h.url, sessionHeaders, "tools/call", {
  name: "http_headers",
  arguments: { url: `http://127.0.0.1:${targetPort}/` },
});
const headersOut = JSON.parse(headersMsg.result.content[0].text);
runtime.assert.equal(headersOut.status, 200, "http_headers status");
runtime.assert.equal(headersOut.headers["x-sercon-toolbox"], "1", "http_headers sees the target's custom header");
runtime.log("http_headers ->", JSON.stringify(headersOut));

const queryMsg = await rpc(h.url, sessionHeaders, "tools/call", {
  name: "sqlite_query",
  arguments: { sql: "SELECT name, price FROM products ORDER BY price" },
});
const rows = JSON.parse(queryMsg.result.content[0].text);
runtime.assert.equal(rows.length, 2, "sqlite_query returns both rows");
runtime.assert.equal(rows[0].name, "widget", "sqlite_query ordered by price");
runtime.log("sqlite_query ->", JSON.stringify(rows));

const dimsMsg = await rpc(h.url, sessionHeaders, "tools/call", {
  name: "image_dims",
  arguments: { path: `${dir}/sample.png` },
});
const dims = JSON.parse(dimsMsg.result.content[0].text);
runtime.assert.equal(dims.width, 16, "image_dims width");
runtime.assert.equal(dims.height, 16, "image_dims height");
runtime.log("image_dims ->", JSON.stringify(dims));

const shaMsg = await rpc(h.url, sessionHeaders, "tools/call", { name: "sha256", arguments: { text: "abc" } });
runtime.assert.equal(
  shaMsg.result.content[0].text,
  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
  "sha256 matches the known vector for 'abc'",
);
runtime.log("sha256 ->", shaMsg.result.content[0].text);

const readMsg = await rpc(h.url, sessionHeaders, "tools/call", { name: "read_text", arguments: { path: `${dir}/note.txt` } });
runtime.assert.equal(readMsg.result.content[0].text, "sercon can read this file for an LLM.", "read_text round-trips the file");
runtime.log("read_text ->", readMsg.result.content[0].text);

// === Teardown ===
await h.close();
await target.close();
await conn.close();
await fs.remove(dir);
runtime.log("mcp-toolbox OK — closed");
