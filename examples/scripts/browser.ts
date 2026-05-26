// Demonstrates api.browser.* — a stateful HTTP session with an
// automatic cookie jar and replayed default headers. Hits httpbin.org
// so it's in the network demo set, not CI.

const b = await api.browser.open();

// Headers set once are replayed on every request.
b.setUserAgent("sercon-browser-demo/1.0");
b.setHeader("Accept", "application/json");

api.log("=== headers replayed ===");
const hdrs = await b.get("https://httpbin.org/headers");
const seen = JSON.parse(hdrs.body).headers;
api.log("server saw User-Agent:", seen["User-Agent"]);

api.log("");
api.log("=== cookie jar persists across requests ===");
// httpbin's /cookies/set redirects and sets a cookie; the jar keeps it.
await b.get("https://httpbin.org/cookies/set/sercon/demo123");
const after = await b.get("https://httpbin.org/cookies");
api.log("cookies on server:", JSON.parse(after.body).cookies.sercon);

api.log("");
api.log("=== inspect the jar locally ===");
const jar = await b.cookies("https://httpbin.org/");
api.log("local jar:", jar.map((c) => `${c.name}=${c.value}`).join(", "));
