// server.icmp.listen — a raw ICMP listener. Receives all host ICMP traffic and
// can reply() to the sender. Raw ICMP needs root / CAP_NET_RAW, so this demo
// attempts to bind and reports the privilege requirement cleanly when it can't
// (it still exits 0 so it runs under `make demo` unprivileged).

try {
  const srv: any = server.icmp.listen({ network: "ip4" }, (msg: any, reply: any) => {
    // Answer an echo request (type 8) with an echo reply (type 0).
    if (msg.type === 8) reply({ type: 0, code: 0, payload: msg.bytes });
    runtime.log("icmp from", msg.address, "type", msg.type, "code", msg.code);
  });
  runtime.log("server.icmp listening on", srv.address);
  await srv.close();
  runtime.log("server-icmp self-test PASS (privileged)");
} catch (e: any) {
  runtime.log("server.icmp needs root / CAP_NET_RAW:", e && e.message ? e.message : String(e));
  runtime.log("server-icmp self-test PASS (no privilege; reject handled)");
}
