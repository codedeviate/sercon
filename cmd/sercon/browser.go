package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"golang.org/x/net/publicsuffix"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// browserNamespace wires `net.browser.*` — a stateful HTTP session
// with an automatic cookie jar and replayed default headers, like a
// browser keeping a session across requests. The namespace exposes
// only `open`; the handle it returns carries the surface
// (setUserAgent / setHeader / get / post / cookies). Another
// stateful-handle binding, in the same shape as `db.sqlite`.
//
// Pure stdlib: `net/http` + `net/http/cookiejar` +
// `golang.org/x/net/publicsuffix` (the jar's public-suffix list, so
// cookies are scoped correctly across subdomains).
func browserNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		// browserOpen builds the handle map (which wires vm/loop-capturing
		// bindings), so it runs in extract — on the loop — and there is no
		// blocking work left; work just relays the finished handle without
		// touching it.
		"open": scriptengine.PromisifyAsync(vm, loop,
			func(goja.FunctionCall) (map[string]any, error) {
				return browserOpen(vm, loop)
			},
			func(_ context.Context, handle map[string]any) (map[string]any, error) {
				return handle, nil
			}),
	}
}

// browserSession is the mutable state behind a handle: an http.Client
// with a cookie jar, plus a header map that's replayed on every
// request. Guarded by a mutex because setHeader and get/post could in
// principle race if a script fired them without awaiting (the event
// loop serialises in practice, but the lock keeps the Go side
// honest).
type browserSession struct {
	mu      sync.Mutex
	client  *http.Client
	headers map[string]string
}

// browserPostArgs carries the on-loop-extracted post arguments to the work
// goroutine.
type browserPostArgs struct {
	url  string
	body string
}

func browserOpen(vm *goja.Runtime, loop *eventloop.EventLoop) (map[string]any, error) {
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return nil, fmt.Errorf("browser.open: cookie jar: %w", err)
	}
	sess := &browserSession{
		client:  &http.Client{Jar: jar},
		headers: map[string]string{},
	}

	return map[string]any{
		"setUserAgent": func(ua string) { sess.setHeader("User-Agent", ua) },
		"setHeader":    func(name, value string) { sess.setHeader(name, value) },
		"get": scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (string, error) {
				return call.Argument(0).String(), nil
			},
			func(ctx context.Context, url string) (map[string]any, error) {
				return sess.do(ctx, http.MethodGet, url, "")
			}).Func,
		"post": scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (browserPostArgs, error) {
				a := browserPostArgs{url: call.Argument(0).String()}
				if len(call.Arguments) > 1 {
					a.body = call.Argument(1).String()
				}
				return a, nil
			},
			func(ctx context.Context, a browserPostArgs) (map[string]any, error) {
				return sess.do(ctx, http.MethodPost, a.url, a.body)
			}).Func,
		"cookies": scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (string, error) {
				return call.Argument(0).String(), nil
			},
			func(_ context.Context, url string) ([]map[string]any, error) {
				return sess.cookiesFor(url)
			}).Func,
	}, nil
}

func (s *browserSession) setHeader(name, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.headers[name] = value
}

// do performs a request with the session's accumulated headers and
// shared cookie jar. Result shape mirrors net.http.request:
// `{ status, ok, headers, body, url }`. The cookie jar is updated
// automatically by the http.Client, so a login POST followed by a
// GET replays the session cookie without the script touching it.
func (s *browserSession) do(ctx context.Context, method, url, body string) (map[string]any, error) {
	if url == "" {
		return nil, errors.New("browser: url required")
	}
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("browser.%s: %w", strings.ToLower(method), err)
	}
	s.mu.Lock()
	for k, v := range s.headers {
		req.Header.Set(k, v)
	}
	client := s.client
	s.mu.Unlock()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("browser.%s: %w", strings.ToLower(method), err)
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := readAllCapped(resp.Body, DefaultMaxHTTPBodyBytes, "browser response")
	if err != nil {
		return nil, fmt.Errorf("browser.%s: read body: %w", strings.ToLower(method), err)
	}
	respHeaders := make(map[string]string, len(resp.Header))
	for k, v := range resp.Header {
		if len(v) > 0 {
			respHeaders[strings.ToLower(k)] = v[len(v)-1]
		}
	}
	finalURL := url
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	return map[string]any{
		"status":  resp.StatusCode,
		"ok":      resp.StatusCode >= 200 && resp.StatusCode < 400,
		"headers": respHeaders,
		"body":    string(respBody),
		"url":     finalURL,
	}, nil
}

// cookiesFor returns the jar's cookies for a given URL as
// `{ name, value }` objects — lets a script inspect the session
// state (e.g. confirm a login set the expected cookie).
func (s *browserSession) cookiesFor(rawURL string) ([]map[string]any, error) {
	if rawURL == "" {
		return nil, errors.New("browser.cookies: url required")
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("browser.cookies: %w", err)
	}
	s.mu.Lock()
	jar := s.client.Jar
	s.mu.Unlock()
	out := []map[string]any{}
	for _, c := range jar.Cookies(req.URL) {
		out = append(out, map[string]any{"name": c.Name, "value": c.Value})
	}
	return out, nil
}
