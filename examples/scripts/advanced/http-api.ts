// Demonstrates a fuller HTTP API with middleware chains, in-memory CRUD
// store, bearer-token auth, and a health endpoint. Uses query params for
// item IDs because path params (req.params) are not yet populated by
// server.http. Self-tests every route including a 401 path.

const port = 38200;

// ── in-memory store ──────────────────────────────────────────────────────────
let nextId = 1;
const items = new Map<number, { id: number; name: string }>();

// ── middleware ───────────────────────────────────────────────────────────────
const TOKEN = "s3cr3t";

const requestLogger = async (req: any, res: any, next: any) => {
  const t0 = runtime.time.nowMs();
  await next();
  runtime.log(`${req.method} ${req.path} → ${runtime.time.nowMs() - t0}ms`);
};

const authMiddleware = async (req: any, res: any, next: any) => {
  // Only protect non-health routes.
  if (req.path === "/health") {
    return next();
  }
  const authHeader: string = (req.headers["authorization"] ?? [""])[0];
  if (authHeader !== `Bearer ${TOKEN}`) {
    res.status(401).json({ error: "unauthorized" });
    return; // do NOT call next — response is finalized
  }
  return next();
};

const errorCatcher = async (req: any, res: any, next: any) => {
  try {
    return await next();
  } catch (e: any) {
    res.status(500).json({ error: String(e) });
  }
};

// ── server ───────────────────────────────────────────────────────────────────
const srv = await server.http.listen({
  port,
  use: [requestLogger, authMiddleware, errorCatcher],
  routes: {
    "GET /health": (_req: any, res: any) => res.json({ ok: true }),

    "GET /items": (_req: any, res: any) =>
      res.json(Array.from(items.values())),

    "POST /items": (req: any, res: any) => {
      const body = JSON.parse(req.body || "{}");
      if (!body.name) return res.status(400).json({ error: "name required" });
      const id = nextId++;
      items.set(id, { id, name: body.name });
      res.status(201).json({ id, name: body.name });
    },

    "GET /items/by-id": (req: any, res: any) => {
      const id = Number((req.query.id ?? [])[0]);
      const item = items.get(id);
      if (!item) return res.status(404).json({ error: "not found" });
      res.json(item);
    },

    "DELETE /items/by-id": (req: any, res: any) => {
      const id = Number((req.query.id ?? [])[0]);
      if (!items.has(id)) return res.status(404).json({ error: "not found" });
      items.delete(id);
      res.status(204).empty();
    },
  },
});

runtime.log("listening on", srv.address);

const base = `http://127.0.0.1:${port}`;
const auth = { Authorization: `Bearer ${TOKEN}` };

// ── health (no auth needed) ───────────────────────────────────────────────────
const health = await net.http.get(`${base}/health`);
runtime.assert.equal(health.status, 200, "health status");
runtime.assert.equal(JSON.parse(health.body).ok, true, "health ok");
runtime.log("GET /health →", health.status);

// ── 401 on missing token ──────────────────────────────────────────────────────
const noAuth = await net.http.get(`${base}/items`);
runtime.assert.equal(noAuth.status, 401, "401 on missing token");
runtime.log("GET /items (no auth) →", noAuth.status);

// ── 401 on wrong token ───────────────────────────────────────────────────────
const badAuth = await net.http.request("GET", `${base}/items`, {
  headers: { Authorization: "Bearer wrong" },
});
runtime.assert.equal(badAuth.status, 401, "401 on wrong token");
runtime.log("GET /items (bad token) →", badAuth.status);

// ── POST /items (create) ──────────────────────────────────────────────────────
const created = await net.http.request("POST", `${base}/items`, {
  headers: { ...auth, "Content-Type": "application/json" },
  body: JSON.stringify({ name: "widget" }),
});
runtime.assert.equal(created.status, 201, "create status");
const createdItem = JSON.parse(created.body);
runtime.assert.equal(createdItem.name, "widget", "created name");
const itemId = createdItem.id as number;
runtime.log("POST /items →", created.status, "id:", itemId);

// ── GET /items (list) ─────────────────────────────────────────────────────────
const list = await net.http.request("GET", `${base}/items`, { headers: auth });
runtime.assert.equal(list.status, 200, "list status");
const listData = JSON.parse(list.body) as any[];
runtime.assert.equal(listData.length, 1, "list length");
runtime.log("GET /items →", list.status, "count:", listData.length);

// ── GET /items/by-id ──────────────────────────────────────────────────────────
const fetched = await net.http.request("GET", `${base}/items/by-id?id=${itemId}`, { headers: auth });
runtime.assert.equal(fetched.status, 200, "fetch status");
runtime.assert.equal(JSON.parse(fetched.body).name, "widget", "fetched name");
runtime.log("GET /items/by-id?id=" + itemId + " →", fetched.status);

// ── GET /items/by-id (404) ────────────────────────────────────────────────────
const missing = await net.http.request("GET", `${base}/items/by-id?id=9999`, { headers: auth });
runtime.assert.equal(missing.status, 404, "missing status");
runtime.log("GET /items/by-id?id=9999 →", missing.status);

// ── DELETE /items/by-id ───────────────────────────────────────────────────────
const deleted = await net.http.request("DELETE", `${base}/items/by-id?id=${itemId}`, { headers: auth });
runtime.assert.equal(deleted.status, 204, "delete status");
runtime.log("DELETE /items/by-id?id=" + itemId + " →", deleted.status);

// ── confirm gone ─────────────────────────────────────────────────────────────
const gone = await net.http.request("GET", `${base}/items/by-id?id=${itemId}`, { headers: auth });
runtime.assert.equal(gone.status, 404, "gone status");
runtime.log("GET /items/by-id after delete →", gone.status);

await srv.close();
runtime.log("PASS");
