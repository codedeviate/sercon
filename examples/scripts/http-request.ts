// Demonstrates net.http.request — the full-featured HTTP client beyond
// net.http.get/post. Headers, body, timeout, retry, basic auth, redirect
// control. Hits httpbin.org so it's in the network demo set, not CI.
//
// Network-tolerant: if httpbin.org is unreachable (transport timeout, DNS
// failure, TLS error) or a proxy returns an HTML error page instead of JSON,
// the script logs a skip message and exits 0 rather than failing the run.
// Genuine binding errors are re-thrown.

// True for failure signatures that mean "the network/endpoint is unusable
// here" — distinct from a real bug, which is re-thrown.
function netSkip(e: unknown): boolean {
  return /deadline|time?out|timed out|connection refused|no such host|dial |i\/o timeout|tls|invalid character '<'|unexpected end of|eof|reset by peer|network is unreachable/i
    .test(String(e));
}

try {
  runtime.log("=== GET with custom headers ===");
  const r1 = await net.http.request("GET", "https://httpbin.org/headers", {
    headers: { "X-Sercon": "demo", "Accept": "application/json" },
  });
  runtime.log("status:", r1.status, "ok:", r1.ok);
  runtime.log("server saw X-Sercon:", JSON.parse(r1.body).headers["X-Sercon"]);

  runtime.log("");
  runtime.log("=== POST JSON body ===");
  const r2 = await net.http.request("POST", "https://httpbin.org/post", {
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ hello: "world" }),
  });
  runtime.log("echoed json:", JSON.parse(r2.body).json.hello);

  runtime.log("");
  runtime.log("=== basic auth ===");
  const r3 = await net.http.request("GET", "https://httpbin.org/basic-auth/user/pass", {
    username: "user",
    password: "pass",
  });
  runtime.log("auth status:", r3.status, "(200 = authenticated)");

  runtime.log("");
  runtime.log("=== 4xx is a normal response, not a throw ===");
  const r4 = await net.http.request("GET", "https://httpbin.org/status/404");
  runtime.log("status:", r4.status, "ok:", r4.ok);

  runtime.log("");
  runtime.log("=== redirect control ===");
  const followed = await net.http.request("GET", "https://httpbin.org/redirect/1");
  runtime.log("with follow (default): final status", followed.status);
  const notFollowed = await net.http.request("GET", "https://httpbin.org/redirect/1", { follow: false });
  runtime.log("follow:false: sees the", notFollowed.status, "redirect itself");

  runtime.log("");
  runtime.log("=== retry rides out transient 5xx (retry: 2) ===");
  const r5 = await net.http.request("GET", "https://httpbin.org/status/200", { retry: 2 });
  runtime.log("status:", r5.status);
} catch (e) {
  if (!netSkip(e)) throw e;
  runtime.log("httpbin.org unreachable — skipping http-request demo. (" + String(e).slice(0, 120) + ")");
}
