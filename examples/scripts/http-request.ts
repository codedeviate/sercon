// Demonstrates api.http.request — the full-featured HTTP client beyond
// api.http.get/post. Headers, body, timeout, retry, basic auth, redirect
// control. Hits httpbin.org so it's in the network demo set, not CI.

api.log("=== GET with custom headers ===");
const r1 = await api.http.request("GET", "https://httpbin.org/headers", {
  headers: { "X-Sercon": "demo", "Accept": "application/json" },
});
api.log("status:", r1.status, "ok:", r1.ok);
api.log("server saw X-Sercon:", JSON.parse(r1.body).headers["X-Sercon"]);

api.log("");
api.log("=== POST JSON body ===");
const r2 = await api.http.request("POST", "https://httpbin.org/post", {
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ hello: "world" }),
});
api.log("echoed json:", JSON.parse(r2.body).json.hello);

api.log("");
api.log("=== basic auth ===");
const r3 = await api.http.request("GET", "https://httpbin.org/basic-auth/user/pass", {
  username: "user",
  password: "pass",
});
api.log("auth status:", r3.status, "(200 = authenticated)");

api.log("");
api.log("=== 4xx is a normal response, not a throw ===");
const r4 = await api.http.request("GET", "https://httpbin.org/status/404");
api.log("status:", r4.status, "ok:", r4.ok);

api.log("");
api.log("=== redirect control ===");
const followed = await api.http.request("GET", "https://httpbin.org/redirect/1");
api.log("with follow (default): final status", followed.status);
const notFollowed = await api.http.request("GET", "https://httpbin.org/redirect/1", { follow: false });
api.log("follow:false: sees the", notFollowed.status, "redirect itself");

api.log("");
api.log("=== retry rides out transient 5xx (retry: 2) ===");
const r5 = await api.http.request("GET", "https://httpbin.org/status/200", { retry: 2 });
api.log("status:", r5.status);
