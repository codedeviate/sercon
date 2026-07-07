package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"sort"
	"strings"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
	"github.com/dop251/goja"
)

// httpRequestExtract + httpRequestCall implement
// `net.http.request(method, url, opts?)` —
// the full-featured HTTP client that goes beyond the two-line
// `net.http.get` / `net.http.post`. It's pure `net/http`; the work is
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
// httpRequestArgs is the plain-Go argument bundle handed from
// httpRequestExtract (on the event loop) to httpRequestCall (the work
// goroutine).
type httpRequestArgs struct {
	method      string
	url         string
	timeout     time.Duration
	retries     int
	headers     map[string]string
	maxBytes    int64
	bodyBytes   []byte
	contentType string
	follow      bool
	authUser    string
	authPass    string
}

// httpRequestExtract parses the (method, url, opts?) goja arguments into
// httpRequestArgs. It runs synchronously on the event loop — the only place
// the goja call may be touched; a validation error here rejects the Promise
// exactly like a work error.
func httpRequestExtract(call goja.FunctionCall) (httpRequestArgs, error) {
	var args httpRequestArgs
	args.method = strings.ToUpper(strings.TrimSpace(call.Argument(0).String()))
	if args.method == "" {
		return args, errors.New("http.request: method required")
	}
	args.url = call.Argument(1).String()
	if args.url == "" {
		return args, errors.New("http.request: url required")
	}
	opts := thirdArgAsMap(call)

	args.timeout = optMillis(opts, "timeout", 30*time.Second)
	args.retries = optInt(opts, "retry", 0)
	if args.retries < 0 {
		args.retries = 0
	}
	args.headers = optStringMap(opts, "headers")
	args.maxBytes = int64(optInt(opts, "maxBytes", DefaultMaxHTTPBodyBytes))
	if args.maxBytes <= 0 {
		args.maxBytes = DefaultMaxHTTPBodyBytes
	}

	// Body vs. multipart: mutually exclusive. body is string | Uint8Array |
	// ArrayBuffer sent byte-for-byte; multipart is assembled in Go and sets
	// the Content-Type header (overriding any caller content-type).
	bodyVal, hasBody := opts["body"]
	hasBody = hasBody && bodyVal != nil
	mpVal, hasMultipart := opts["multipart"]
	hasMultipart = hasMultipart && mpVal != nil
	if hasBody && hasMultipart {
		return args, errors.New("http.request: set either body or multipart, not both")
	}
	switch {
	case hasMultipart:
		parts, ok := mpVal.([]any)
		if !ok {
			return args, errors.New("http.request: multipart must be an array of parts")
		}
		b, ct, err := buildMultipartBody(parts)
		if err != nil {
			return args, fmt.Errorf("http.request: %w", err)
		}
		args.bodyBytes, args.contentType = b, ct
	case hasBody:
		b, err := bytesFromExported(bodyVal)
		if err != nil {
			return args, fmt.Errorf("http.request: body: %w", err)
		}
		args.bodyBytes = b
	}
	args.follow = optBool(opts, "follow", true)
	args.authUser = optString(opts, "username", "")
	args.authPass = optString(opts, "password", "")
	return args, nil
}

func httpRequestCall(ctx context.Context, args httpRequestArgs) (*scriptengine.Ordered, error) {
	client := &http.Client{
		Timeout: args.timeout,
	}
	if !args.follow {
		// Returning ErrUseLastResponse stops the client after the first
		// response without following the Location header — the script
		// sees the 3xx itself.
		client.CheckRedirect = func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}
	}

	var lastErr error
	// attempts = 1 initial + `retries` re-tries.
	for attempt := 0; attempt <= args.retries; attempt++ {
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

		result, status, retryable, err := httpRequestOnce(ctx, client, args.method, args.url, args.bodyBytes, args.contentType, args.headers, args.authUser, args.authPass, args.maxBytes)
		if err != nil {
			lastErr = err
			if retryable && attempt < args.retries {
				continue
			}
			return nil, fmt.Errorf("http.request: %w", err)
		}
		// A 5xx is retryable; 4xx and 2xx/3xx are final.
		if status >= 500 && attempt < args.retries {
			lastErr = fmt.Errorf("server returned %d", status)
			continue
		}
		return result, nil
	}
	return nil, fmt.Errorf("http.request: exhausted %d retries: %w", args.retries, lastErr)
}

// httpRequestOnce performs a single attempt. The bool return is
// "retryable" — true for transport errors (worth retrying), used by
// the caller's retry loop.
func httpRequestOnce(ctx context.Context, client *http.Client, method, url string, body []byte, contentType string, headers map[string]string, user, pass string, maxBytes int64) (*scriptengine.Ordered, int, bool, error) {
	var reqBody io.Reader
	if len(body) > 0 {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		// A malformed method/URL is a programmer error, not retryable.
		return nil, 0, false, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	// A generated multipart Content-Type (with its boundary) must win over any
	// caller-set content-type, so apply it after the caller headers.
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if user != "" || pass != "" {
		req.SetBasicAuth(user, pass)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Transport-level failures are the retryable class.
		return nil, 0, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := readAllCapped(resp.Body, maxBytes, "response body")
	if err != nil {
		// readAllCapped's own size-limit error is deterministic (not
		// retryable); anything else came from the underlying io.ReadAll and
		// is a transport-class failure worth retrying, same as before.
		if strings.Contains(err.Error(), "exceeds maxBytes limit") {
			return nil, 0, false, err
		}
		return nil, 0, true, fmt.Errorf("read body: %w", err)
	}

	// Build the headers sub-object with a stable, canonical key order:
	// lower-cased name → last value, names sorted alphabetically. Go's
	// http.Header is a map, so we sort to avoid run-to-run shuffle.
	names := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		names = append(names, k)
	}
	sort.Strings(names)
	respHeaders := scriptengine.NewOrdered()
	for _, k := range names {
		v := resp.Header[k]
		if len(v) > 0 {
			respHeaders.Set(strings.ToLower(k), v[len(v)-1])
		}
	}

	finalURL := url
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}

	result := scriptengine.NewOrdered().
		Set("status", resp.StatusCode).
		Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 400).
		Set("headers", respHeaders).
		Set("body", string(respBody)).
		Set("bodyBytes", respBody). // raw, undecoded bytes (→ Uint8Array); pair with text.charset.decode for non-UTF-8
		Set("url", finalURL)

	return result, resp.StatusCode, false, nil
}

// buildMultipartBody assembles a multipart/form-data body from the script's
// `multipart` option: an array of parts, each an object. A part with a
// non-empty `filename` is a file part — its `content` (string | Uint8Array |
// ArrayBuffer) is the file bytes and `type` sets the part's Content-Type
// (default application/octet-stream via CreateFormFile). Any other part is a
// text field carrying a string `value`. Returns the encoded body and the
// Content-Type header value, which carries the generated boundary.
func buildMultipartBody(parts []any) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for i, p := range parts {
		part, ok := p.(map[string]any)
		if !ok {
			return nil, "", fmt.Errorf("multipart[%d]: each part must be an object", i)
		}
		name, _ := part["name"].(string)
		if name == "" {
			return nil, "", fmt.Errorf("multipart[%d]: name is required", i)
		}
		if filename, _ := part["filename"].(string); filename != "" {
			content, ok := part["content"]
			if !ok || content == nil {
				return nil, "", fmt.Errorf("multipart[%d] (%s): file part requires content", i, name)
			}
			b, err := bytesFromExported(content)
			if err != nil {
				return nil, "", fmt.Errorf("multipart[%d] (%s): content: %w", i, name, err)
			}
			var fw io.Writer
			if ctype, _ := part["type"].(string); ctype != "" {
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, name, filename))
				h.Set("Content-Type", ctype)
				fw, err = w.CreatePart(h)
			} else {
				fw, err = w.CreateFormFile(name, filename)
			}
			if err != nil {
				return nil, "", fmt.Errorf("multipart[%d] (%s): %w", i, name, err)
			}
			if _, err := fw.Write(b); err != nil {
				return nil, "", fmt.Errorf("multipart[%d] (%s): %w", i, name, err)
			}
			continue
		}
		value, ok := part["value"].(string)
		if !ok {
			return nil, "", fmt.Errorf("multipart[%d] (%s): text field requires a string value", i, name)
		}
		if err := w.WriteField(name, value); err != nil {
			return nil, "", fmt.Errorf("multipart[%d] (%s): %w", i, name, err)
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

// bytesFromExported coerces an already-exported JS value (a value pulled from
// an opts map, so already run through goja's Export) into bytes: a string
// yields its UTF-8 bytes, a Uint8Array exports to []byte, and an ArrayBuffer
// to goja.ArrayBuffer. Mirrors exportBytes in compression.go, which takes a
// goja.Value rather than an already-exported any and so can't be reused here.
func bytesFromExported(v any) ([]byte, error) {
	switch e := v.(type) {
	case string:
		return []byte(e), nil
	case []byte:
		return e, nil
	case goja.ArrayBuffer:
		return e.Bytes(), nil
	default:
		return nil, fmt.Errorf("want string, Uint8Array, or ArrayBuffer, got %T", e)
	}
}
