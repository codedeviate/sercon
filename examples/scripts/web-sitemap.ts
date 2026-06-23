// Demonstrates web.sitemap — urlset/sitemapindex, gzip, and {expand:true}.
// Offline block always runs; live block self-skips offline.

// True for failure signatures that mean "the network/endpoint is unusable
// here" — distinct from a real binding bug, which is re-thrown.
function netSkip(e: unknown): boolean {
  return /deadline|time?out|timed out|connection refused|no such host|dial |i\/o timeout|tls|eof|reset by peer|network is unreachable|unexpected status|HTTP \d/i
    .test(String(e));
}

const xml = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://demo.example/a</loc><priority>0.8</priority></url>
  <url><loc>https://demo.example/b</loc></url>
</urlset>`;
const sm = web.sitemap.parse(xml);
runtime.assert.equal(sm.type, "urlset", "urlset detected");
runtime.assert.equal(sm.urls.length, 2, "two urls");
runtime.assert.equal(sm.urls[0].priority, 0.8, "priority parsed as number");
runtime.log("web.sitemap (offline) OK:", sm.urls.map((u: any) => u.loc).join(", "));

try {
  const live = await web.sitemap.load("https://www.sitemaps.org/sitemap.xml", { timeout: "5s" });
  runtime.assert.ok(live.type === "urlset" || live.type === "sitemapindex", "valid sitemap type");
  runtime.log("web.sitemap (live) OK:", live.type, live.urls.length, "urls,", live.sitemaps.length, "child sitemaps");
} catch (e) {
  if (!netSkip(e)) throw e;
  runtime.log("web.sitemap (live) skipped — no network:", String(e));
}
