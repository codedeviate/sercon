package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"golang.org/x/net/publicsuffix"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// maxSitemapChildren caps how many child sitemaps an expand will fetch, so an
// adversarial index can't trigger unbounded fan-out. Excess is skipped and
// noted in the result's errors[] (never silently dropped).
const maxSitemapChildren = 50

type smURLSet struct {
	XMLName xml.Name `xml:"urlset"`
	URLs    []struct {
		Loc        string `xml:"loc"`
		LastMod    string `xml:"lastmod"`
		ChangeFreq string `xml:"changefreq"`
		Priority   string `xml:"priority"`
	} `xml:"url"`
}

type smIndex struct {
	XMLName  xml.Name `xml:"sitemapindex"`
	Sitemaps []struct {
		Loc string `xml:"loc"`
	} `xml:"sitemap"`
}

// gunzipIfNeeded decompresses data when it carries the gzip magic header,
// otherwise returns it unchanged. Decompressed output is capped at
// DefaultMaxDecompressBytes — this path has no opts surface to override it —
// so a crafted small .xml.gz can't inflate to gigabytes (a "decompression
// bomb") before the sitemap parser ever sees it.
func gunzipIfNeeded(data []byte) ([]byte, error) {
	return gunzipIfNeededMax(data, DefaultMaxDecompressBytes)
}

// gunzipIfNeededMax is gunzipIfNeeded with an explicit cap, split out so
// tests can exercise the overflow path with a small bound instead of
// inflating real gigabytes to hit the default.
func gunzipIfNeededMax(data []byte, maxBytes int64) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer func() { _ = zr.Close() }()
	return readAllCapped(zr, maxBytes, "decompressed sitemap")
}

// urlEntry converts a parsed urlset entry to the result map (priority parsed as
// a number when present, omitted otherwise).
func urlEntry(loc, lastmod, changefreq, priority string) map[string]any {
	m := map[string]any{"loc": loc}
	if lastmod != "" {
		m["lastmod"] = lastmod
	}
	if changefreq != "" {
		m["changefreq"] = changefreq
	}
	if priority != "" {
		if p, err := strconv.ParseFloat(strings.TrimSpace(priority), 64); err == nil {
			m["priority"] = p
		}
	}
	return m
}

// parseSitemap parses a single (possibly gzipped) sitemap document. It returns
// the result map with type=="urlset" or "sitemapindex". A document that is
// neither shape returns an error.
func parseSitemap(data []byte) (map[string]any, error) {
	data, err := gunzipIfNeeded(data)
	if err != nil {
		return nil, fmt.Errorf("sitemap: gunzip: %w", err)
	}
	var us smURLSet
	if err := xml.Unmarshal(data, &us); err == nil && us.XMLName.Local == "urlset" {
		urls := make([]map[string]any, 0, len(us.URLs))
		for _, u := range us.URLs {
			urls = append(urls, urlEntry(u.Loc, u.LastMod, u.ChangeFreq, u.Priority))
		}
		return map[string]any{
			"type": "urlset", "urls": urls, "sitemaps": []string{}, "errors": []map[string]any{},
		}, nil
	}
	var ix smIndex
	if err := xml.Unmarshal(data, &ix); err == nil && ix.XMLName.Local == "sitemapindex" {
		locs := make([]string, 0, len(ix.Sitemaps))
		for _, s := range ix.Sitemaps {
			if s.Loc != "" {
				locs = append(locs, s.Loc)
			}
		}
		return map[string]any{
			"type": "sitemapindex", "urls": []map[string]any{}, "sitemaps": locs, "errors": []map[string]any{},
		}, nil
	}
	return nil, fmt.Errorf("sitemap: document is neither <urlset> nor <sitemapindex>")
}

// sameSite reports whether child is safe to fetch on behalf of a sitemapindex
// served from parent: same scheme and same registrable domain (eTLD+1). This
// constrains expansion of DATA-named targets (child <loc> URLs read from a
// remote, attacker-controllable document) — it does not apply to
// author-named fetch paths elsewhere in the codebase. eTLD+1 has no public
// suffix for bare IPs / "localhost", so those fall back to an exact-host
// compare, which correctly treats e.g. a cloud metadata IP as cross-site
// relative to a normal domain parent.
func sameSite(parent, child string) bool {
	pu, err := url.Parse(parent)
	if err != nil {
		return false
	}
	cu, err := url.Parse(child)
	if err != nil {
		return false
	}
	if pu.Scheme != cu.Scheme {
		return false
	}
	pd, err1 := publicsuffix.EffectiveTLDPlusOne(pu.Hostname())
	cd, err2 := publicsuffix.EffectiveTLDPlusOne(cu.Hostname())
	if err1 != nil || err2 != nil {
		return pu.Hostname() == cu.Hostname()
	}
	return pd == cd
}

// sitemapChildRedirectGuard returns an http.Client.CheckRedirect that
// re-applies sameSite(parentURL, hop) to every redirect hop encountered while
// fetching an expanded child sitemap. The initial sameSite check in
// loadSitemap only inspects the pre-redirect child <loc> — since that value
// is attacker-controllable data read from the parent document, a same-site
// child could 302 to a cross-site or internal target (e.g. a cloud metadata
// IP) and bypass the gate entirely, because http.Client follows redirects
// transparently. This closes that hole for the child-expand fetch only; it
// is never installed on author-named fetches (web.feed.load, web.html.load,
// or the sitemap's own top-level URL). Capped at 10 hops as a sanity
// backstop, matching net.load.http's redirectGuard in load.go.
func sitemapChildRedirectGuard(parentURL string) func(req *http.Request, via []*http.Request) error {
	return func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("web.sitemap.load: stopped after 10 redirects")
		}
		if !sameSite(parentURL, req.URL.String()) {
			return fmt.Errorf("web.sitemap.load: refusing to follow redirect to cross-site host %q", req.URL.Hostname())
		}
		return nil
	}
}

// loadSitemap parses data and, when expand is true and it's an index, fetches
// each child sitemap (bounded by maxSitemapChildren), gunzips, and merges their
// urls into the result. Per-child failures are captured in errors[], not thrown.
// Children that are themselves sitemapindex documents yield 0 urls — expansion is
// limited to one level (their child sitemaps are not recursed). Expansion is
// further restricted to children that are same-site (see sameSite) as
// parentURL — the sitemapindex's own source URL — since child <loc> values
// are attacker-controllable data from the fetched document, not
// maintainer-authored URLs, and following them cross-site is an SSRF vector.
func loadSitemap(ctx context.Context, parentURL string, data []byte, optsMap map[string]any, expand bool) (map[string]any, error) {
	sm, err := parseSitemap(data)
	if err != nil {
		return nil, err
	}
	if !expand || sm["type"] != "sitemapindex" {
		return sm, nil
	}
	children, _ := sm["sitemaps"].([]string)
	merged := make([]map[string]any, 0)
	errs := make([]map[string]any, 0)
	fo := parseFetchOpts(optsMap)
	// The child fetch is DATA-named (attacker-controllable <loc> values), so
	// unlike the top-level sitemap fetch it must re-validate sameSite on
	// every redirect hop, not just the pre-redirect URL — see
	// sitemapChildRedirectGuard.
	fo.redirectGuard = sitemapChildRedirectGuard(parentURL)
	for i, child := range children {
		if i >= maxSitemapChildren {
			errs = append(errs, map[string]any{"url": child, "error": fmt.Sprintf("skipped: exceeds child cap of %d", maxSitemapChildren)})
			continue
		}
		if !sameSite(parentURL, child) {
			errs = append(errs, map[string]any{"url": child, "error": "skipped: cross-site child (same-site only)"})
			continue
		}
		body, _, status, ferr := webFetch(ctx, child, fo)
		if ferr != nil {
			errs = append(errs, map[string]any{"url": child, "error": ferr.Error()})
			continue
		}
		if status < 200 || status >= 300 {
			errs = append(errs, map[string]any{"url": child, "error": fmt.Sprintf("status %d", status)})
			continue
		}
		csm, perr := parseSitemap(body)
		if perr != nil {
			errs = append(errs, map[string]any{"url": child, "error": perr.Error()})
			continue
		}
		if cu, ok := csm["urls"].([]map[string]any); ok {
			merged = append(merged, cu...)
		}
	}
	sm["urls"] = merged
	sm["errors"] = errs
	return sm, nil
}

// sitemapParseBinding implements web.sitemap.parse(source) — synchronous.
func sitemapParseBinding(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		out, err := parseSitemap([]byte(call.Argument(0).String()))
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return vm.ToValue(out)
	}
}

// sitemapLoadWork is the off-loop worker for web.sitemap.load(url, opts?).
func sitemapLoadWork(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	url := call.Argument(0).String()
	var optsMap map[string]any
	if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
		if m, ok := o.Export().(map[string]any); ok {
			optsMap = m
		}
	}
	expand := optBool(optsMap, "expand", false)
	body, _, err := loadBytes(ctx, url, optsMap)
	if err != nil {
		return nil, err
	}
	return loadSitemap(ctx, url, body, optsMap, expand)
}

// sitemapNamespace builds the web.sitemap sub-namespace.
func sitemapNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"parse": sitemapParseBinding(vm),
		"load":  scriptengine.PromisifyAsync(vm, loop, sitemapLoadWork),
	}
}
