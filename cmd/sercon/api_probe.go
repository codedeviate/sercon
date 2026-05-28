package main

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/ntp"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/likexian/whois"
	whoisparser "github.com/likexian/whois-parser"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// probeNamespace builds the `api.net.*` member map: `tcp`, `dns`, `tls`,
// `ntp`, `whois`. Every member returns a Promise (uses
// `scriptengine.PromisifyAsync` under the hood) so scripts can
// `await api.net.tcp("host:port")`. All bindings honour a `timeout` opt in
// milliseconds; defaults vary per binding.
func probeNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"tcp":   scriptengine.PromisifyAsync(vm, loop, tcpProbe),
		"dns":   scriptengine.PromisifyAsync(vm, loop, dnsLookup),
		"tls":   scriptengine.PromisifyAsync(vm, loop, tlsProbe),
		"ntp":   scriptengine.PromisifyAsync(vm, loop, ntpQuery),
		"whois": scriptengine.PromisifyAsync(vm, loop, whoisLookup),
		"ping":  scriptengine.PromisifyAsync(vm, loop, pingProbe),
		"smtp":  scriptengine.PromisifyAsync(vm, loop, smtpProbe),
		"wss":   scriptengine.PromisifyAsync(vm, loop, wssProbe),
	}
}

// optsAsMap pulls the second positional argument out of a Promise-shaped call
// and returns it as a Go map. JS callers can pass `undefined`, `null`, omit
// the arg entirely, or hand in an object — any of those yield nil here.
func optsAsMap(call goja.FunctionCall) map[string]any {
	if len(call.Arguments) < 2 {
		return nil
	}
	arg := call.Argument(1)
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return nil
	}
	if m, ok := arg.Export().(map[string]any); ok {
		return m
	}
	return nil
}

func optMillis(opts map[string]any, key string, fallback time.Duration) time.Duration {
	v, ok := opts[key]
	if !ok {
		return fallback
	}
	switch t := v.(type) {
	case int64:
		return time.Duration(t) * time.Millisecond
	case int:
		return time.Duration(t) * time.Millisecond
	case float64:
		return time.Duration(t) * time.Millisecond
	}
	return fallback
}

func optString(opts map[string]any, key, fallback string) string {
	if v, ok := opts[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

func tcpProbe(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	target := call.Argument(0).String()
	opts := optsAsMap(call)
	timeout := optMillis(opts, "timeout", 5*time.Second)
	defaultPort := optString(opts, "port", "80")

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		port = defaultPort
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	d := &net.Dialer{}
	conn, err := d.DialContext(dialCtx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	latency := time.Since(start)
	defer func() { _ = conn.Close() }()

	remoteIP := ""
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		remoteIP = addr.IP.String()
	}

	portInt, _ := strconv.Atoi(port)
	return map[string]any{
		"host":      host,
		"port":      portInt,
		"ip":        remoteIP,
		"latencyMs": float64(latency.Microseconds()) / 1000.0,
	}, nil
}

// dnsLookup runs lookups in parallel-friendly form but actually sequential
// here — the surface is small and each lookup is fast. Empty result sets are
// omitted from the returned object so scripts can check membership with
// `if ("mx" in result)`.
func dnsLookup(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	host := call.Argument(0).String()
	opts := optsAsMap(call)

	var typesFilter map[string]bool
	if raw, ok := opts["types"]; ok {
		if arr, ok := raw.([]any); ok {
			typesFilter = make(map[string]bool, len(arr))
			for _, v := range arr {
				if s, ok := v.(string); ok {
					typesFilter[strings.ToLower(s)] = true
				}
			}
		}
	}
	want := func(t string) bool {
		if typesFilter == nil {
			return true
		}
		return typesFilter[t]
	}

	r := &net.Resolver{}
	out := map[string]any{}

	if want("a") || want("aaaa") {
		ips, err := r.LookupIPAddr(ctx, host)
		if err == nil {
			var a4, a6 []string
			for _, ip := range ips {
				if ip.IP.To4() != nil {
					a4 = append(a4, ip.IP.String())
				} else {
					a6 = append(a6, ip.IP.String())
				}
			}
			if want("a") && len(a4) > 0 {
				out["a"] = a4
			}
			if want("aaaa") && len(a6) > 0 {
				out["aaaa"] = a6
			}
		}
	}

	if want("mx") {
		if mxs, err := r.LookupMX(ctx, host); err == nil && len(mxs) > 0 {
			entries := make([]map[string]any, 0, len(mxs))
			for _, m := range mxs {
				entries = append(entries, map[string]any{
					"preference": int(m.Pref),
					"host":       m.Host,
				})
			}
			out["mx"] = entries
		}
	}

	if want("txt") {
		if txts, err := r.LookupTXT(ctx, host); err == nil && len(txts) > 0 {
			out["txt"] = txts
		}
	}

	if want("cname") {
		if cname, err := r.LookupCNAME(ctx, host); err == nil && cname != "" && cname != host+"." {
			out["cname"] = cname
		}
	}

	if want("ns") {
		if nses, err := r.LookupNS(ctx, host); err == nil && len(nses) > 0 {
			names := make([]string, 0, len(nses))
			for _, ns := range nses {
				names = append(names, ns.Host)
			}
			out["ns"] = names
		}
	}

	return out, nil
}

// tlsProbe dials the target with InsecureSkipVerify so even expired or
// hostname-mismatched certs can be inspected — the binding is for surveying
// the certificate, not for proving it's valid. Hosts that care about that
// should re-validate themselves with crypto/x509.Verify or
// scripts can call api.net.tls and decide based on the returned fields.
func tlsProbe(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	target := call.Argument(0).String()
	opts := optsAsMap(call)
	timeout := optMillis(opts, "timeout", 5*time.Second)

	host, port, err := net.SplitHostPort(target)
	if err != nil {
		host = target
		port = "443"
	}

	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	d := tls.Dialer{
		NetDialer: &net.Dialer{},
		Config: &tls.Config{
			ServerName:         host,
			InsecureSkipVerify: true, //nolint:gosec // certificate inspection on demand
		},
	}
	conn, err := d.DialContext(dialCtx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return nil, errors.New("tls dial returned non-TLS connection")
	}
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, errors.New("no peer certificates presented")
	}
	leaf := certs[0]
	fp := sha256.Sum256(leaf.Raw)

	now := time.Now()
	daysRemaining := int(leaf.NotAfter.Sub(now).Hours() / 24)

	return map[string]any{
		"cn":                leaf.Subject.CommonName,
		"issuer":            leaf.Issuer.CommonName,
		"notBefore":         leaf.NotBefore.UTC().Format(time.RFC3339),
		"notAfter":          leaf.NotAfter.UTC().Format(time.RFC3339),
		"daysRemaining":     daysRemaining,
		"dnsNames":          leaf.DNSNames,
		"serialNumber":      leaf.SerialNumber.String(),
		"fingerprintSha256": hex.EncodeToString(fp[:]),
	}, nil
}

// ntpQuery hits an NTP server (UDP 123 by default) and returns clock-skew
// data. Uses beevik/ntp which speaks NTPv4 and packages the response into
// a struct we flatten for JS. Durations are reported in milliseconds with
// sub-millisecond precision so the values are easy to compare.
func ntpQuery(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	host := call.Argument(0).String()
	opts := optsAsMap(call)
	timeout := optMillis(opts, "timeout", 5*time.Second)
	port := 123
	if v, ok := opts["port"]; ok {
		switch t := v.(type) {
		case int64:
			port = int(t)
		case float64:
			port = int(t)
		case string:
			if p, err := strconv.Atoi(t); err == nil {
				port = p
			}
		}
	}

	resp, err := ntp.QueryWithOptions(host, ntp.QueryOptions{
		Timeout: timeout,
		Port:    port,
	})
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"serverTime":       resp.Time.UTC().Format(time.RFC3339Nano),
		"offsetMs":         float64(resp.ClockOffset.Microseconds()) / 1000.0,
		"rttMs":            float64(resp.RTT.Microseconds()) / 1000.0,
		"stratum":          int(resp.Stratum),
		"referenceTime":    resp.ReferenceTime.UTC().Format(time.RFC3339Nano),
		"rootDelayMs":      float64(resp.RootDelay.Microseconds()) / 1000.0,
		"rootDispersionMs": float64(resp.RootDispersion.Microseconds()) / 1000.0,
	}, nil
}

// whoisLookup performs a two-hop WHOIS query (IANA -> registrar's whois
// server) via likexian/whois, then runs likexian/whois-parser over the raw
// text to extract the common fields. The raw response is always returned;
// parsed fields are best-effort and may be missing for TLDs the parser
// doesn't recognise.
//
// likexian/whois doesn't accept a context — its timeout is a per-Client
// setting (time.Duration). The host engine's outer timeout still kicks in
// via vm.Interrupt; this just shapes the wire-level wait.
func whoisLookup(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
	domain := call.Argument(0).String()
	opts := optsAsMap(call)
	timeout := optMillis(opts, "timeout", 10*time.Second)

	client := whois.NewClient().SetTimeout(timeout)
	raw, err := client.Whois(domain)
	if err != nil {
		return nil, err
	}

	out := map[string]any{"raw": raw}
	parsed, parseErr := whoisparser.Parse(raw)
	if parseErr == nil {
		if parsed.Domain != nil {
			// whois-parser puts the whois server on Domain, not on Registrar
			// — it's the server that served the lookup, not a registrar field.
			out["domain"] = map[string]any{
				"name":           parsed.Domain.Name,
				"punycode":       parsed.Domain.Punycode,
				"whoisServer":    parsed.Domain.WhoisServer,
				"nameServers":    parsed.Domain.NameServers,
				"status":         parsed.Domain.Status,
				"dnssec":         parsed.Domain.DNSSec,
				"createdDate":    parsed.Domain.CreatedDate,
				"updatedDate":    parsed.Domain.UpdatedDate,
				"expirationDate": parsed.Domain.ExpirationDate,
			}
		}
		if parsed.Registrar != nil {
			out["registrar"] = map[string]any{
				"name": parsed.Registrar.Name,
			}
		}
	}
	return out, nil
}
