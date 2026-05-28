// Demonstrates api.net.* — the stdlib-backed protocol probes. All three
// return Promises; await them. They hit the real network so this script is
// excluded from the CI matrix (see Makefile: DEMO_SCRIPTS is the local set,
// CI invokes its own offline subset).

api.runtime.log("=== api.net.probe.tcp ===");
const t = await api.net.probe.tcp("example.com:443");
api.runtime.log("connected:", t.host, t.ip, "in", t.latencyMs.toFixed(1), "ms");

api.runtime.log("=== api.net.probe.dns ===");
const d = await api.net.probe.dns("example.com", { types: ["a", "aaaa", "mx", "txt"] });
api.runtime.log("a:    ", JSON.stringify(d.a));
api.runtime.log("aaaa: ", JSON.stringify(d.aaaa));
api.runtime.log("mx:   ", JSON.stringify(d.mx));
api.runtime.log("txt:  ", JSON.stringify(d.txt));

api.runtime.log("=== api.net.probe.tls ===");
const c = await api.net.probe.tls("example.com");
api.runtime.log("cn:           ", c.cn);
api.runtime.log("issuer:       ", c.issuer);
api.runtime.log("not after:    ", c.notAfter);
api.runtime.log("days left:    ", c.daysRemaining);
api.runtime.log("dns names:    ", c.dnsNames.join(", "));
api.runtime.log("sha256:       ", c.fingerprintSha256);

api.runtime.log("=== api.net.probe.ntp ===");
const n = await api.net.probe.ntp("pool.ntp.org", { timeout: 3000 });
api.runtime.log("server time:  ", n.serverTime);
api.runtime.log("offset:       ", n.offsetMs.toFixed(3), "ms");
api.runtime.log("rtt:          ", n.rttMs.toFixed(3), "ms");
api.runtime.log("stratum:      ", n.stratum);

api.runtime.log("=== api.net.probe.whois ===");
const w = await api.net.probe.whois("example.com");
if (w.domain) {
  api.runtime.log("name:         ", w.domain.name);
  api.runtime.log("whois server: ", w.domain.whoisServer);
  api.runtime.log("name servers: ", (w.domain.nameServers ?? []).join(", "));
  api.runtime.log("expires:      ", w.domain.expirationDate);
}
if (w.registrar) {
  api.runtime.log("registrar:    ", w.registrar.name);
}

api.runtime.log("");
api.runtime.log("=== api.net.probe.ping (TCP mode) ===");
const ping = await api.net.probe.ping("github.com", { mode: "tcp", port: "443", count: 3 });
api.runtime.log(`  ${ping.host} (${ping.ip}): ${ping.received}/${ping.sent} received, ${ping.lossPercent}% loss, avg ${ping.avgMs.toFixed(1)}ms`);
api.runtime.log("  (mode 'icmp' available too — needs raw-socket privileges)");

api.runtime.log("");
api.runtime.log("=== api.net.probe.smtp (capability probe) ===");
try {
  const smtp = await api.net.probe.smtp("smtp.gmail.com", { port: "587", timeout: 5000 });
  api.runtime.log("  banner:", smtp.banner.slice(0, 50));
  api.runtime.log("  STARTTLS:", smtp.starttls, " AUTH:", smtp.authMechanisms.join(", ") || "(none advertised pre-TLS)");
  api.runtime.log("  SIZE limit:", smtp.sizeLimit, " extensions:", smtp.extensions.length);
} catch (e) {
  api.runtime.log("  smtp probe skipped:", String(e).slice(0, 60));
}


api.runtime.log("");
api.runtime.log("=== api.net.netstatus.check (aggregate probe) ===");
const status = await api.net.netstatus.check("github.com");
api.runtime.log(`  reachable: ${status.reachable}  (${status.elapsedMs.toFixed(0)}ms)`);
api.runtime.log(`  dns: ${status.dns.ok}  tcp: ${status.tcp.ok}  tls: ${status.tls.ok} (${status.tls.daysRemaining}d)  http: ${status.http.status}`);
