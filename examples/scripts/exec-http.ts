// Demonstrates api.tools.exec.http — recon-with-curl-fallback HTTP client.
// Hits real public endpoints so it's part of the network-dependent demo
// set (excluded from CI for the same reason as net-probe.ts and
// email-auth.ts).

api.runtime.log("=== auto-selected backend (recon preferred) ===");
const r1 = await api.tools.exec.http("GET", "https://httpbin.org/get");
api.runtime.log("status:", r1.status, "backend:", r1.backend, "in", r1.durationMs, "ms");
api.runtime.log("content-type:", r1.headers["content-type"]);

api.runtime.log("");
api.runtime.log("=== forced backend = curl ===");
const r2 = await api.tools.exec.http("GET", "https://httpbin.org/get", {
  backend: "curl",
});
api.runtime.log("status:", r2.status, "backend:", r2.backend);

api.runtime.log("");
api.runtime.log("=== POST with body + custom headers ===");
const r3 = await api.tools.exec.http("POST", "https://httpbin.org/post", {
  headers: { "X-Sercon": "demo", "Content-Type": "application/json" },
  body: JSON.stringify({ hello: "world" }),
});
api.runtime.log("status:", r3.status);
const echoed = JSON.parse(r3.body);
api.runtime.log("echoed body:", echoed.data);
api.runtime.log("echoed header:", echoed.headers["X-Sercon"]);

api.runtime.log("");
api.runtime.log("=== 4xx does not throw — surfaces as status ===");
const r4 = await api.tools.exec.http("GET", "https://httpbin.org/status/404");
api.runtime.log("status:", r4.status, "(no throw)");

api.runtime.log("");
api.runtime.log("=== transport error throws ===");
try {
  await api.tools.exec.http("GET", "http://127.0.0.1:1/", { timeout: 1500 });
} catch (e) {
  api.runtime.log("caught:", String(e).slice(0, 80) + "…");
}
