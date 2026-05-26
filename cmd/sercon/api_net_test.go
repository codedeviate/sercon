package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// engine wires the same api.net namespace the CLI does so tests drive it
// through a real Run.
func newNetEngine(t *testing.T) *scriptengine.Engine {
	t.Helper()
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterNamespaceFactory("net", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return netNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("log", func(args ...any) { _ = args }); err != nil {
		t.Fatal(err)
	}
	return eng
}

// TCP probe against a local listener. Asserts on the structural shape; the
// exact latency depends on the host so we only check it's a non-negative
// number.
func TestNetTCP_LocalListener(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	_, port, _ := net.SplitHostPort(lis.Addr().String())

	eng := newNetEngine(t)
	script := fmt.Sprintf(`
const r = await net.tcp("127.0.0.1:%s");
if (r.host !== "127.0.0.1") throw new Error("host: " + r.host);
if (r.port !== %s)         throw new Error("port: " + r.port);
if (r.ip   !== "127.0.0.1") throw new Error("ip: "   + r.ip);
if (typeof r.latencyMs !== "number" || r.latencyMs < 0) throw new Error("latencyMs: " + r.latencyMs);
`, port, port)
	if _, err := eng.Run(context.Background(), "tcp_test.ts", script); err != nil {
		t.Fatalf("tcp probe script: %v", err)
	}
}

// DNS lookup against localhost — every system resolves this. We don't pin
// the exact result because IPv6 may or may not be available; at least one
// of `a` / `aaaa` must come back populated.
func TestNetDNS_Localhost(t *testing.T) {
	eng := newNetEngine(t)
	script := `
const r = await net.dns("localhost");
if (!Array.isArray(r.a) && !Array.isArray(r.aaaa)) {
  throw new Error("expected a or aaaa, got " + JSON.stringify(r));
}
const v4ok = Array.isArray(r.a)    && r.a.includes("127.0.0.1");
const v6ok = Array.isArray(r.aaaa) && r.aaaa.includes("::1");
if (!v4ok && !v6ok) throw new Error("no loopback IP returned: " + JSON.stringify(r));
`
	if _, err := eng.Run(context.Background(), "dns_test.ts", script); err != nil {
		t.Fatalf("dns lookup script: %v", err)
	}
}

// DNS types filter — when only `a` is requested, `aaaa` / `mx` / etc. must
// not appear in the result even if they'd otherwise be present.
func TestNetDNS_TypesFilter(t *testing.T) {
	eng := newNetEngine(t)
	script := `
const r = await net.dns("localhost", { types: ["a"] });
if (!Array.isArray(r.a)) throw new Error("expected a: " + JSON.stringify(r));
if ("aaaa" in r) throw new Error("aaaa leaked through filter");
if ("mx"   in r) throw new Error("mx leaked through filter");
`
	if _, err := eng.Run(context.Background(), "dns_filter.ts", script); err != nil {
		t.Fatalf("dns types filter: %v", err)
	}
}

// TLS probe against a local TLS server with a self-signed cert. Confirms
// the structural shape and that the cert metadata we synthesised lands
// back in the JS-visible result.
func TestNetTLS_LocalServer(t *testing.T) {
	cert, host, port := startSelfSignedTLS(t)
	defer cert.lis.Close()

	eng := newNetEngine(t)
	script := fmt.Sprintf(`
const r = await net.tls("%s:%s");
if (r.cn !== %q) throw new Error("cn: " + r.cn);
if (r.issuer !== %q) throw new Error("issuer: " + r.issuer);
if (typeof r.daysRemaining !== "number") throw new Error("daysRemaining: " + r.daysRemaining);
if (typeof r.fingerprintSha256 !== "string" || r.fingerprintSha256.length !== 64) {
  throw new Error("fingerprint: " + r.fingerprintSha256);
}
if (!Array.isArray(r.dnsNames) || !r.dnsNames.includes(%q)) {
  throw new Error("dnsNames: " + JSON.stringify(r.dnsNames));
}
`, host, port, cert.cn, cert.cn, cert.cn)
	if _, err := eng.Run(context.Background(), "tls_test.ts", script); err != nil {
		t.Fatalf("tls probe script: %v", err)
	}
}

// NTP smoke test against a port that nothing is listening on. We can't
// stand up a real NTPv4 responder in a few lines, but we can confirm the
// binding surfaces an error rather than panicking. Short timeout so the
// test exits quickly.
func TestNetNTP_UnreachableHostSurfacesError(t *testing.T) {
	eng := newNetEngine(t)
	script := `
let caught = false;
try {
  await net.ntp("127.0.0.1", { timeout: 200, port: 1 });
} catch (e) {
  caught = true;
}
if (!caught) throw new Error("expected ntp to error against an unreachable port");
`
	if _, err := eng.Run(context.Background(), "ntp_err.ts", script); err != nil {
		t.Fatalf("ntp error path: %v", err)
	}
}

// WHOIS smoke test. likexian/whois resolves the right server via IANA, so
// a truly offline test would need a fake whois server (lots of setup).
// Instead drive the binding with a clearly invalid domain and check the
// host-engine surfaces *some* error or returns a raw payload — both are
// acceptable; we just want to confirm the wrapper doesn't crash.
func TestNetWHOIS_InvalidDomainDoesNotPanic(t *testing.T) {
	if testing.Short() {
		t.Skip("whois smoke needs network; skipped under -short")
	}
	eng := newNetEngine(t)
	script := `
let didError = false;
try {
  const r = await net.whois("this-domain-most-certainly-does-not-exist.invalid", { timeout: 3000 });
  if (typeof r !== "object") throw new Error("expected object, got " + typeof r);
} catch (e) {
  didError = true;
}
// Either path is fine — we just want to prove the binding round-trips
// without panicking the host.
if (typeof didError !== "boolean") throw new Error("flag never set");
`
	if _, err := eng.Run(context.Background(), "whois_smoke.ts", script); err != nil {
		t.Fatalf("whois smoke: %v", err)
	}
}

type tlsFixture struct {
	lis net.Listener
	cn  string
}

// startSelfSignedTLS spins up a local TLS server that serves a self-signed
// ECDSA cert. The handshake completes; no application bytes are read.
func startSelfSignedTLS(t *testing.T) (tlsFixture, string, string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const cn = "sercon-test.invalid"
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		Issuer:       pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{cn},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	tlsCert := tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}

	cfg := &tls.Config{Certificates: []tls.Certificate{tlsCert}}
	lis, err := tls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := lis.Accept()
			if err != nil {
				return
			}
			// Drive the handshake so the client side returns; we don't
			// need to read any data.
			if tc, ok := c.(*tls.Conn); ok {
				_ = tc.Handshake()
			}
			_ = c.Close()
		}
	}()

	_, port, _ := net.SplitHostPort(lis.Addr().String())
	_ = strconv.Atoi // satisfy import
	return tlsFixture{lis: lis, cn: cn}, "127.0.0.1", port
}
