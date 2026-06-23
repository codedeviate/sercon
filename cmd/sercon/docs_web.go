package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// webNodeTS is the inline TypeScript type for the chainable HTML Node handle
// returned by web.html.parse/load and the find/xpath family. Chainable methods
// return `unknown` to avoid a self-referential type in the generated d.ts
// (mirrors imageHandleTS in docs_image.go).
const webNodeTS = `{ ` +
	`find(selector: string): unknown; findAll(selector: string): unknown[]; ` +
	`xpath(expr: string): unknown; xpathAll(expr: string): unknown[]; ` +
	`text(): string; html(): string; innerHTML(): string; tag(): string; ` +
	`attr(name: string): string | null; attrs(): Record<string, string>; ` +
	`}`

const webFeedTS = `{ feedType: string; title: string; description: string; link: string; ` +
	`updated: string | null; items: Array<{ title: string; link: string; ` +
	`published: string | null; updated: string | null; content: string; summary: string; ` +
	`author: string; guid: string; categories: string[]; raw: Record<string, unknown> }> }`

const webSitemapTS = `{ type: "urlset" | "sitemapindex"; ` +
	`urls: Array<{ loc: string; lastmod?: string; changefreq?: string; priority?: number }>; ` +
	`sitemaps: string[]; errors: Array<{ url: string; error: string }> }`

const webFetchOptsTS = `{ timeout?: number | string; headers?: Record<string, string>; ` +
	`follow?: boolean; userAgent?: string; username?: string; password?: string }`

// webDocs documents the `web` global. The html node handle methods are carried
// as prose in MANUAL (the per-call handle is not reflected into the surface),
// matching docs_image.go.
func webDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"feed.parse": {
			Summary: "Parse RSS, Atom, or JSON-feed text into a normalized feed model. Format is auto-detected; feedType reports it. RSS/Atom field differences are unified (pubDate/updated, description/summary). Each item carries a `raw` escape hatch with format-specific extras (enclosures, namespaced elements like media:*/dc:*).",
			Params:  []scriptengine.Param{{Name: "source", Type: "string", Desc: "The feed document text (RSS/Atom XML or JSON Feed)."}},
			ReturnType: webFeedTS,
			Returns:    "The normalized feed object.",
			Errors:     "Throws on empty/malformed/undetectable feed input.",
			Example:    `const f = web.feed.parse(xml); f.items[0].title;`,
		},
		"feed.load": {
			Summary: "Fetch a URL and parse it as a feed (see feed.parse). Reuses the net.http option surface and sends a default User-Agent unless overridden. Throws on a non-2xx response.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "The feed URL to GET."},
				{Name: "opts", Type: webFetchOptsTS, Optional: true, Desc: "Fetch options: timeout (ms or duration string), headers, follow redirects, userAgent, basic-auth username/password."},
			},
			ReturnType: "Promise<" + webFeedTS + ">",
			Returns:    "A promise resolving to the normalized feed object.",
			Errors:     "Throws on transport failure or a non-2xx response, and on malformed feed content.",
			Example:    `const f = await web.feed.load("https://example.com/feed.xml");`,
		},
		"sitemap.parse": {
			Summary: "Parse a sitemap document (urlset or sitemapindex) into {type, urls, sitemaps, errors}. urls carry loc/lastmod/changefreq/priority; an index lists child sitemap URLs in `sitemaps`.",
			Params:  []scriptengine.Param{{Name: "source", Type: "string", Desc: "The sitemap XML text (decompressed)."}},
			ReturnType: webSitemapTS,
			Returns:    "The parsed sitemap object.",
			Errors:     "Throws when the document is neither <urlset> nor <sitemapindex>.",
			Example:    `const sm = web.sitemap.parse(xml); sm.urls.map(u => u.loc);`,
		},
		"sitemap.load": {
			Summary: "Fetch a sitemap URL and parse it. Transparently decompresses .xml.gz (gzip magic / Content-Encoding). For a sitemapindex, pass {expand:true} to fetch all child sitemaps (bounded; per-child errors collected in `errors[]`) and merge their urls into `urls`.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "The sitemap URL to GET (may be .xml.gz)."},
				{Name: "opts", Type: `{ expand?: boolean } & ` + webFetchOptsTS, Optional: true, Desc: "expand:true fetches & merges child sitemaps for an index. Plus the standard fetch options."},
			},
			ReturnType: "Promise<" + webSitemapTS + ">",
			Returns:    "A promise resolving to the sitemap object.",
			Errors:     "Throws on transport failure, non-2xx, or a non-sitemap document. Per-child expand failures are captured in errors[], not thrown.",
			Example:    `const sm = await web.sitemap.load("https://example.com/sitemap.xml", { expand: true });`,
		},
		"html.parse": {
			Summary: "Parse HTML leniently (real-world tag soup is accepted, never throws on bad markup) into a chainable Node. Query with CSS (find/findAll) or XPath (xpath/xpathAll); read with text/html/innerHTML/tag/attr/attrs. Sub-queries are scoped to the receiver node (use .// for relative XPath; // is document-wide).",
			Params:  []scriptengine.Param{{Name: "source", Type: "string", Desc: "The HTML document text."}},
			ReturnType: webNodeTS,
			Returns:    "A Node handle rooted at the document.",
			Errors:     "Does not throw on malformed markup; only on unreadable input.",
			Example:    `const doc = web.html.parse(html); doc.find("h1").text();`,
		},
		"html.load": {
			Summary: "Fetch a URL and parse the response as lenient HTML (see html.parse). Reuses the net.http option surface with a default User-Agent. Throws on a non-2xx response.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "The page URL to GET."},
				{Name: "opts", Type: webFetchOptsTS, Optional: true, Desc: "Fetch options: timeout (ms or duration string), headers, follow redirects, userAgent, basic-auth username/password."},
			},
			ReturnType: "Promise<" + webNodeTS + ">",
			Returns:    "A promise resolving to the document Node handle.",
			Errors:     "Throws on transport failure or a non-2xx response.",
			Example:    `const doc = await web.html.load("https://example.com"); doc.findAll("a").map(a => a.attr("href"));`,
		},
	}
}
