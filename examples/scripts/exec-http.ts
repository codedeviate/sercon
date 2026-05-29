// Demonstrates services.exec.http — recon-with-curl-fallback HTTP client.
// Hits real public endpoints so it's part of the network-dependent demo
// set (excluded from CI for the same reason as net-probe.ts and
// email-auth.ts).

runtime.log("=== auto-selected backend (recon preferred) ===");
const r1 = await services.exec.http("GET", "https://httpbin.org/get");
runtime.log("status:", r1.status, "backend:", r1.backend, "in", r1.durationMs, "ms");
runtime.log("content-type:", r1.headers["content-type"]);

runtime.log("");
runtime.log("=== forced backend = curl ===");
const r2 = await services.exec.http("GET", "https://httpbin.org/get", {
  backend: "curl",
});
runtime.log("status:", r2.status, "backend:", r2.backend);

runtime.log("");
runtime.log("=== POST with body + custom headers ===");
const r3 = await services.exec.http("POST", "https://httpbin.org/post", {
  headers: { "X-Sercon": "demo", "Content-Type": "application/json" },
  body: JSON.stringify({ hello: "world" }),
});
runtime.log("status:", r3.status);
const echoed = JSON.parse(r3.body);
runtime.log("echoed body:", echoed.data);
runtime.log("echoed header:", echoed.headers["X-Sercon"]);

runtime.log("");
runtime.log("=== 4xx does not throw — surfaces as status ===");
const r4 = await services.exec.http("GET", "https://httpbin.org/status/404");
runtime.log("status:", r4.status, "(no throw)");

runtime.log("");
runtime.log("=== transport error throws ===");
try {
  await services.exec.http("GET", "http://127.0.0.1:1/", { timeout: 1500 });
} catch (e) {
  runtime.log("caught:", String(e).slice(0, 80) + "…");
}
