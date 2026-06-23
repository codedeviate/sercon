// Demonstrates web.html — lenient HTML parsing with CSS + XPath. The offline
// block always runs (parses an inline tag-soup string). The live block fetches
// a real page only when the network is reachable; any network failure self-skips.

// True for failure signatures that mean "the network/endpoint is unusable
// here" — distinct from a real binding bug, which is re-thrown.
function netSkip(e: unknown): boolean {
  return /deadline|time?out|timed out|connection refused|no such host|dial |i\/o timeout|tls|eof|reset by peer|network is unreachable|unexpected status|HTTP \d/i
    .test(String(e));
}

// --- offline: always runs ---
const doc = web.html.parse(`<ul><li class="p"><a href="/a">Alpha<li class="p"><a href="/b">Beta</ul><h1>Title</h1>`);
const links = doc.findAll("li.p a").map((a: any) => a.attr("href"));
runtime.assert.equal(links.join(","), "/a,/b", "CSS findAll + attr");
runtime.assert.equal(doc.find("h1").text(), "Title", "CSS find + text");
runtime.assert.equal(doc.xpath("//a/@href").text(), "/a", "XPath attribute value");
runtime.log("web.html (offline) OK: links", links.join(", "));

// --- live: runs only when reachable ---
try {
  const live = await web.html.load("https://example.com", { timeout: "5s" });
  const h1 = live.find("h1");
  runtime.assert.ok(h1 !== null, "example.com has an <h1>");
  runtime.log("web.html (live) OK: <h1> =", h1.text());
} catch (e) {
  if (!netSkip(e)) throw e;
  runtime.log("web.html (live) skipped — no network:", String(e));
}
