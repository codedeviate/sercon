package main

import (
	"context"
	"errors"
	"net"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// emailNamespace builds the `api.email.*` member map. Each member returns a
// Promise. Lookups for record types that aren't found return
// `{ present: false }` rather than throwing, so scripts can use a uniform
// presence-check pattern across the email-auth probe family.
func emailNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"spf":   scriptengine.PromisifyAsync(vm, loop, emailSPF),
		"dmarc": scriptengine.PromisifyAsync(vm, loop, emailDMARC),
	}
}

// emailSPF queries TXT records at the apex of the given domain and looks for
// an SPF record (one starting with `v=spf1`). The record is returned verbatim
// plus a tokenised list of mechanisms; the trailing `all`-style mechanism is
// summarised under `allPolicy` ("pass" / "fail" / "softfail" / "neutral")
// because that's the single most-asked-for SPF facet.
func emailSPF(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	domain := call.Argument(0).String()
	r := &net.Resolver{}

	txts, err := r.LookupTXT(ctx, domain)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return map[string]any{"present": false}, nil
		}
		return nil, err
	}
	var record string
	for _, t := range txts {
		switch {
		case t == "v=spf1",
			strings.HasPrefix(t, "v=spf1 "),
			strings.HasPrefix(t, "v=spf1\t"):
			record = t
		}
		if record != "" {
			break
		}
	}
	if record == "" {
		return map[string]any{"present": false}, nil
	}
	parts := strings.Fields(record)
	var mechanisms []string
	if len(parts) > 1 {
		mechanisms = parts[1:]
	}
	allPolicy := ""
	for _, m := range mechanisms {
		switch strings.ToLower(m) {
		case "all", "+all":
			allPolicy = "pass"
		case "-all":
			allPolicy = "fail"
		case "~all":
			allPolicy = "softfail"
		case "?all":
			allPolicy = "neutral"
		}
	}
	return map[string]any{
		"present":    true,
		"record":     record,
		"mechanisms": mechanisms,
		"allPolicy":  allPolicy,
	}, nil
}

// emailDMARC queries TXT records at `_dmarc.<domain>` and parses the one
// starting with `v=DMARC1` into a tag map. We expose every tag verbatim
// (as a flat string map) plus the parsed policy / subdomain-policy values
// because those are what most callers actually want.
func emailDMARC(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	domain := call.Argument(0).String()
	r := &net.Resolver{}

	txts, err := r.LookupTXT(ctx, "_dmarc."+domain)
	if err != nil {
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return map[string]any{"present": false}, nil
		}
		return nil, err
	}
	var record string
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(t), "v=dmarc1") {
			record = t
			break
		}
	}
	if record == "" {
		return map[string]any{"present": false}, nil
	}
	tags := parseDMARCTags(record)
	return map[string]any{
		"present":   true,
		"record":    record,
		"tags":      tags,
		"policy":    tags["p"],
		"subdomain": tags["sp"],
		"percent":   tags["pct"],
		"rua":       tags["rua"],
		"ruf":       tags["ruf"],
	}, nil
}

// parseDMARCTags splits a DMARC record (`v=DMARC1; p=reject; …`) into a
// case-folded key → raw-value map. Whitespace around `;` and around the `=`
// is trimmed; values keep their internal whitespace since some tags carry
// comma-separated lists (e.g. `rua`).
func parseDMARCTags(record string) map[string]string {
	tags := map[string]string{}
	for _, part := range strings.Split(record, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		eq := strings.IndexByte(part, '=')
		if eq <= 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(part[:eq]))
		val := strings.TrimSpace(part[eq+1:])
		tags[key] = val
	}
	return tags
}
