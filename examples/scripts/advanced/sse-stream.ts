// Advanced demo: a live-metrics stream over Server-Sent Events (res.sse).
//
// The GET /metrics route opens an SSE stream and pushes a JSON "tick" event
// on a timer, plus one named "alert" event mid-stream, then closes the
// stream server-side after a fixed number of ticks. A `stream.closed`
// handler stops the timer so nothing leaks if the client disconnects first.
//
// The self-test acts as the client: net.http.request does a buffered GET
// that returns once the server closes the stream, so the whole
// text/event-stream body is available to assert against. Offline + fixed
// port; runs in `make demo` (server demos are excluded from CI).

const port = 38084;
const TICKS = 5;

const srv = await server.http.listen({
  port,
  routes: {
    "GET /metrics": (req: any, res: any) => {
      // keepAlive is harmless here (the stream is short) but demonstrates the
      // option that defeats idle-proxy timeouts on long-lived streams.
      const stream = res.sse({ keepAlive: 1000, retry: 3000 });
      let n = 0;
      const timer = setInterval(async () => {
        n++;
        await stream.send({
          event: "tick",
          id: String(n),
          data: { n, cpu: 10 + n, mem: 100 + n * 2 },
        });
        if (n === Math.floor(TICKS / 2)) {
          await stream.send({ event: "alert", data: { level: "warn", msg: "halfway" } });
        }
        if (n >= TICKS) {
          clearInterval(timer);
          await stream.close();
        }
      }, 20);
      // Stop the producer if the client goes away before we finish.
      stream.closed.then(() => clearInterval(timer));
      return stream.closed;
    },
  },
});

runtime.log("listening on", srv.address);

const r = await net.http.request("GET", `http://127.0.0.1:${port}/metrics`);
runtime.assert.equal(r.status, 200, "status 200");
runtime.assert.ok(
  (r.headers["content-type"] || "").includes("text/event-stream"),
  "content-type is text/event-stream",
);
runtime.assert.ok(r.body.includes("retry: 3000"), "retry hint present");
runtime.assert.ok(r.body.includes("event: tick"), "tick events present");
runtime.assert.ok(r.body.includes("event: alert"), "named alert event present");
runtime.assert.ok(r.body.includes(`"cpu":11`), "first tick JSON payload present");
runtime.assert.ok(r.body.includes("id: " + TICKS), "last tick id present");

// Count how many tick frames arrived (one "event: tick" line each).
const ticks = (r.body.match(/event: tick/g) || []).length;
runtime.assert.equal(ticks, TICKS, `received ${TICKS} tick events`);
runtime.log(`received ${ticks} tick events + 1 alert; stream length ${r.body.length} bytes`);

await srv.close();
runtime.log("closed");
