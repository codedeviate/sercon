// Demonstrates api.net.http.request — the full-featured HTTP client beyond
// api.net.http.get/post. Headers, body, timeout, retry, basic auth, redirect
// control. Hits httpbin.org so it's in the network demo set, not CI.

api.runtime.log("=== GET with custom headers ===");
const r1 = await api.net.http.request("GET", "https://httpbin.org/headers", {
  headers: { "X-Sercon": "demo", "Accept": "application/json" },
});
api.runtime.log("status:", r1.status, "ok:", r1.ok);
api.runtime.log("server saw X-Sercon:", JSON.parse(r1.body).headers["X-Sercon"]);

api.runtime.log("");
api.runtime.log("=== POST JSON body ===");
const r2 = await api.net.http.request("POST", "https://httpbin.org/post", {
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ hello: "world" }),
});
api.runtime.log("echoed json:", JSON.parse(r2.body).json.hello);

api.runtime.log("");
api.runtime.log("=== basic auth ===");
const r3 = await api.net.http.request("GET", "https://httpbin.org/basic-auth/user/pass", {
  username: "user",
  password: "pass",
});
api.runtime.log("auth status:", r3.status, "(200 = authenticated)");

api.runtime.log("");
api.runtime.log("=== 4xx is a normal response, not a throw ===");
const r4 = await api.net.http.request("GET", "https://httpbin.org/status/404");
api.runtime.log("status:", r4.status, "ok:", r4.ok);

api.runtime.log("");
api.runtime.log("=== redirect control ===");
const followed = await api.net.http.request("GET", "https://httpbin.org/redirect/1");
api.runtime.log("with follow (default): final status", followed.status);
const notFollowed = await api.net.http.request("GET", "https://httpbin.org/redirect/1", { follow: false });
api.runtime.log("follow:false: sees the", notFollowed.status, "redirect itself");

api.runtime.log("");
api.runtime.log("=== retry rides out transient 5xx (retry: 2) ===");
const r5 = await api.net.http.request("GET", "https://httpbin.org/status/200", { retry: 2 });
api.runtime.log("status:", r5.status);
