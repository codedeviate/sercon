// Demonstrates server.http.listen — minimal HTTP server with routing
// and middleware. Picks a high random port; self-tests; closes.

const port = 38080;

const logger = async (req: any, res: any, next: any) => {
  const start = runtime.time.nowMs();
  await next();
  runtime.log(req.method, req.path, "→", runtime.time.nowMs() - start, "ms");
};

const srv = await server.http.listen({
  port,
  use: [logger],
  // onError renders a custom response when a handler throws/rejects,
  // instead of the stock "500 Internal Server Error".
  onError: (err: any, req: any, res: any) =>
    res.status(500).json({ error: String(err?.message ?? err), path: req.path }),
  routes: {
    "GET /":              (req: any, res: any) => res.text("hello, world"),
    "GET /json":          (req: any, res: any) => res.json({path: req.path, query: req.query}),
    "POST /echo":         (req: any, res: any) => res.status(201).json({echoed: req.body}),
    "GET /boom":          () => { throw new Error("boom"); },
  },
});

runtime.log("listening on", srv.address);

// Self-test: hit each route once.
const r1 = await net.http.get(`http://127.0.0.1:${port}/`);
runtime.assert.equal(r1.status, 200, "GET /");
runtime.assert.equal(r1.body, "hello, world", "body /");

const r2 = await net.http.get(`http://127.0.0.1:${port}/json?a=1&a=2&b=3`);
const data = JSON.parse(r2.body);
runtime.assert.ok(data.query.a.length === 2, "query.a length");
runtime.assert.equal(data.query.b[0], "3", "query.b");

// A throwing route is caught by onError → custom JSON 500.
const r3 = await net.http.get(`http://127.0.0.1:${port}/boom`);
runtime.assert.equal(r3.status, 500, "GET /boom status");
runtime.assert.equal(JSON.parse(r3.body).error, "boom", "onError surfaced the message");
runtime.log("onError handled /boom →", r3.body);

await srv.close();
runtime.log("closed");
