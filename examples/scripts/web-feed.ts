// Demonstrates web.feed — RSS/Atom/JSON feeds normalized to one model with a
// `.raw` escape hatch. Offline block always runs; live block self-skips offline.

// True for failure signatures that mean "the network/endpoint is unusable
// here" — distinct from a real binding bug, which is re-thrown.
function netSkip(e: unknown): boolean {
  return /deadline|time?out|timed out|connection refused|no such host|dial |i\/o timeout|tls|eof|reset by peer|network is unreachable|unexpected status|HTTP \d/i
    .test(String(e));
}

const rss = `<?xml version="1.0"?><rss version="2.0">
  <channel><title>Demo</title><link>https://demo.example</link>
    <item><title>First</title><link>https://demo.example/1</link>
      <pubDate>Mon, 02 Jan 2006 15:04:05 +0000</pubDate></item></channel></rss>`;
const f = web.feed.parse(rss);
runtime.assert.equal(f.feedType, "rss", "feed type detected");
runtime.assert.equal(f.items[0].title, "First", "normalized item title");
runtime.assert.ok(f.items[0].published !== null, "normalized published date");
runtime.log("web.feed (offline) OK:", f.title, "→", f.items.length, "item(s)");

try {
  const live = await web.feed.load("https://hnrss.org/frontpage", { timeout: "5s" });
  runtime.assert.ok(live.items.length >= 1, "live feed has items");
  runtime.assert.ok(!!live.items[0].title && !!live.items[0].link, "items have title+link");
  runtime.log("web.feed (live) OK:", live.feedType, live.items.length, "items");
} catch (e) {
  if (!netSkip(e)) throw e;
  runtime.log("web.feed (live) skipped — no network:", String(e));
}
