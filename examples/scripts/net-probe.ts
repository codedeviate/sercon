// Demonstrates api.net.* — the stdlib-backed protocol probes. All three
// return Promises; await them. They hit the real network so this script is
// excluded from the CI matrix (see Makefile: DEMO_SCRIPTS is the local set,
// CI invokes its own offline subset).

api.log("=== api.net.tcp ===");
const t = await api.net.tcp("example.com:443");
api.log("connected:", t.host, t.ip, "in", t.latencyMs.toFixed(1), "ms");

api.log("=== api.net.dns ===");
const d = await api.net.dns("example.com", { types: ["a", "aaaa", "mx", "txt"] });
api.log("a:    ", JSON.stringify(d.a));
api.log("aaaa: ", JSON.stringify(d.aaaa));
api.log("mx:   ", JSON.stringify(d.mx));
api.log("txt:  ", JSON.stringify(d.txt));

api.log("=== api.net.tls ===");
const c = await api.net.tls("example.com");
api.log("cn:           ", c.cn);
api.log("issuer:       ", c.issuer);
api.log("not after:    ", c.notAfter);
api.log("days left:    ", c.daysRemaining);
api.log("dns names:    ", c.dnsNames.join(", "));
api.log("sha256:       ", c.fingerprintSha256);

api.log("=== api.net.ntp ===");
const n = await api.net.ntp("pool.ntp.org", { timeout: 3000 });
api.log("server time:  ", n.serverTime);
api.log("offset:       ", n.offsetMs.toFixed(3), "ms");
api.log("rtt:          ", n.rttMs.toFixed(3), "ms");
api.log("stratum:      ", n.stratum);

api.log("=== api.net.whois ===");
const w = await api.net.whois("example.com");
if (w.domain) {
  api.log("name:         ", w.domain.name);
  api.log("whois server: ", w.domain.whoisServer);
  api.log("name servers: ", (w.domain.nameServers ?? []).join(", "));
  api.log("expires:      ", w.domain.expirationDate);
}
if (w.registrar) {
  api.log("registrar:    ", w.registrar.name);
}
