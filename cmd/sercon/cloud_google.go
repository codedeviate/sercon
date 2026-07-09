package main

import (
	"fmt"

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
