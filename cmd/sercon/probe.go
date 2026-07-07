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

// probeNamespace builds the `net.*` member map: `tcp`, `dns`, `tls`,
// `ntp`, `whois`. Every member returns a Promise (uses
// `scriptengine.PromisifyAsync` under the hood) so scripts can
// `await net.probe.tcp("host:port")`. All bindings honour a `timeout` opt in
// milliseconds; defaults vary per binding.
func probeNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"tcp":        scriptengine.PromisifyAsyncLegacy(vm, loop, tcpProbe),
		"dns":        scriptengine.PromisifyAsyncLegacy(vm, loop, dnsLookup),
		"tls":        scriptengine.PromisifyAsyncLegacy(vm, loop, tlsProbe),
		"ntp":        scriptengine.PromisifyAsyncLegacy(vm, loop, ntpQuery),
		"whois":      scriptengine.PromisifyAsyncLegacy(vm, loop, whoisLookup),
		"ping":       scriptengine.PromisifyAsyncLegacy(vm, loop, pingProbe),
		"smtp":       scriptengine.PromisifyAsyncLegacy(vm, loop, smtpProbe),
		"wss":        scriptengine.PromisifyAsyncLegacy(vm, loop, wssProbe),
		"traceroute": scriptengine.PromisifyAsyncLegacy(vm, loop, traceroute),
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

// tcpProbeResult is the resolved value of net.probe.tcp. It's a json-tagged
// struct rather than a map[string]any so the JS object's key order is stable
// (goja enumerates struct fields in declaration order; a Go map would shuffle
// JSON.stringify output run-to-run — see TestStructResult_StableKeyOrder).
type tcpProbeResult struct {
	Host      string  `json:"host"`
	Port      int     `json:"port"`
	IP        string  `json:"ip"`
	LatencyMs float64 `json:"latencyMs"`
}

func tcpProbe(ctx context.Context, call goja.FunctionCall) (tcpProbeResult, error) {
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
		return tcpProbeResult{}, err
	}
	latency := time.Since(start)
	defer func() { _ = conn.Close() }()

	remoteIP := ""
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		remoteIP = addr.IP.String()
	}

	portInt, _ := strconv.Atoi(port)
	return tcpProbeResult{
		Host:      host,
		Port:      portInt,
		IP:        remoteIP,
		LatencyMs: float64(latency.Microseconds()) / 1000.0,
	}, nil
}

// dnsLookup runs lookups in parallel-friendly form but actually sequential
// here — the surface is small and each lookup is fast. Empty result sets are
// omitted from the returned object so scripts can check membership with
// `if ("mx" in result)`.
func dnsLookup(ctx context.Context, call goja.FunctionCall) (*scriptengine.Ordered, error) {
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
	out := scriptengine.NewOrdered()

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
				out.Set("a", a4)
			}
			if want("aaaa") && len(a6) > 0 {
				out.Set("aaaa", a6)
			}
		}
	}

	if want("mx") {
		if mxs, err := r.LookupMX(ctx, host); err == nil && len(mxs) > 0 {
			entries := make([]*scriptengine.Ordered, 0, len(mxs))
			for _, m := range mxs {
				entries = append(entries, scriptengine.NewOrdered().
					Set("preference", int(m.Pref)).
					Set("host", m.Host))
			}
			out.Set("mx", entries)
		}
	}

	if want("txt") {
		if txts, err := r.LookupTXT(ctx, host); err == nil && len(txts) > 0 {
			out.Set("txt", txts)
		}
	}

	if want("cname") {
		if cname, err := r.LookupCNAME(ctx, host); err == nil && cname != "" && cname != host+"." {
			out.Set("cname", cname)
		}
	}

	if want("ns") {
		if nses, err := r.LookupNS(ctx, host); err == nil && len(nses) > 0 {
			names := make([]string, 0, len(nses))
			for _, ns := range nses {
				names = append(names, ns.Host)
			}
			out.Set("ns", names)
		}
	}

	return out, nil
}

// tlsProbe dials the target with InsecureSkipVerify so even expired or
// hostname-mismatched certs can be inspected — the binding is for surveying
// the certificate, not for proving it's valid. Hosts that care about that
// should re-validate themselves with crypto/x509.Verify or
// scripts can call net.probe.tls and decide based on the returned fields.
func tlsProbe(ctx context.Context, call goja.FunctionCall) (*scriptengine.Ordered, error) {
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

	return scriptengine.NewOrdered().
		Set("cn", leaf.Subject.CommonName).
		Set("issuer", leaf.Issuer.CommonName).
		Set("notBefore", leaf.NotBefore.UTC().Format(time.RFC3339)).
		Set("notAfter", leaf.NotAfter.UTC().Format(time.RFC3339)).
		Set("daysRemaining", daysRemaining).
		Set("dnsNames", leaf.DNSNames).
		Set("serialNumber", leaf.SerialNumber.String()).
		Set("fingerprintSha256", hex.EncodeToString(fp[:])), nil
}

// ntpQuery hits an NTP server (UDP 123 by default) and returns clock-skew
// data. Uses beevik/ntp which speaks NTPv4 and packages the response into
// a struct we flatten for JS. Durations are reported in milliseconds with
// sub-millisecond precision so the values are easy to compare.
func ntpQuery(_ context.Context, call goja.FunctionCall) (*scriptengine.Ordered, error) {
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

	return scriptengine.NewOrdered().
		Set("serverTime", resp.Time.UTC().Format(time.RFC3339Nano)).
		Set("offsetMs", float64(resp.ClockOffset.Microseconds())/1000.0).
		Set("rttMs", float64(resp.RTT.Microseconds())/1000.0).
		Set("stratum", int(resp.Stratum)).
		Set("referenceTime", resp.ReferenceTime.UTC().Format(time.RFC3339Nano)).
		Set("rootDelayMs", float64(resp.RootDelay.Microseconds())/1000.0).
		Set("rootDispersionMs", float64(resp.RootDispersion.Microseconds())/1000.0), nil
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
func whoisLookup(_ context.Context, call goja.FunctionCall) (*scriptengine.Ordered, error) {
	domain := call.Argument(0).String()
	opts := optsAsMap(call)
	timeout := optMillis(opts, "timeout", 10*time.Second)

	client := whois.NewClient().SetTimeout(timeout)
	raw, err := client.Whois(domain)
	if err != nil {
		return nil, err
	}

	out := scriptengine.NewOrdered().Set("raw", raw)
	parsed, parseErr := whoisparser.Parse(raw)
	if parseErr == nil {
		if parsed.Domain != nil {
			// whois-parser puts the whois server on Domain, not on Registrar
			// — it's the server that served the lookup, not a registrar field.
			out.Set("domain", scriptengine.NewOrdered().
				Set("name", parsed.Domain.Name).
				Set("punycode", parsed.Domain.Punycode).
				Set("whoisServer", parsed.Domain.WhoisServer).
				Set("nameServers", parsed.Domain.NameServers).
				Set("status", parsed.Domain.Status).
				Set("dnssec", parsed.Domain.DNSSec).
				Set("createdDate", parsed.Domain.CreatedDate).
				Set("updatedDate", parsed.Domain.UpdatedDate).
				Set("expirationDate", parsed.Domain.ExpirationDate))
		}
		if parsed.Registrar != nil {
			out.Set("registrar", scriptengine.NewOrdered().
				Set("name", parsed.Registrar.Name))
		}
	}
	return out, nil
}
