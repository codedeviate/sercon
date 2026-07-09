package main

import (
	"errors"
	"fmt"
	"net/http"

	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
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
		opts = append(opts, option.WithCredentialsFile(c.credentialsFile))
	}
	if len(c.credentialsJSON) > 0 {
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
	return fmt.Sprintf("googleConfig{project:%q location:%q quotaProject:%q scopes:%d creds:%s}",
		c.project, c.location, c.quotaProject, len(c.scopes), creds)
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
