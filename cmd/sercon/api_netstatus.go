package main

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// netstatusNamespace wires `api.netstatus.*`. The single member,
// `check`, is an orchestration layer over the lower-level probes: it
// runs DNS / TCP / TLS / HTTP against one host concurrently and folds
// the results into a single status object with an overall `reachable`
// verdict. No new library — it composes net / crypto/tls / net/http
// directly and fans out with a WaitGroup.
func netstatusNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"check": scriptengine.PromisifyAsync(vm, loop, netstatusCheck),
	}
}

// netstatusCheck runs the four sub-probes concurrently against `host`
// and returns:
//
//	{
//	  host, port, elapsedMs, reachable,
//	  dns:  { ok, ips, error? },
//	  tcp:  { ok, latencyMs, error? },
//	  tls:  { ok, daysRemaining, error? },
//	  http: { ok, status, error? },
//	}
//
// `reachable` is the AND of dns.ok and tcp.ok — name resolves and a
// connection opens. TLS / HTTP are reported but don't gate
// reachability (a plain-HTTP host or one with an expired cert is
// still "reachable"). Each sub-probe captures its own error string
// rather than failing the whole call — the point is a status
// snapshot, so individual failures are data. Only a missing host
// argument throws.
func netstatusCheck(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	host := call.Argument(0).String()
	if host == "" {
		return nil, errors.New("netstatus.check: host required")
	}
	opts := optsAsMap(call)
	port := optString(opts, "port", "443")
	timeout := optMillis(opts, "timeout", 10*time.Second)

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	var (
		wg              sync.WaitGroup
		dnsRes, tcpRes  map[string]any
		tlsRes, httpRes map[string]any
	)

	wg.Add(4)
	go func() { defer wg.Done(); dnsRes = nsDNS(probeCtx, host) }()
	go func() { defer wg.Done(); tcpRes = nsTCP(probeCtx, host, port) }()
	go func() { defer wg.Done(); tlsRes = nsTLS(probeCtx, host, port) }()
	go func() { defer wg.Done(); httpRes = nsHTTP(probeCtx, host) }()
	wg.Wait()

	dnsOK, _ := dnsRes["ok"].(bool)
	tcpOK, _ := tcpRes["ok"].(bool)
	reachable := dnsOK && tcpOK
	return map[string]any{
		"host":      host,
		"port":      port,
		"elapsedMs": float64(time.Since(start)) / float64(time.Millisecond),
		"reachable": reachable,
		"dns":       dnsRes,
		"tcp":       tcpRes,
		"tls":       tlsRes,
		"http":      httpRes,
	}, nil
}

func nsDNS(ctx context.Context, host string) map[string]any {
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return map[string]any{"ok": false, "ips": []string{}, "error": err.Error()}
	}
	out := make([]string, 0, len(ips))
	for _, ip := range ips {
		out = append(out, ip.IP.String())
	}
	return map[string]any{"ok": true, "ips": out}
}

func nsTCP(ctx context.Context, host, port string) map[string]any {
	dialer := net.Dialer{}
	start := time.Now()
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return map[string]any{"ok": false, "latencyMs": 0.0, "error": err.Error()}
	}
	_ = conn.Close()
	return map[string]any{"ok": true, "latencyMs": float64(time.Since(start)) / float64(time.Millisecond)}
}

func nsTLS(ctx context.Context, host, port string) map[string]any {
	dialer := tls.Dialer{Config: &tls.Config{ServerName: host}} //nolint:gosec // verified handshake; ServerName set
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return map[string]any{"ok": false, "daysRemaining": 0, "error": err.Error()}
	}
	defer func() { _ = conn.Close() }()
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return map[string]any{"ok": false, "daysRemaining": 0, "error": "not a TLS connection"}
	}
	certs := tlsConn.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return map[string]any{"ok": true, "daysRemaining": 0}
	}
	days := int(time.Until(certs[0].NotAfter).Hours() / 24)
	return map[string]any{"ok": true, "daysRemaining": days}
}

func nsHTTP(ctx context.Context, host string) map[string]any {
	url := "https://" + host
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return map[string]any{"ok": false, "status": 0, "error": err.Error()}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]any{"ok": false, "status": 0, "error": err.Error()}
	}
	_ = resp.Body.Close()
	return map[string]any{
		"ok":     resp.StatusCode >= 200 && resp.StatusCode < 500,
		"status": resp.StatusCode,
	}
}
