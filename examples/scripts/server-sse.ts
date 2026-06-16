// Demonstrates res.sse — a one-way Server-Sent Events stream.
// The /events route sends three named events then closes the stream
// server-side. The self-test fetches the endpoint with a buffered HTTP
// GET (which returns once the server closes the connection) and asserts
// the accumulated text/event-stream frames.

const port = 38083;

const srv = await server.http.listen({
  port,
  routes: {
    "GET /events": async (req: any, res: any) => {
      const stream = res.sse({ retry: 2000 });
      await stream.send("warming up");
      await stream.send({ event: "tick", data: { n: 1 }, id: "1" });
      await stream.send({ event: "tick", data: { n: 2 }, id: "2" });
      await stream.close();
      return stream.closed;
    },
  },
});

runtime.log("listening on", srv.address);

// Use net.http.request (not net.http.get) — it exposes response headers.
// The GET returns once the server closes the stream after its three sends.
const r = await net.http.request("GET", `http://127.0.0.1:${port}/events`);
runtime.assert.equal(r.status, 200, "status 200");
runtime.assert.ok(
  (r.headers["content-type"] || "").includes("text/event-stream"),
  "content-type is text/event-stream",
);
runtime.assert.ok(r.body.includes("retry: 2000"), "retry hint present");
runtime.assert.ok(r.body.includes("data: warming up"), "string event present");
runtime.assert.ok(r.body.includes("event: tick"), "named event present");
runtime.assert.ok(r.body.includes(`data: {"n":1}`), "json data encoded");
runtime.assert.ok(r.body.includes("id: 2"), "event id present");
runtime.log("received stream:\n" + r.body);

await srv.close();
runtime.log("closed");
