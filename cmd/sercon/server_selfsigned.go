package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"time"
)

// server_selfsigned.go backs the `cert: "self-signed"` convenience for
// server.https.listen — an in-process, ephemeral self-signed certificate so a
// local HTTPS dev server needs no openssl step and no committed PEM. The key
// lives only for the life of the Run; nothing is written to disk. Self-signed
// certs fail browser/most-client verification by design — this is a dev-only
// convenience, not a production cert path (own the cert in your supervisor for
// real deployments). Pure stdlib crypto; no cgo.

// generateSelfSignedCert mints a fresh P-256 ECDSA self-signed certificate
// valid for the given SAN hosts (IPs go in IPAddresses, names in DNSNames),
// usable for ~1 year. The keypair is generated per call and never persisted.
func generateSelfSignedCert(hosts []string) (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	now := time.Now()
	tmpl := x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{Organization: []string{"sercon self-signed"}, CommonName: "localhost"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	return tls.X509KeyPair(certPEM, keyPEM)
}

// selfSignedHosts builds the SAN list for a self-signed cert from the listen
// host. Loopback names/addresses are always included so the common
// `https://localhost:PORT` / `https://127.0.0.1:PORT` dev case verifies
// against the cert's SANs. A wildcard bind ("0.0.0.0" / "::") contributes no
// extra SAN beyond loopback.
func selfSignedHosts(listenHost string) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if listenHost == "" || listenHost == "0.0.0.0" || listenHost == "::" {
		return hosts
	}
	for _, h := range hosts {
		if h == listenHost {
			return hosts
		}
	}
	return append(hosts, listenHost)
}
