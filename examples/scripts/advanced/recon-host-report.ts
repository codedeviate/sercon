// Multi-binding host recon report.
//
// Resolves DNS, TCP-connects to port 443, inspects the TLS certificate, and
// fetches HTTP response headers for a target host, then prints a tidy
// structured report.
//
// NETWORK-DEPENDENT — change TARGET below to any host you are authorised to
// probe. This script is excluded from CI and from `make demo` (which runs
// the offline DEMO_SCRIPTS subset).
//
// OFFLINE-SAFE: every probe is wrapped so that if the network is unreachable
// or a call fails, the script logs a skip message and exits 0. It never
// hangs: net.probe.* all accept a timeout option; net.http.request has its
// own timeout; all failures are caught and handled gracefully.

const TARGET = "example.com"; // ← change to your authorised target

// ── helpers ───────────────────────────────────────────────────────────────────

// Pad a label to a fixed width for aligned output.
function lbl(s: string, width = 18): string {
  return s.padEnd(width);
}

// Format an ISO date string as "YYYY-MM-DD" for readability.
function shortDate(iso: string): string {
  return typeof iso === "string" ? iso.slice(0, 10) : String(iso);
}

// ── DNS resolution ────────────────────────────────────────────────────────────
runtime.log("=== Host Recon Report: " + TARGET + " ===");
runtime.log("");

let dnsOk = false;
let resolvedIPs: string[] = [];

try {
  const dns = await net.probe.dns(TARGET, { types: ["a", "aaaa"] });
  const aRecords: string[] = dns.a ?? [];
  const aaaaRecords: string[] = dns.aaaa ?? [];
  resolvedIPs = [...aRecords, ...aaaaRecords];

  runtime.log("── DNS ──────────────────────────────────────────────────────────");
  runtime.log(lbl("A records:"),     aRecords.join(", ") || "(none)");
  runtime.log(lbl("AAAA records:"),  aaaaRecords.join(", ") || "(none)");
  runtime.log(lbl("Resolved IPs:"),  resolvedIPs.length);
  dnsOk = resolvedIPs.length > 0;
} catch (e: any) {
  runtime.log("DNS probe failed — network may be unreachable. Skipping recon demo.");
  runtime.log("(" + String(e).slice(0, 120) + ")");
}

if (!dnsOk) {
  runtime.log("network unreachable — skipping recon demo");
  // Exit 0 cleanly without throwing.
  // (Top-level await + no uncaught rejection means we just fall off the end.)
} else {

// ── TCP connectivity ──────────────────────────────────────────────────────────
runtime.log("");
runtime.log("── TCP ──────────────────────────────────────────────────────────");

let tcpOk = false;
let tcpLatency = "";
try {
  const tcp = await net.probe.tcp(TARGET + ":443", { timeout: 5000 });
  tcpOk = true;
  tcpLatency = tcp.latencyMs.toFixed(1) + " ms";
  runtime.log(lbl("Port 443:"),   "OPEN");
  runtime.log(lbl("Resolved IP:"), tcp.ip);
  runtime.log(lbl("Latency:"),    tcpLatency);
} catch (e: any) {
  runtime.log(lbl("Port 443:"), "CLOSED or unreachable (" + String(e).slice(0, 80) + ")");
}

// ── TLS certificate ───────────────────────────────────────────────────────────
runtime.log("");
runtime.log("── TLS Certificate ──────────────────────────────────────────────");

if (tcpOk) {
  try {
    const tls = await net.probe.tls(TARGET, { timeout: 8000 });

    const expiryClass = tls.daysRemaining > 30
      ? "OK"
      : tls.daysRemaining > 7
        ? "WARNING"
        : "CRITICAL";

    runtime.log(lbl("CN:"),            tls.cn);
    runtime.log(lbl("Issuer:"),        tls.issuer);
    runtime.log(lbl("Not after:"),     shortDate(tls.notAfter));
    runtime.log(lbl("Days remaining:"), tls.daysRemaining + "  [" + expiryClass + "]");
    runtime.log(lbl("DNS SANs:"),      tls.dnsNames.slice(0, 5).join(", ") +
                                        (tls.dnsNames.length > 5 ? " …" : ""));
    runtime.log(lbl("Fingerprint:"),   tls.fingerprintSha256.slice(0, 40) + "…");
  } catch (e: any) {
    runtime.log("TLS probe failed: " + String(e).slice(0, 100));
  }
} else {
  runtime.log("(skipped — TCP port 443 not reachable)");
}

// ── HTTP headers ──────────────────────────────────────────────────────────────
runtime.log("");
runtime.log("── HTTP Headers ─────────────────────────────────────────────────");

try {
  const resp = await net.http.request("GET", "https://" + TARGET + "/", {
    headers: { "User-Agent": "sercon-recon/1.0" },
    follow: true,
    timeout: 10000,
  });

  // Interesting headers to surface (case-insensitive lookup)
  const interesting = [
    "server", "x-powered-by", "content-type", "cache-control",
    "strict-transport-security", "x-frame-options", "x-content-type-options",
    "content-security-policy",
  ];

  runtime.log(lbl("HTTP status:"), resp.status, resp.ok ? "OK" : "");

  const headers: Record<string, string[]> = resp.headers ?? {};
  for (const key of interesting) {
    // Header map from net.http.request uses original case; search case-insensitively.
    const match = Object.entries(headers).find(([k]) => k.toLowerCase() === key);
    if (match) {
      const val = Array.isArray(match[1]) ? match[1].join("; ") : String(match[1]);
      runtime.log(lbl(match[0] + ":"), val.slice(0, 80));
    }
  }
} catch (e: any) {
  runtime.log("HTTP request failed: " + String(e).slice(0, 100));
}

// ── summary ───────────────────────────────────────────────────────────────────
runtime.log("");
runtime.log("── Summary ──────────────────────────────────────────────────────");
runtime.log(lbl("Target:"),      TARGET);
runtime.log(lbl("IPs found:"),   resolvedIPs.join(", ") || "(none)");
runtime.log(lbl("TCP 443:"),     tcpOk ? "OPEN (" + tcpLatency + ")" : "unreachable");
runtime.log("");
runtime.log("=== recon complete ===");

} // end if (dnsOk)
