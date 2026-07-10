package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/dop251/goja"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	htransport "google.golang.org/api/transport/http"
)

// googleTestOptions is consulted ONLY by the JS-level binding path so tests can
// point clients at an httptest server. It is empty in production. Go-level unit
// tests instead pass their options through clientOptions(extra...).
var googleTestOptions []option.ClientOption

// clientOptions maps the config to google.golang.org/api options. extra is
// appended last (tests inject WithoutAuthentication/WithEndpoint/WithHTTPClient).
func (c googleConfig) clientOptions(extra ...option.ClientOption) []option.ClientOption {
	opts := make([]option.ClientOption, 0, 6)
	if c.credentialsFile != "" {
		//nolint:staticcheck // SA1019: WithCredentialsFile is the documented way to load an explicit service-account key path (exactly this binding's purpose); the non-deprecated path needs a ctx clientOptions() intentionally does not carry.
		opts = append(opts, option.WithCredentialsFile(c.credentialsFile))
	}
	if len(c.credentialsJSON) > 0 {
		//nolint:staticcheck // SA1019: WithCredentialsJSON is the documented way to load inline service-account JSON (exactly this binding's purpose); the non-deprecated path needs a ctx clientOptions() intentionally does not carry.
		opts = append(opts, option.WithCredentialsJSON(c.credentialsJSON))
	}
	if len(c.scopes) > 0 {
		opts = append(opts, option.WithScopes(c.scopes...))
	}
	if c.quotaProject != "" {
		opts = append(opts, option.WithQuotaProject(c.quotaProject))
	}
	opts = append(opts, extra...)
	return opts
}

// String redacts credentials — safe for logs and error strings.
func (c googleConfig) String() string {
	creds := "adc"
	if c.credentialsFile != "" || len(c.credentialsJSON) > 0 {
		creds = "explicit(redacted)"
	}
	return fmt.Sprintf("googleConfig{project:%q quotaProject:%q scopes:%d creds:%s}",
		c.project, c.quotaProject, len(c.scopes), creds)
}

type googleError struct {
	code    int
	status  string
	message string
	details any
}

func (e googleError) Error() string {
	return fmt.Sprintf("cloud.google: %d %s: %s", e.code, e.status, e.message)
}

func (e googleError) ErrorFields() map[string]any {
	return map[string]any{"code": e.code, "status": e.status, "message": e.message, "details": e.details}
}

// mapGoogleError normalises an SDK/transport error into a googleError (which
// carries structured fields). Non-API errors (DNS/TLS/timeout) map to code 0.
func mapGoogleError(err error) error {
	if err == nil {
		return nil
	}
	var ae *googleapi.Error
	if errors.As(err, &ae) {
		return googleError{code: ae.Code, status: http.StatusText(ae.Code), message: ae.Message, details: ae.Errors}
	}
	return googleError{code: 0, status: "TRANSPORT", message: err.Error()}
}

// googleCallArgs is the plain-Go carrier for cloud.google(...).call({...}),
// extracted on-loop by googleCallExtract and consumed off-loop by
// googleCallWork.
type googleCallArgs struct {
	api, version, httpMethod, path string
	params                         map[string]string
	body                           any
	endpointBase                   string // test-only; empty ⇒ https://{api}.googleapis.com
}

// googleCallExtract runs on the event loop: read + validate JS args.
func googleCallExtract(cfg googleConfig) func(goja.FunctionCall) (googleCallArgs, error) {
	return func(call goja.FunctionCall) (googleCallArgs, error) {
		obj, ok := call.Argument(0).(*goja.Object)
		if !ok {
			return googleCallArgs{}, errors.New("cloud.google.call: an options object is required")
		}
		o, ok := obj.Export().(map[string]any)
		if !ok {
			return googleCallArgs{}, errors.New("cloud.google.call: an options object is required")
		}
		a := googleCallArgs{
			api:        optString(o, "api", ""),
			version:    optString(o, "version", "v1"),
			httpMethod: strings.ToUpper(optString(o, "httpMethod", "GET")),
			path:       optString(o, "path", ""),
			params:     optStringMap(o, "params"),
			body:       o["body"],
		}
		if a.api == "" || a.path == "" {
			return a, errors.New("cloud.google.call: `api` and `path` are required")
		}
		return a, nil
	}
}

// authedClient returns an authenticated *http.Client for cfg (ADC or explicit
// credentials), with test options appended. extra is appended last, after
// googleTestOptions; googleCallWork uses it to pass option.WithoutAuthentication()
// when a.endpointBase is set (Go-level unit tests against httptest, which have
// no real Google credentials to present).
func authedClient(ctx context.Context, cfg googleConfig, extra ...option.ClientOption) (*http.Client, error) {
	opts := append(append([]option.ClientOption{}, googleTestOptions...), extra...)
	c, _, err := htransport.NewClient(ctx, cfg.clientOptions(opts...)...)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	return c, nil
}

// googleCallWork runs off the event loop: perform the REST call, decode JSON.
func googleCallWork(ctx context.Context, cfg googleConfig, a googleCallArgs) (any, error) {
	var authExtra []option.ClientOption
	if a.endpointBase != "" {
		// Test-only: endpointBase points at an httptest server, which has no
		// real Google auth to present. Production calls never set this field.
		authExtra = append(authExtra, option.WithoutAuthentication())
	}
	client, err := authedClient(ctx, cfg, authExtra...)
	if err != nil {
		return nil, err
	}
	base := a.endpointBase
	if base == "" {
		base = "https://" + a.api + ".googleapis.com"
	}
	u, err := url.Parse(base + a.path)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	q := u.Query()
	for k, v := range a.params {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()

	var bodyReader io.Reader
	if a.body != nil {
		bb, mErr := json.Marshal(a.body)
		if mErr != nil {
			return nil, googleError{code: 0, status: "ENCODE", message: "cloud.google.call: body is not JSON-serialisable"}
		}
		bodyReader = bytes.NewReader(bb)
	}
	req, err := http.NewRequestWithContext(ctx, a.httpMethod, u.String(), bodyReader)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, mapGoogleError(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, googleError{code: resp.StatusCode, status: http.StatusText(resp.StatusCode), message: strings.TrimSpace(string(raw))}
	}
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, googleError{code: resp.StatusCode, status: "DECODE", message: "cloud.google.call: response was not JSON"}
	}
	return out, nil
}
