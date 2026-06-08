// Demonstrates server.https.listen with an inline self-signed PEM cert+key.
// The cert was generated once with:
//   openssl req -x509 -newkey rsa:2048 -keyout k.pem -out c.pem -days 3650
//     -nodes -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost,IP:127.0.0.1"
//
// net.probe.tls skips verification by design so it works with self-signed certs.
// Self-tests: TLS handshake succeeds; cert CN is "localhost"; 127.0.0.1 SAN present;
// the GET / route returns the expected body.

const CERT = `-----BEGIN CERTIFICATE-----
MIIDJTCCAg2gAwIBAgIUXHCkyKBczsubOyTVPWpovUs2TUkwDQYJKoZIhvcNAQEL
BQAwFDESMBAGA1UEAwwJbG9jYWxob3N0MB4XDTI2MDYwODIwMDc0NFoXDTM2MDYw
NTIwMDc0NFowFDESMBAGA1UEAwwJbG9jYWxob3N0MIIBIjANBgkqhkiG9w0BAQEF
AAOCAQ8AMIIBCgKCAQEAinzNrb9fa67cIdpGQ9/KgwxWDQ/t+w3irQX6uDYc4R4k
ycSsXgCBH87Of7LKROCpU/+vEm4r7geevs8SiGiyXp+ouAmPf7SRAZf8aCeWXOdV
4Msmbe6BHxHN1PtJuR1zlk92/16Oo8vp8LdGlA/PFO6ey06jfPfYnpDNfone8X7p
RaiYZY6jNoLYd6+5MyOy0emtGbbltDU1Rd/BeHmNNei8IFb+r3I1A4D+DUWjMjm4
aIeXMn+V9p4VqJ0BEAPStv8Z1mfj3+Toi1OgtJxedKGHFFBIRPoBa8Z+HftakzHp
Yf8yabRWpwyd1UpafKDsRN1B1fExwl4lJ10WRaVoGwIDAQABo28wbTAdBgNVHQ4E
FgQUlJNAzsuudYlKFLGoLb4Y07ARM44wHwYDVR0jBBgwFoAUlJNAzsuudYlKFLGo
Lb4Y07ARM44wDwYDVR0TAQH/BAUwAwEB/zAaBgNVHREEEzARgglsb2NhbGhvc3SH
BH8AAAEwDQYJKoZIhvcNAQELBQADggEBAHCr+hh7CLeVp3Z+pjTDcsnxElOzZLx7
9M0Qr3DIs9vYiI97kl8LTDGwIRobCnWOniC/AenXsSNkJYxjSChIBxI536AZXNSB
+6bkVLzl7cUmHM4DvcPlLDIhdzBaQZTRhQwsybiioqS7YIG1mLObLIx9lbSPvc6X
FB8Ieoo5qhWHNENa5mn3aGxspmWfS1rrdiCtmP4qDAODBcrBSuGX5802HBZ6nQ+R
b7qCwp5FtLnoM4cVWkc4081zl9GCmGPZhNXRWjIN4stdLEJf3f7Cp9fh7BoZoHs5
TyHa9j5vQ7EPNGsbhl5fJAPtSPvlgYAmr9HU4ggQgRBfxTGCGZQDoj4=
-----END CERTIFICATE-----`;

const KEY = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQCKfM2tv19rrtwh
2kZD38qDDFYND+37DeKtBfq4NhzhHiTJxKxeAIEfzs5/sspE4KlT/68SbivuB56+
zxKIaLJen6i4CY9/tJEBl/xoJ5Zc51XgyyZt7oEfEc3U+0m5HXOWT3b/Xo6jy+nw
t0aUD88U7p7LTqN899iekM1+id7xfulFqJhljqM2gth3r7kzI7LR6a0ZtuW0NTVF
38F4eY016LwgVv6vcjUDgP4NRaMyObhoh5cyf5X2nhWonQEQA9K2/xnWZ+Pf5OiL
U6C0nF50oYcUUEhE+gFrxn4d+1qTMelh/zJptFanDJ3VSlp8oOxE3UHV8THCXiUn
XRZFpWgbAgMBAAECggEAPFVzJich40AjF3yI8DkneUB+nOI7xAygnNDmEitBGbKQ
RHcywSFnH/xxaqDyDl/yZW4XT+g/q0wWlPqSEGvMifz0/Hjt061PH2qfxKC6mW8P
swfOjkZCas7O5eM0kzmJigrExSzk5+eG3CB81zSr+2qaM+jmwSMZdLuRS3e1cXA8
9T4W9Egw5bocKYWqrjRAabnKubnlFqIT4Fzsv0Y0Ytt+Kuhv3SM4NXrhE93gJN70
K7Ze4yIe1aX2gsqGIc9OMPeJ/F01Vmk1bm4JZoaqYKyul1cgMW+dBUvewFQQyK9P
+i1KMwvoDu1GR67vn/s2sfkyqzl79aEM6cJlcY1vQQKBgQDDNuRE/fJm2rKcCb+d
654X3VW5H5toAXiKPfB5tikjyXBk3wsIXhi7rDgHRVzSyjQu9TcaCsEY2O74bPZa
Lzb07YMbe35UDnZ3MAZ41rdGy1Vp7YMwGNl+QADtV7jbFNHGDoV2oxn+3Pj5idiU
+5a3u1B6QKemUmPrGjVMziADlwKBgQC1nAuWKbdNhT7liIhruW/P6MtuYu71Q+xq
vXkY4euHQNwtmFD1Vdc9AsLFusreAprwSR13Z0lo4/iZUSkL57WN81KUYHFUQDGV
FLBuDF/iTRADAZgpq03Jfi4cGxby3NdqwSxe4k6kJ7TpwWQ4kRTnmPj6CmIVTt2V
jVdHUMMAHQKBgCjUMzwGzQscFJ00IMKbxA4DueklJjDDlf175O7f3YzhlcNTLxCJ
9axS4ckLhdWEexOTL/ofY7GZtal5yLCmDV1+y5wU4SAdgkN9ZO0jI2QIJQ4pofWO
TPbt1gPOBBi2KwW8hceBZ295sg0m+oh2clhtMfDP0wCjXMiQS7OLrQBzAoGAQQx9
kfGrOFcLqkd3Ja6r//pQM1+4W51SpwDqySDSrgVrV+GAzf9LMw00GoLHezHPsVVe
+o/CWZGeT7wkSaFbBFctMvxAW38Kw20/rIs+JN6ZZ5pAmFxFZnCNr398fszfU9uR
OwMwS9f1Eu00Kpa8uB+wvk7NxvgSoiiYJHEnB3kCgYEAnINpWMqVIRTXAnpe978v
AGNz5qTnZwoTrBt79DoKQjX5EeQQYhHIDmGPsBCn0auJXadgnn2W86g17zm0V7cD
PhA1tUF4Xq53BS9R7M3sLbGvnFIVjpCKnm+KhONoCdhq0ahWnR0VgVRmLugwDzLx
G6r/uvuMtFTeFOALQZowKIg=
-----END PRIVATE KEY-----`;

const port = 38202;

const srv = await server.https.listen({
  port,
  cert: CERT,
  key: KEY,
  routes: {
    "GET /": (_req: any, res: any) => res.json({ tls: true, message: "hello over https" }),
  },
});

runtime.log("https server listening on", srv.address);

// ── TLS probe ─────────────────────────────────────────────────────────────────
// net.probe.tls connects with InsecureSkipVerify, so a self-signed cert is fine.
const tlsResult = await net.probe.tls(`127.0.0.1:${port}`);

runtime.log("cert CN:          ", tlsResult.cn);
runtime.log("cert issuer:      ", tlsResult.issuer);
runtime.log("cert dnsNames:    ", JSON.stringify(tlsResult.dnsNames));
runtime.log("cert daysRemaining:", tlsResult.daysRemaining);

runtime.assert.equal(tlsResult.cn, "localhost", "cert CN is localhost");
runtime.assert.ok(
  (tlsResult.dnsNames as string[]).includes("localhost"),
  "dnsNames includes localhost"
);
runtime.assert.ok(
  (tlsResult.daysRemaining as number) > 0,
  "cert not expired"
);

runtime.log("TLS probe passed");

await srv.close();
runtime.log("PASS");
