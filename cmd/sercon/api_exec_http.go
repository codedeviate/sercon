package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// execHTTP runs an HTTP request by shelling out to `recon` (preferred) or
// `curl` (fallback). Both tools share enough of curl's CLI surface that
// a single argv builder targets both — with one wrinkle: recon's `-i`
// output is verbose-debug style (`< Header: value`), incompatible with
// curl's wire-format `-i`. We sidestep that by writing the response
// headers to a temp file via `-D <path>` (both backends agree there) and
// taking the body off stdout.
//
// Contract:
//
//   - `method` is the HTTP verb (GET, POST, PUT, DELETE, PATCH, HEAD).
//     Lower-case input is uppercased before forwarding.
//   - `url` is the target. Both backends require a fully qualified URL.
//   - `opts.headers` is a map of header name → value. One
//     `-H "Name: Value"` per entry.
//   - `opts.body` is the request body. When present it is written to a
//     temp file and passed via `--data-binary @<path>` so CR/LF stay
//     intact regardless of backend.
//   - `opts.timeout` is in milliseconds and defaults to 30 000 ms.
//   - `opts.follow` toggles `-L` so 3xx redirects are followed.
//   - `opts.insecure` toggles `-k` so TLS verification is skipped.
//   - `opts.backend` picks the backend explicitly: "recon", "curl", or
//     "auto" (the default — prefer recon, fall back to curl).
//
// Result shape:
//
//	{
//	  status:     number,                  // HTTP status code
//	  headers:    Record<string, string>,  // lower-cased header names
//	  body:       string,                  // UTF-8 decoded body
//	  durationMs: number,                  // wall-clock ms
//	  backend:    "recon" | "curl",        // which one ran
//	}
//
// Process-start failures (neither backend on PATH) and transport errors
// (DNS, connect refused, TLS handshake) throw. HTTP 4xx / 5xx do **not**
// throw — they're a normal HTTP outcome reported via `status`. Context
// deadline / cancel throws.
func execHTTP(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	methodArg := call.Argument(0)
	if methodArg == nil || goja.IsUndefined(methodArg) || goja.IsNull(methodArg) {
		return nil, errors.New("http: method argument required")
	}
	urlArg := call.Argument(1)
	if urlArg == nil || goja.IsUndefined(urlArg) || goja.IsNull(urlArg) {
		return nil, errors.New("http: url argument required")
	}
	method := strings.ToUpper(strings.TrimSpace(methodArg.String()))
	if method == "" {
		return nil, errors.New("http: method is empty")
	}
	url := urlArg.String()
	if url == "" {
		return nil, errors.New("http: url is empty")
	}

	opts := thirdArgAsMap(call)
	timeout := optMillis(opts, "timeout", 30*time.Second)
	backendPref := strings.ToLower(optString(opts, "backend", "auto"))
	follow := optBool(opts, "follow", false)
	insecure := optBool(opts, "insecure", false)
	body := optString(opts, "body", "")
	headers := optStringMap(opts, "headers")

	binPath, backend, err := resolveHTTPBackend(backendPref)
	if err != nil {
		return nil, err
	}

	// Two temp files: one for the request body (so --data-binary @<path>
	// works for both backends), one for the response headers (so -D
	// gives us a clean text file independent of the chosen backend's
	// stdout conventions).
	headerFile, err := os.CreateTemp("", "sercon-http-h-*")
	if err != nil {
		return nil, fmt.Errorf("http: temp header file: %w", err)
	}
	headerPath := headerFile.Name()
	_ = headerFile.Close()
	defer func() { _ = os.Remove(headerPath) }()

	var bodyPath string
	if body != "" {
		bf, err := os.CreateTemp("", "sercon-http-b-*")
		if err != nil {
			return nil, fmt.Errorf("http: temp body file: %w", err)
		}
		bodyPath = bf.Name()
		if _, err := bf.WriteString(body); err != nil {
			_ = bf.Close()
			_ = os.Remove(bodyPath)
			return nil, fmt.Errorf("http: write request body: %w", err)
		}
		_ = bf.Close()
		defer func() { _ = os.Remove(bodyPath) }()
	}

	argv := buildHTTPArgv(method, url, headers, bodyPath, headerPath, follow, insecure)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, binPath, argv...) //nolint:gosec // user-supplied url + headers are intentional
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	start := time.Now()
	runErr := cmd.Run()
	durationMs := time.Since(start).Milliseconds()

	if runErr != nil {
		if ctxErr := runCtx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("http: %w", ctxErr)
		}
		stderr := strings.TrimSpace(stderrBuf.String())
		if stderr != "" {
			return nil, fmt.Errorf("http: %s exited %w: %s", backend, runErr, stderr)
		}
		return nil, fmt.Errorf("http: %s exited %w", backend, runErr)
	}

	rawHeaders, err := os.ReadFile(headerPath)
	if err != nil {
		return nil, fmt.Errorf("http: read header file: %w", err)
	}
	status, respHeaders, err := parseHeaderFile(rawHeaders)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}

	return map[string]any{
		"status":     status,
		"headers":    respHeaders,
		"body":       stdoutBuf.String(),
		"durationMs": durationMs,
		"backend":    backend,
	}, nil
}

// resolveHTTPBackend turns the user's `opts.backend` preference into a
// concrete binary path plus a label for the result struct. "auto" walks
// recon → curl; the explicit choices error out if the chosen backend is
// missing rather than silently swapping.
func resolveHTTPBackend(pref string) (string, string, error) {
	switch pref {
	case "", "auto":
		if p, err := exec.LookPath("recon"); err == nil {
			return p, "recon", nil
		}
		if p, err := exec.LookPath("curl"); err == nil {
			return p, "curl", nil
		}
		return "", "", errors.New("http: neither recon nor curl found on PATH")
	case "recon":
		p, err := exec.LookPath("recon")
		if err != nil {
			return "", "", fmt.Errorf("http: recon not found on PATH: %w", err)
		}
		return p, "recon", nil
	case "curl":
		p, err := exec.LookPath("curl")
		if err != nil {
			return "", "", fmt.Errorf("http: curl not found on PATH: %w", err)
		}
		return p, "curl", nil
	default:
		return "", "", fmt.Errorf("http: backend must be 'auto', 'recon', or 'curl'; got %q", pref)
	}
}

// buildHTTPArgv assembles the curl-compatible argument list. `-D <path>`
// dumps response headers to a file we'll read back; the request body (if
// any) was already materialised to bodyPath, so we pass `--data-binary
// @<path>` to feed it byte-for-byte.
func buildHTTPArgv(method, url string, headers map[string]string, bodyPath, headerPath string, follow, insecure bool) []string {
	argv := []string{
		"-s", // silent
		"-X", method,
		"-D", headerPath,
	}
	for k, v := range headers {
		argv = append(argv, "-H", fmt.Sprintf("%s: %s", k, v))
	}
	if bodyPath != "" {
		argv = append(argv, "--data-binary", "@"+bodyPath)
	}
	if follow {
		argv = append(argv, "-L")
	}
	if insecure {
		argv = append(argv, "-k")
	}
	argv = append(argv, url)
	return argv
}

// parseHeaderFile decodes the output of `-D` into a status code and a
// header map. Both backends end each line with `\n`; curl uses `\r\n` and
// recon uses `\n`. Recon also prefixes the status line oddly
// (`HTTP/HTTP/2.0 200 OK`); we tolerate that by scanning for the first
// space-separated 3-digit token rather than positionally indexing.
//
// On redirect chains, both backends concatenate one block per hop in the
// same file. We pick the last block — the final response.
func parseHeaderFile(raw []byte) (int, map[string]string, error) {
	if len(raw) == 0 {
		return 0, nil, errors.New("empty header dump")
	}
	block := lastResponseBlock(raw)
	lines := strings.Split(string(block), "\n")
	if len(lines) == 0 {
		return 0, nil, errors.New("empty header block")
	}

	status, err := parseStatusCode(strings.TrimRight(lines[0], "\r"))
	if err != nil {
		return 0, nil, err
	}

	headers := map[string]string{}
	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		headers[key] = val
	}
	return status, headers, nil
}

// parseStatusCode finds the first three-digit integer in the status
// line. Curl prints `HTTP/1.1 200 OK`; recon prints `HTTP/HTTP/2.0 200
// OK`. Walking by token sidesteps the difference.
func parseStatusCode(line string) (int, error) {
	for _, tok := range strings.Fields(line) {
		if len(tok) != 3 {
			continue
		}
		n, err := strconv.Atoi(tok)
		if err != nil {
			continue
		}
		if n >= 100 && n < 600 {
			return n, nil
		}
	}
	return 0, fmt.Errorf("no status code in line %q", line)
}

// lastResponseBlock scans for line-start "HTTP" markers and returns the
// slice starting at the final one. Used to skip past redirect-chain
// intermediates.
func lastResponseBlock(raw []byte) []byte {
	marker := []byte("HTTP")
	lastStart := -1
	for i := 0; i+len(marker) <= len(raw); i++ {
		if i != 0 && raw[i-1] != '\n' {
			continue
		}
		if bytes.HasPrefix(raw[i:], marker) {
			lastStart = i
		}
	}
	if lastStart < 0 {
		return raw
	}
	return raw[lastStart:]
}

// thirdArgAsMap pulls the third positional argument out as a Go map.
// Mirrors optsAsMap but at position 2 (zero-indexed) for bindings whose
// signature is (a, b, opts) rather than (a, opts).
func thirdArgAsMap(call goja.FunctionCall) map[string]any {
	if len(call.Arguments) < 3 {
		return nil
	}
	arg := call.Argument(2)
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return nil
	}
	if m, ok := arg.Export().(map[string]any); ok {
		return m
	}
	return nil
}

// optBool reads a boolean option with a fallback. Anything that isn't a
// JS boolean (or missing) yields the fallback.
func optBool(opts map[string]any, key string, fallback bool) bool {
	if opts == nil {
		return fallback
	}
	v, ok := opts[key]
	if !ok {
		return fallback
	}
	b, ok := v.(bool)
	if !ok {
		return fallback
	}
	return b
}

// optStringMap reads a Record<string, string> option. Non-string values
// are stringified via fmt.Sprint so callers can pass `{ "X-Count": 7 }`
// without explicit coercion.
func optStringMap(opts map[string]any, key string) map[string]string {
	if opts == nil {
		return nil
	}
	v, ok := opts[key]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		out[k] = fmt.Sprint(val)
	}
	return out
}
