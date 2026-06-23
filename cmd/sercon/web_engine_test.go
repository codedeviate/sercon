package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

func TestWeb_EngineSurface(t *testing.T) {
	opts := scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second}
	eng := scriptengine.New(opts)
	if err := registerSurface(eng); err != nil {
		t.Fatalf("registerSurface: %v", err)
	}

	script := `
		const doc = web.html.parse('<div><a href="/p">Hi</a><a href="/q">Yo</a></div>');
		const hrefs = doc.findAll("a").map(a => a.attr("href")).join(",");
		const f = web.feed.parse('<rss version="2.0"><channel><title>T</title><item><title>I</title><link>L</link></item></channel></rss>');
		const sm = web.sitemap.parse('<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://x/1</loc></url></urlset>');
		runtime.assert.equal(hrefs, "/p,/q", "html findAll/attr");
		runtime.assert.equal(f.feedType, "rss", "feed type");
		runtime.assert.equal(f.items[0].title, "I", "feed item");
		runtime.assert.equal(sm.urls[0].loc, "https://x/1", "sitemap url");
		runtime.assert.equal(typeof web.feed.load, "function", "feed.load present");
		runtime.assert.equal(typeof web.html.load, "function", "html.load present");
	`
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), script)
	if err != nil {
		t.Fatalf("web engine surface: %v", err)
	}
}
