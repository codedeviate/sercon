// Demonstrates net.* — the stdlib-backed protocol probes. All three
// return Promises; await them. They hit the real network so this script is
// excluded from the CI matrix (see Makefile: DEMO_SCRIPTS is the local set,
// CI invokes its own offline subset).

runtime.log("=== net.probe.tcp ===");
const t = await net.probe.tcp("example.com:443");
runtime.log("connected:", t.host, t.ip, "in", t.latencyMs.toFixed(1), "ms");

runtime.log("=== net.probe.dns ===");
const d = await net.probe.dns("example.com", { types: ["a", "aaaa", "mx", "txt"] });
runtime.log("a:    ", JSON.stringify(d.a));
runtime.log("aaaa: ", JSON.stringify(d.aaaa));
runtime.log("mx:   ", JSON.stringify(d.mx));
runtime.log("txt:  ", JSON.stringify(d.txt));

runtime.log("=== net.probe.tls ===");
const c = await net.probe.tls("example.com");
runtime.log("cn:           ", c.cn);
runtime.log("issuer:       ", c.issuer);
runtime.log("not after:    ", c.notAfter);
runtime.log("days left:    ", c.daysRemaining);
runtime.log("dns names:    ", c.dnsNames.join(", "));
runtime.log("sha256:       ", c.fingerprintSha256);

runtime.log("=== net.probe.ntp ===");
const n = await net.probe.ntp("pool.ntp.org", { timeout: 3000 });
runtime.log("server time:  ", n.serverTime);
runtime.log("offset:       ", n.offsetMs.toFixed(3), "ms");
runtime.log("rtt:          ", n.rttMs.toFixed(3), "ms");
runtime.log("stratum:      ", n.stratum);

runtime.log("=== net.probe.whois ===");
const w = await net.probe.whois("example.com");
if (w.domain) {
  runtime.log("name:         ", w.domain.name);
  runtime.log("whois server: ", w.domain.whoisServer);
  runtime.log("name servers: ", (w.domain.nameServers ?? []).join(", "));
  runtime.log("expires:      ", w.domain.expirationDate);
}
if (w.registrar) {
  runtime.log("registrar:    ", w.registrar.name);
}

runtime.log("");
runtime.log("=== net.probe.ping (TCP mode) ===");
const ping = await net.probe.ping("github.com", { mode: "tcp", port: "443", count: 3 });
runtime.log(`  ${ping.host} (${ping.ip}): ${ping.received}/${ping.sent} received, ${ping.lossPercent}% loss, avg ${ping.avgMs.toFixed(1)}ms`);
runtime.log("  (mode 'icmp' available too — needs raw-socket privileges)");

runtime.log("");
runtime.log("=== net.probe.smtp (capability probe) ===");
try {
  const smtp = await net.probe.smtp("smtp.gmail.com", { port: "587", timeout: 5000 });
  runtime.log("  banner:", smtp.banner.slice(0, 50));
  runtime.log("  STARTTLS:", smtp.starttls, " AUTH:", smtp.authMechanisms.join(", ") || "(none advertised pre-TLS)");
  runtime.log("  SIZE limit:", smtp.sizeLimit, " extensions:", smtp.extensions.length);
} catch (e) {
  runtime.log("  smtp probe skipped:", String(e).slice(0, 60));
}


runtime.log("");
runtime.log("=== net.netstatus.check (aggregate probe) ===");
const status = await net.netstatus.check("github.com");
runtime.log(`  reachable: ${status.reachable}  (${status.elapsedMs.toFixed(0)}ms)`);
runtime.log(`  dns: ${status.dns.ok}  tcp: ${status.tcp.ok}  tls: ${status.tls.ok} (${status.tls.daysRemaining}d)  http: ${status.http.status}`);
