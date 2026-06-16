// Demonstrates net.load.http — an authorized HTTP load / resilience self-test.
// Spins up a loopback server, drives it at modest concurrency, prints the
// latency/error report, and asserts a clean run. Loopback target needs no
// `confirm` (the guardrail only gates public hosts). Offline; in make demo.

const port = 38085;
const srv = await server.http.listen({
  port,
  routes: { "GET /": (req: any, res: any) => res.json({ ok: true }) },
});

const report = await net.load.http({
  url: `http://127.0.0.1:${port}/`,
  requests: 200,
  concurrency: 10,
});

runtime.log("sent:", report.sent, "completed:", report.completed, "rps:", report.rps);
runtime.log("latency p50/p95/max ms:", report.latency.p50, report.latency.p95, report.latency.max);
runtime.log("status:", JSON.stringify(report.statusCounts));

runtime.assert.equal(report.sent, 200, "sent all requests");
runtime.assert.equal(report.completed, 200, "all completed");
runtime.assert.equal(report.failed, 0, "no transport failures");
runtime.assert.equal(report.errorRate, 0, "zero error rate");
runtime.assert.ok(report.latency.p95 >= report.latency.p50, "p95 >= p50");

await srv.close();
runtime.log("load self-test PASS");
