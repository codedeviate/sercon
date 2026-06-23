package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

const urlsetXML = `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>https://e.example/a</loc><lastmod>2024-01-01</lastmod><priority>0.8</priority></url>
  <url><loc>https://e.example/b</loc></url>
</urlset>`

func TestSitemap_ParseUrlsetAndIndex(t *testing.T) {
	sm, err := parseSitemap([]byte(urlsetXML))
	if err != nil {
		t.Fatalf("parseSitemap urlset: %v", err)
	}
	if sm["type"] != "urlset" {
		t.Fatalf("type = %v, want urlset", sm["type"])
	}
	urls := sm["urls"].([]map[string]any)
	if len(urls) != 2 || urls[0]["loc"] != "https://e.example/a" || urls[0]["priority"] != 0.8 {
		t.Fatalf("urls wrong: %v", urls)
	}

	idx := `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	  <sitemap><loc>https://e.example/sm1.xml</loc></sitemap></sitemapindex>`
	smi, err := parseSitemap([]byte(idx))
	if err != nil {
		t.Fatalf("parseSitemap index: %v", err)
	}
	if smi["type"] != "sitemapindex" {
		t.Fatalf("type = %v, want sitemapindex", smi["type"])
	}
	if got := smi["sitemaps"].([]string); len(got) != 1 || got[0] != "https://e.example/sm1.xml" {
		t.Fatalf("sitemaps wrong: %v", got)
	}

	if _, err := parseSitemap([]byte(`<html></html>`)); err == nil {
		t.Fatalf("expected error on non-sitemap XML")
	}
}

func TestSitemap_GzipAndExpand(t *testing.T) {
	// child served gzipped; index points to it; expand must merge + gunzip.
	var childURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/child.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		var b bytes.Buffer
		gz := gzip.NewWriter(&b)
		_, _ = gz.Write([]byte(urlsetXML))
		_ = gz.Close()
		_, _ = w.Write(b.Bytes())
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	childURL = srv.URL + "/child.xml.gz"

	idx := `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	  <sitemap><loc>` + childURL + `</loc></sitemap></sitemapindex>`
	sm, err := loadSitemap(context.Background(), []byte(idx), nil, true)
	if err != nil {
		t.Fatalf("loadSitemap expand: %v", err)
	}
	urls := sm["urls"].([]map[string]any)
	if len(urls) != 2 {
		t.Fatalf("expanded urls = %d, want 2", len(urls))
	}
	if errs := sm["errors"].([]map[string]any); len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
}
