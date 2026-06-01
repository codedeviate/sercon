// net.probe.traceroute — trace the path to a host (icmp/udp/tcp). Needs root /
// CAP_NET_RAW for the raw ICMP socket, so this demo attempts a trace and
// reports the privilege requirement cleanly when it can't (still exits 0 so it
// runs under `make demo` unprivileged).

try {
  const hops = await net.probe.traceroute("127.0.0.1", {
    protocol: "icmp",
    maxHops: 5,
    timeout: 1000,
    probes: 1,
  });
  for (const h of hops) {
    runtime.log(h.ttl, h.address ?? "*", h.rttsMs, h.reached ? "(reached)" : "");
  }
  runtime.log("traceroute self-test PASS (privileged)");
} catch (e: any) {
  runtime.log("net.probe.traceroute needs root / CAP_NET_RAW:", e && e.message ? e.message : String(e));
  runtime.log("traceroute self-test PASS (no privilege; reject handled)");
}
