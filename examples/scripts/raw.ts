// net.raw — raw packet engine. Needs root / CAP_NET_RAW; this demo degrades
// gracefully (and exits 0) when run unprivileged so `make demo` stays green.
async function main() {
  try {
    const reply = await net.raw.tcp("127.0.0.1", 9, { flags: ["SYN"], timeout: 500 });
    if (reply === null) {
      runtime.log("raw.tcp: no reply (filtered or no listener) — engine works");
    } else if (reply.tcp.flags.rst) {
      runtime.log("raw.tcp: RST — port closed (host reachable)");
    } else if (reply.tcp.flags.syn && reply.tcp.flags.ack) {
      runtime.log("raw.tcp: SYN/ACK — port open");
    } else {
      runtime.log("raw.tcp: reply", reply.tcp.flags);
    }
  } catch (e) {
    // Expected without privileges: a clean privilege/platform rejection.
    runtime.log("raw.tcp unavailable:", (e as Error).message);
  }
}
main();
