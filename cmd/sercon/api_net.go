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

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// netNamespace builds the `api.net.*` member map: `tcp`, `dns`, `tls`. Every
// member returns a Promise (uses `scriptengine.PromisifyAsync` under the
// hood) so scripts can `await api.net.tcp("host:port")`. All bindings honour
// a `timeout` opt in milliseconds; the default is 5s.
func netNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"tcp": scriptengine.PromisifyAsync(vm, loop, tcpProbe),
		"dns": scriptengine.PromisifyAsync(vm, loop, dnsLookup),
		"tls": scriptengine.PromisifyAsync(vm, loop, tlsProbe),
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
