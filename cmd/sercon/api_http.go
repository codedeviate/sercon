package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// httpRequestCall implements `api.net.http.request(method, url, opts?)` —
// the full-featured HTTP client that goes beyond the two-line
// `api.net.http.get` / `api.net.http.post`. It's pure `net/http`; the work is
// shaping the option surface (headers, body, timeout, retry, basic
// auth, redirect control) into a JS-friendly object and a single
// result shape.
//
// Result: `{ status, ok, headers, body, url }` where `headers` is a
// lower-cased name → value map (last value wins on repeats), `ok` is
// `status` in [200,400), and `url` is the final URL after redirects.
//
// HTTP 4xx / 5xx do NOT throw — they're normal responses surfaced via
// `status` / `ok`. Transport errors (DNS, connection refused, TLS) and
// context deadline throw. Retries (opts.retry) re-attempt only on
// transport errors and 5xx — never on 4xx, which are deterministic.
func httpRequestCall(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	method := strings.ToUpper(strings.TrimSpace(call.Argument(0).String()))
	if method == "" {
		return nil, errors.New("http.request: method required")
	}
	url := call.Argument(1).String()
	if url == "" {
		return nil, errors.New("http.request: url required")
	}
	opts := thirdArgAsMap(call)

	timeout := optMillis(opts, "timeout", 30*time.Second)
	retries := optInt(opts, "retry", 0)
	if retries < 0 {
		retries = 0
	}
	body := optString(opts, "body", "")
	headers := optStringMap(opts, "headers")
	follow := optBool(opts, "follow", true)
	authUser := optString(opts, "username", "")
	authPass := optString(opts, "password", "")

	client := &http.Client{
		Timeout: timeout,
	}
	if !follow {
		// Returning ErrUseLastResponse stops the client after the first
		// response without following the Location header — the script
		// sees the 3xx itself.
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	var lastErr error
	// attempts = 1 initial + `retries` re-tries.
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			// Linear backoff capped at a second — enough to ride out a
			// flapping upstream without stalling the event loop for long.
			delay := time.Duration(attempt) * 200 * time.Millisecond
			if delay > time.Second {
				delay = time.Second
			}
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("http.request: %w", ctx.Err())
			case <-time.After(delay):
			}
		}

		result, retryable, err := httpRequestOnce(ctx, client, method, url, body, headers, authUser, authPass)
		if err != nil {
			lastErr = err
			if retryable && attempt < retries {
				continue
			}
			return nil, fmt.Errorf("http.request: %w", err)
		}
		// A 5xx is retryable; 4xx and 2xx/3xx are final.
		if status, _ := result["status"].(int); status >= 500 && attempt < retries {
			lastErr = fmt.Errorf("server returned %d", status)
			continue
		}
		return result, nil
	}
	return nil, fmt.Errorf("http.request: exhausted %d retries: %w", retries, lastErr)
}

// httpRequestOnce performs a single attempt. The bool return is
// "retryable" — true for transport errors (worth retrying), used by
// the caller's retry loop.
func httpRequestOnce(ctx context.Context, client *http.Client, method, url, body string, headers map[string]string, user, pass string) (map[string]any, bool, error) {
	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		// A malformed method/URL is a programmer error, not retryable.
		return nil, false, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Transport-level failures are the retryable class.
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, fmt.Errorf("read body: %w", err)
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
	}, false, nil
}
