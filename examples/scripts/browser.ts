// Demonstrates net.browser.* — a stateful HTTP session with an
// automatic cookie jar and replayed default headers. Hits httpbin.org
// so it's in the network demo set, not CI.
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
  const b = await net.browser.open();

  // Headers set once are replayed on every request.
  b.setUserAgent("sercon-browser-demo/1.0");
  b.setHeader("Accept", "application/json");

  runtime.log("=== headers replayed ===");
  const hdrs = await b.get("https://httpbin.org/headers");
  const seen = JSON.parse(hdrs.body).headers;
  runtime.log("server saw User-Agent:", seen["User-Agent"]);

  runtime.log("");
  runtime.log("=== cookie jar persists across requests ===");
  // httpbin's /cookies/set redirects and sets a cookie; the jar keeps it.
  await b.get("https://httpbin.org/cookies/set/sercon/demo123");
  const after = await b.get("https://httpbin.org/cookies");
  runtime.log("cookies on server:", JSON.parse(after.body).cookies.sercon);

  runtime.log("");
  runtime.log("=== inspect the jar locally ===");
  const jar = await b.cookies("https://httpbin.org/");
  runtime.log("local jar:", jar.map((c) => `${c.name}=${c.value}`).join(", "));
} catch (e) {
  if (!netSkip(e)) throw e;
  runtime.log("httpbin.org unreachable — skipping browser demo. (" + String(e).slice(0, 120) + ")");
}
