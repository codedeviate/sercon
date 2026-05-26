// Demonstrates api.exec.http — recon-with-curl-fallback HTTP client.
// Hits real public endpoints so it's part of the network-dependent demo
// set (excluded from CI for the same reason as net-probe.ts and
// email-auth.ts).

api.log("=== auto-selected backend (recon preferred) ===");
const r1 = await api.exec.http("GET", "https://httpbin.org/get");
api.log("status:", r1.status, "backend:", r1.backend, "in", r1.durationMs, "ms");
api.log("content-type:", r1.headers["content-type"]);

api.log("");
api.log("=== forced backend = curl ===");
const r2 = await api.exec.http("GET", "https://httpbin.org/get", {
  backend: "curl",
});
api.log("status:", r2.status, "backend:", r2.backend);

api.log("");
api.log("=== POST with body + custom headers ===");
const r3 = await api.exec.http("POST", "https://httpbin.org/post", {
  headers: { "X-Sercon": "demo", "Content-Type": "application/json" },
  body: JSON.stringify({ hello: "world" }),
});
api.log("status:", r3.status);
const echoed = JSON.parse(r3.body);
api.log("echoed body:", echoed.data);
api.log("echoed header:", echoed.headers["X-Sercon"]);

api.log("");
api.log("=== 4xx does not throw — surfaces as status ===");
const r4 = await api.exec.http("GET", "https://httpbin.org/status/404");
api.log("status:", r4.status, "(no throw)");

api.log("");
api.log("=== transport error throws ===");
try {
  await api.exec.http("GET", "http://127.0.0.1:1/", { timeout: 1500 });
} catch (e) {
  api.log("caught:", String(e).slice(0, 80) + "…");
}
