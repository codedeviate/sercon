package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// defaultWebUserAgent is sent on web.*.load requests unless the caller
// supplies their own User-Agent (via opts.userAgent or opts.headers).
var defaultWebUserAgent = "sercon-web/" + scriptengine.Version

// fetchOpts is the parsed option surface shared by web.feed/sitemap/html load().
// It mirrors the subset of net.http.request options that make sense for a GET.
type fetchOpts struct {
	timeout   time.Duration
	headers   map[string]string
	follow    bool
	userAgent string
	user      string
	pass      string
}

// parseFetchOpts reads a JS opts object into fetchOpts, reusing the same option
// helpers net.http.request uses so the surfaces stay consistent.
func parseFetchOpts(opts map[string]any) fetchOpts {
	if opts == nil {
		return fetchOpts{timeout: 30 * time.Second, follow: true}
	}
	return fetchOpts{
		timeout:   optMillis(opts, "timeout", 30*time.Second),
		headers:   optStringMap(opts, "headers"),
		follow:    optBool(opts, "follow", true),
		userAgent: optString(opts, "userAgent", ""),
		user:      optString(opts, "username", ""),
		pass:      optString(opts, "password", ""),
	}
}

// webFetch performs a single GET and returns the raw body, the final URL after
// redirects, and the HTTP status. err is non-nil only for transport-level
// failures (DNS, refused, TLS, deadline) — a non-2xx status is NOT an error
// here (callers decide; sitemap-expand tolerates per-child non-2xx).
func webFetch(ctx context.Context, url string, fo fetchOpts) (body []byte, finalURL string, status int, err error) {
	client := &http.Client{Timeout: fo.timeout}
	if !fo.follow {
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, url, 0, err
	}
	for k, v := range fo.headers {
		req.Header.Set(k, v)
	}
	if fo.userAgent != "" {
		req.Header.Set("User-Agent", fo.userAgent)
	} else if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", defaultWebUserAgent)
	}
	if fo.user != "" || fo.pass != "" {
		req.SetBasicAuth(fo.user, fo.pass)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, url, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, url, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	finalURL = url
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return b, finalURL, resp.StatusCode, nil
}

// loadBytes fetches a URL and throws (returns an error) on a transport failure
// OR a non-2xx status — the contract for the top-level web.*.load helpers.
// optsMap may be nil.
func loadBytes(ctx context.Context, url string, optsMap map[string]any) ([]byte, string, error) {
	fo := parseFetchOpts(optsMap)
	body, finalURL, status, err := webFetch(ctx, url, fo)
	if err != nil {
		return nil, finalURL, fmt.Errorf("web: GET %s: %w", url, err)
	}
	if status < 200 || status >= 300 {
		return nil, finalURL, fmt.Errorf("web: GET %s: HTTP %d", finalURL, status)
	}
	return body, finalURL, nil
}
