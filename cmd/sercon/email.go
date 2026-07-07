package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// emailNamespace builds the `net.email.*` member map. Each member returns a
// Promise. Lookups for record types that aren't found return
// `{ present: false }` rather than throwing, so scripts can use a uniform
// presence-check pattern across the email-auth probe family.
func emailNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"spf":    scriptengine.PromisifyAsync(vm, loop, emailDomainExtract, emailSPFCore),
		"dmarc":  scriptengine.PromisifyAsync(vm, loop, emailDomainExtract, emailDMARCCore),
		"mtaSts": scriptengine.PromisifyAsync(vm, loop, emailDomainExtract, emailMTASTSCore),
		"tlsRpt": scriptengine.PromisifyAsync(vm, loop, emailDomainExtract, emailTLSRPTCore),
		"bimi":   scriptengine.PromisifyAsync(vm, loop, emailBIMIExtract, emailBIMIOp),
		"all":    scriptengine.PromisifyAsync(vm, loop, emailDomainExtract, emailAllOp),
		"send":   emailSend(vm, loop),
	}
}

// emailDomainExtract is the on-loop extract shared by the probes whose only
// argument is the domain string (spf, dmarc, mtaSts, tlsRpt, all).
func emailDomainExtract(call goja.FunctionCall) (string, error) {
	return call.Argument(0).String(), nil
}

// findRecord scans TXT records for one starting with the given (case-
// insensitive) marker. The first match wins.
func findRecord(txts []string, marker string) string {
	lm := strings.ToLower(marker)
	for _, t := range txts {
		if strings.HasPrefix(strings.ToLower(t), lm) {
			return t
		}
	}
	return ""
}

// dnsTXTOrAbsent looks up TXT records for name, treating NXDOMAIN as a
// successful "no record" outcome. Lets the email-auth probes return
// `{ present: false }` uniformly instead of throwing for missing names.
func dnsTXTOrAbsent(ctx context.Context, name string) ([]string, bool, error) {
	r := &net.Resolver{}
	txts, err := r.LookupTXT(ctx, name)
	if err == nil {
		return txts, true, nil
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
		return nil, false, nil
	}
	return nil, false, err
}

// emailSPFCore queries TXT records at the apex of the given domain and looks
// for an SPF record (one starting with `v=spf1`). The record is returned
// verbatim plus a tokenised list of mechanisms; the trailing `all`-style
// mechanism is summarised under `allPolicy`.
func emailSPFCore(ctx context.Context, domain string) (map[string]any, error) {
	txts, found, err := dnsTXTOrAbsent(ctx, domain)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{"present": false}, nil
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

// emailDMARCCore queries TXT records at `_dmarc.<domain>` and parses the one
// starting with `v=DMARC1` into a tag map. The common policy / report-uri
// tags are also surfaced on the result so scripts don't have to dig into
// the raw tag map for the usual cases.
func emailDMARCCore(ctx context.Context, domain string) (map[string]any, error) {
	txts, found, err := dnsTXTOrAbsent(ctx, "_dmarc."+domain)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{"present": false}, nil
	}
	record := findRecord(txts, "v=DMARC1")
	if record == "" {
		return map[string]any{"present": false}, nil
	}
	tags := parseTagMap(record)
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

// emailMTASTSCore combines the `_mta-sts.<domain>` TXT record with the policy
// file served at `https://mta-sts.<domain>/.well-known/mta-sts.txt`. The
// TXT carries a versioned `id` for change detection; the policy file is
// where the actual mode + mx list lives.
func emailMTASTSCore(ctx context.Context, domain string) (map[string]any, error) {
	txts, found, err := dnsTXTOrAbsent(ctx, "_mta-sts."+domain)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{"present": false}, nil
	}
	record := findRecord(txts, "v=STSv1")
	if record == "" {
		return map[string]any{"present": false}, nil
	}
	tags := parseTagMap(record)
	out := map[string]any{
		"present": true,
		"record":  record,
		"txt":     map[string]any{"v": tags["v"], "id": tags["id"]},
	}
	if policy, perr := fetchMTASTSPolicy(ctx, domain); perr == nil {
		out["policy"] = policy
	} else {
		out["policyError"] = perr.Error()
	}
	return out, nil
}

// fetchMTASTSPolicy retrieves and parses the well-known policy file. The
// fetch is bounded by its own context; the outer engine timeout applies on
// top via the watcher in pkg/scriptengine.
func fetchMTASTSPolicy(ctx context.Context, domain string) (map[string]any, error) {
	httpCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(httpCtx, http.MethodGet,
		"https://mta-sts."+domain+"/.well-known/mta-sts.txt", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("mta-sts policy: HTTP " + strconv.Itoa(resp.StatusCode))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return nil, err
	}
	return parseMTASTSPolicy(string(body)), nil
}

// parseMTASTSPolicy reads the line-based `key: value` format defined by
// RFC 8461. `mx:` lines are aggregated into a single slice; `max_age` is
// parsed to an int when it looks numeric. Unknown keys are kept verbatim.
func parseMTASTSPolicy(body string) map[string]any {
	out := map[string]any{}
	var mxList []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimRight(line, "\r")
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(line[:colon]))
		val := strings.TrimSpace(line[colon+1:])
		switch key {
		case "mx":
			mxList = append(mxList, val)
		case "max_age":
			if n, err := strconv.Atoi(val); err == nil {
				out["maxAge"] = n
			} else {
				out["maxAge"] = val
			}
		case "version", "mode":
			out[key] = val
		default:
			out[key] = val
		}
	}
	if mxList != nil {
		out["mx"] = mxList
	}
	return out
}

// emailTLSRPTCore looks up `_smtp._tls.<domain>` TXT and parses the
// `v=TLSRPTv1; rua=...` form into a tag map. `rua` is surfaced separately
// because that's the actionable bit.
func emailTLSRPTCore(ctx context.Context, domain string) (map[string]any, error) {
	txts, found, err := dnsTXTOrAbsent(ctx, "_smtp._tls."+domain)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{"present": false}, nil
	}
	record := findRecord(txts, "v=TLSRPTv1")
	if record == "" {
		return map[string]any{"present": false}, nil
	}
	tags := parseTagMap(record)
	return map[string]any{
		"present": true,
		"record":  record,
		"tags":    tags,
		"rua":     tags["rua"],
	}, nil
}

// emailBIMIArgs carries the on-loop-extracted arguments for net.email.bimi.
type emailBIMIArgs struct {
	domain   string
	selector string
}

// emailBIMIExtract is the on-loop extract for net.email.bimi: the domain plus
// the optional opts.selector (defaults to `default`), which lets callers
// query a non-default selector explicitly.
func emailBIMIExtract(call goja.FunctionCall) (emailBIMIArgs, error) {
	args := emailBIMIArgs{
		domain:   call.Argument(0).String(),
		selector: "default",
	}
	if opts := optsAsMap(call); opts != nil {
		if s, ok := opts["selector"].(string); ok && s != "" {
			args.selector = s
		}
	}
	return args, nil
}

// emailBIMIOp looks up `<selector>._bimi.<domain>`.
func emailBIMIOp(ctx context.Context, args emailBIMIArgs) (map[string]any, error) {
	return emailBIMICore(ctx, args.domain, args.selector)
}

func emailBIMICore(ctx context.Context, domain, selector string) (map[string]any, error) {
	if selector == "" {
		selector = "default"
	}
	txts, found, err := dnsTXTOrAbsent(ctx, selector+"._bimi."+domain)
	if err != nil {
		return nil, err
	}
	if !found {
		return map[string]any{"present": false, "selector": selector}, nil
	}
	record := findRecord(txts, "v=BIMI1")
	if record == "" {
		return map[string]any{"present": false, "selector": selector}, nil
	}
	tags := parseTagMap(record)
	return map[string]any{
		"present":  true,
		"selector": selector,
		"record":   record,
		"tags":     tags,
		"l":        tags["l"],
		"a":        tags["a"],
	}, nil
}

// emailAllOp runs every probe in parallel and returns a single aggregate
// object keyed by probe name. Per-probe failures don't fail the aggregate
// — they surface under `<probe>.error` so a partial result is still
// useful (e.g. SPF + DMARC found, MTA-STS policy fetch timed out).
func emailAllOp(ctx context.Context, domain string) (map[string]any, error) {
	type result struct {
		name string
		val  map[string]any
		err  error
	}
	results := make(chan result, 5)

	probes := []struct {
		name string
		run  func(ctx context.Context, domain string) (map[string]any, error)
	}{
		{"spf", emailSPFCore},
		{"dmarc", emailDMARCCore},
		{"mtaSts", emailMTASTSCore},
		{"tlsRpt", emailTLSRPTCore},
		{"bimi", func(ctx context.Context, d string) (map[string]any, error) {
			return emailBIMICore(ctx, d, "default")
		}},
	}
	for _, p := range probes {
		p := p
		go func() {
			val, err := p.run(ctx, domain)
			results <- result{name: p.name, val: val, err: err}
		}()
	}

	out := map[string]any{"domain": domain}
	for i := 0; i < len(probes); i++ {
		r := <-results
		if r.err != nil {
			out[r.name] = map[string]any{"error": r.err.Error()}
		} else {
			out[r.name] = r.val
		}
	}
	return out, nil
}

// parseTagMap splits a `k1=v1; k2=v2` record (DMARC, BIMI, TLS-RPT, the
// MTA-STS TXT marker) into a case-folded key → raw-value map. Whitespace
// around `;` and around the `=` is trimmed; values keep their internal
// whitespace since some tags carry comma-separated lists (e.g. DMARC `rua`).
func parseTagMap(record string) map[string]string {
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

// parseDMARCTags is the legacy alias kept around for the existing test
// that pins the parser's behaviour. The shape is identical to parseTagMap.
func parseDMARCTags(record string) map[string]string { return parseTagMap(record) }
