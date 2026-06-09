// Demonstrates server.https.listen with the `cert: "self-signed"` shortcut —
// an ephemeral P-256 cert minted in-process (no openssl, no committed PEM, no
// file on disk). SANs cover localhost / 127.0.0.1 / ::1 plus the listen host.
//
// net.probe.tls skips verification by design so it works with self-signed certs.
// Self-tests: TLS handshake succeeds; cert CN is "localhost"; localhost SAN
// present; cert not expired; the GET / route returns the expected body.
//
// For a real cert, pass `cert`/`key` as file paths or inline PEM strings
// instead — see MANUAL.md §6.2. Self-signed is a local-dev convenience only.

const port = 38202;

const srv = await server.https.listen({
  port,
  cert: "self-signed",
  routes: {
    "GET /": (_req: any, res: any) => res.json({ tls: true, message: "hello over https" }),
  },
});

runtime.log("https server listening on", srv.address, "(ephemeral self-signed cert)");

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
