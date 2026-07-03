package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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

	if urls[0]["lastmod"] != "2024-01-01" {
		t.Fatalf("lastmod not surfaced: %v", urls[0]["lastmod"])
	}
	if _, present := urls[1]["lastmod"]; present {
		t.Fatalf("absent lastmod should be omitted, got %v", urls[1]["lastmod"])
	}
	// expand:true on a urlset is a harmless no-op.
	noop, err := loadSitemap(context.Background(), "https://e.example/index.xml", []byte(urlsetXML), nil, true)
	if err != nil || noop["type"] != "urlset" || len(noop["urls"].([]map[string]any)) != 2 {
		t.Fatalf("urlset expand no-op wrong: %v err=%v", noop, err)
	}
}

func TestSameSite(t *testing.T) {
	cases := []struct {
		name          string
		parent, child string
		want          bool
	}{
		{"subdomain is same site", "https://a.example.com/s.xml", "https://b.example.com/c.xml", true},
		{"scheme mismatch is cross site", "https://example.com/s.xml", "http://example.com/c.xml", false},
		{"different domain is cross site", "https://example.com/s.xml", "https://evil.com/c.xml", false},
		{"metadata IP is cross site", "https://example.com/s.xml", "https://169.254.169.254/x.xml", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sameSite(tc.parent, tc.child); got != tc.want {
				t.Fatalf("sameSite(%q, %q) = %v, want %v", tc.parent, tc.child, got, tc.want)
			}
		})
	}
}

func TestSitemap_ExpandErrorsTolerated(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(urlsetXML))
	})
	mux.HandleFunc("/missing.xml", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	idx := `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	  <sitemap><loc>` + srv.URL + `/ok.xml</loc></sitemap>
	  <sitemap><loc>` + srv.URL + `/missing.xml</loc></sitemap></sitemapindex>`
	sm, err := loadSitemap(context.Background(), srv.URL+"/index.xml", []byte(idx), nil, true)
	if err != nil {
		t.Fatalf("loadSitemap: %v", err)
	}
	// the ok child contributed its 2 urls; the 404 child is captured, not fatal.
	if got := len(sm["urls"].([]map[string]any)); got != 2 {
		t.Fatalf("merged urls = %d, want 2", got)
	}
	errs := sm["errors"].([]map[string]any)
	if len(errs) != 1 || errs[0]["url"] != srv.URL+"/missing.xml" {
		t.Fatalf("expected 1 captured child error for missing.xml, got %v", errs)
	}
}

// The parent sitemapindex may live on one origin while listing children on
// another (attacker-controlled) origin — an SSRF vector. Expansion must only
// fetch same-site (same scheme + registrable domain) children; cross-site
// children are skipped into errors[] and never fetched.
func TestSitemap_ExpandRestrictsToSameSite(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ok.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(urlsetXML))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	const crossSiteChild = "http://169.254.169.254/x.xml"
	idx := `<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
	  <sitemap><loc>` + srv.URL + `/ok.xml</loc></sitemap>
	  <sitemap><loc>` + crossSiteChild + `</loc></sitemap></sitemapindex>`
	sm, err := loadSitemap(context.Background(), srv.URL+"/index.xml", []byte(idx), nil, true)
	if err != nil {
		t.Fatalf("loadSitemap: %v", err)
	}
	// only the same-site child's 2 urls are merged; the cross-site child is
	// never fetched.
	if got := len(sm["urls"].([]map[string]any)); got != 2 {
		t.Fatalf("merged urls = %d, want 2 (same-site child only)", got)
	}
	errs := sm["errors"].([]map[string]any)
	if len(errs) != 1 || errs[0]["url"] != crossSiteChild {
		t.Fatalf("expected 1 skipped cross-site child, got %v", errs)
	}
	if msg, _ := errs[0]["error"].(string); !strings.Contains(msg, "cross-site") {
		t.Fatalf("expected cross-site error message, got %v", errs[0]["error"])
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
	sm, err := loadSitemap(context.Background(), srv.URL+"/index.xml", []byte(idx), nil, true)
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

// A crafted small .xml.gz can inflate far beyond a reasonable sitemap size
// (a "decompression bomb"); gunzipIfNeeded must cap the output rather than
// io.ReadAll-ing it unbounded. gunzipIfNeededMax lets the test exercise the
// cap with a small bound instead of inflating gigabytes to hit the real
// DefaultMaxDecompressBytes default.
func TestSitemap_GunzipCap(t *testing.T) {
	payload := bytes.Repeat([]byte("<url><loc>https://e.example/x</loc></url>"), 50_000)
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	compressed := buf.Bytes()

	if _, err := gunzipIfNeededMax(compressed, 1024); err == nil {
		t.Fatal("expected error for gunzip output exceeding maxBytes")
	} else if !strings.Contains(err.Error(), "exceeds maxBytes limit") {
		t.Fatalf("unexpected error: %v", err)
	}

	// Exactly at the cap must succeed — off-by-one boundary correctness.
	out, err := gunzipIfNeededMax(compressed, int64(len(payload)))
	if err != nil {
		t.Fatalf("gunzip at exact cap: %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Fatal("gunzip at exact cap: mismatch")
	}

	// Non-gzip input passes through unchanged regardless of the cap.
	plain := []byte(urlsetXML)
	out2, err := gunzipIfNeededMax(plain, 1)
	if err != nil {
		t.Fatalf("passthrough: %v", err)
	}
	if !bytes.Equal(out2, plain) {
		t.Fatal("passthrough mismatch")
	}
}
