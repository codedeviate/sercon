// Packet capture file round-trip + traffic analysis.
//
// Fully OFFLINE — builds raw Ethernet/IPv4 frames by hand, writes a .pcap
// with net.capture.toFile, reads it back with net.capture.openFile, then
// computes per-protocol counts and a "top destination ports" tally.
//
// Frame layout (same link type as capture-file.ts — Ethernet default):
//   [Ethernet 14B: dst-MAC(6) src-MAC(6) EtherType=0x0800(2)] [IPv4 20B] [proto hdr] [payload]
//
// We build five frames:
//   frame 0 → UDP  dst=9999   payload "hi"
//   frame 1 → UDP  dst=9999   payload "there"
//   frame 2 → UDP  dst=8080   payload "x"
//   frame 3 → TCP  dst=443    SYN
//   frame 4 → TCP  dst=443    SYN
//
// Expected analysis result:
//   protocol:  udp=3, tcp=2
//   top port:  9999 → 2 packets, 443 → 2 packets, 8080 → 1 packet
//
// Live capture (net.capture.open) needs root / CAP_NET_RAW (Linux) or
// /dev/bpf access (macOS). See the commented reference at the bottom.

// Shared Ethernet header bytes (dst=02:00:00:00:00:02, src=02:00:00:00:00:01, type=IPv4)
const ETH: number[] = [0x02, 0x00, 0x00, 0x00, 0x00, 0x02,  // dst MAC
                        0x02, 0x00, 0x00, 0x00, 0x00, 0x01,  // src MAC
                        0x08, 0x00];                           // EtherType = IPv4

// Helper: build an Ethernet/IPv4/UDP frame.
// srcPort, dstPort as numbers; payload as number[].
// IPv4 total-length = 20 (IP hdr) + 8 (UDP hdr) + payload.length
// IP identification increments per frame to keep pcap happy.
function makeUDPFrame(id: number, srcPort: number, dstPort: number, payload: number[]): Uint8Array {
  const ipLen = 20 + 8 + payload.length;
  const udpLen = 8 + payload.length;
  const frame: number[] = [
    ...ETH,
    // IPv4 header (checksum = 0 — gopacket doesn't verify)
    0x45, 0x00,                                                 // version+IHL, DSCP
    (ipLen >> 8) & 0xff, ipLen & 0xff,                          // total length
    (id >> 8) & 0xff, id & 0xff, 0x00, 0x00,                   // id, flags+frag
    0x40, 0x11, 0x00, 0x00,                                     // TTL=64, proto=UDP(17), cksum=0
    0x7f, 0x00, 0x00, 0x01,                                     // src=127.0.0.1
    0x7f, 0x00, 0x00, 0x01,                                     // dst=127.0.0.1
    // UDP header
    (srcPort >> 8) & 0xff, srcPort & 0xff,                      // src port
    (dstPort >> 8) & 0xff, dstPort & 0xff,                      // dst port
    (udpLen >> 8) & 0xff, udpLen & 0xff,                        // length
    0x00, 0x00,                                                  // checksum=0
    ...payload,
  ];
  return new Uint8Array(frame);
}

// Helper: build an Ethernet/IPv4/TCP SYN frame.
function makeTCPFrame(id: number, srcPort: number, dstPort: number): Uint8Array {
  const ipLen = 40; // 20 IP + 20 TCP (data-offset=5)
  const frame: number[] = [
    ...ETH,
    // IPv4 header
    0x45, 0x00,
    (ipLen >> 8) & 0xff, ipLen & 0xff,
    (id >> 8) & 0xff, id & 0xff, 0x00, 0x00,
    0x40, 0x06, 0x00, 0x00,                                     // TTL=64, proto=TCP(6)
    0x7f, 0x00, 0x00, 0x01,
    0x7f, 0x00, 0x00, 0x01,
    // TCP header (20 bytes, flags=SYN)
    (srcPort >> 8) & 0xff, srcPort & 0xff,
    (dstPort >> 8) & 0xff, dstPort & 0xff,
    0x00, 0x00, 0x00, 0x00,                                     // seq
    0x00, 0x00, 0x00, 0x00,                                     // ack
    0x50, 0x02,                                                  // data-offset=5, flags=SYN
    0xff, 0xff,                                                  // window
    0x00, 0x00, 0x00, 0x00,                                     // checksum, urgent
  ];
  return new Uint8Array(frame);
}

const frames = [
  makeUDPFrame(1, 0x10_92, 9999, [0x68, 0x69]),           // UDP dst=9999 "hi"
  makeUDPFrame(2, 0x10_93, 9999, [0x74, 0x68, 0x65, 0x72, 0x65]), // UDP dst=9999 "there"
  makeUDPFrame(3, 0x10_94, 8080, [0x78]),                  // UDP dst=8080 "x"
  makeTCPFrame(4, 0x04_d2, 443),                           // TCP dst=443 SYN src=1234
  makeTCPFrame(5, 0x04_d3, 443),                           // TCP dst=443 SYN src=1235
];
const TOTAL = frames.length; // 5

// ── write pcap ────────────────────────────────────────────────────────────────
const pcapPath = "/tmp/sercon-packet-analysis.pcap";
const w = net.capture.toFile(pcapPath, { snaplen: 65536 });
let ts = 1700000000000;
for (const frame of frames) {
  w.write(frame, { ts });
  ts += 1;
}
await w.close();
runtime.log("wrote", TOTAL, "frames to", pcapPath);

// ── read back + analyse ───────────────────────────────────────────────────────
let totalSeen = 0;
const protoCounts: Record<string, number> = {};
const dstPortCounts: Record<number, number> = {};

await net.capture.openFile(pcapPath, (pkt: any) => {
  totalSeen++;

  // Determine transport-layer protocol
  const proto = pkt.tcp ? "tcp" : pkt.udp ? "udp" : "other";
  protoCounts[proto] = (protoCounts[proto] ?? 0) + 1;

  // Track destination port
  const dstPort: number | undefined = pkt.tcp?.dstPort ?? pkt.udp?.dstPort;
  if (dstPort !== undefined) {
    dstPortCounts[dstPort] = (dstPortCounts[dstPort] ?? 0) + 1;
  }
});

runtime.log("--- Analysis report ---");
runtime.log("total packets :", totalSeen);

runtime.log("protocol breakdown:");
for (const [proto, count] of Object.entries(protoCounts).sort()) {
  runtime.log(" ", proto.padEnd(6), count);
}

// Top destination ports sorted by count descending
const topPorts = Object.entries(dstPortCounts)
  .sort(([, a], [, b]) => b - a);

runtime.log("top destination ports:");
for (const [port, count] of topPorts) {
  runtime.log("  port", String(port).padStart(5), ":", count, "packet(s)");
}

// ── assertions ────────────────────────────────────────────────────────────────
runtime.assert.equal(totalSeen,           TOTAL, `expected ${TOTAL} packets total`);
runtime.assert.equal(protoCounts["udp"],  3,     "expected 3 UDP packets");
runtime.assert.equal(protoCounts["tcp"],  2,     "expected 2 TCP packets");
runtime.assert.equal(dstPortCounts[9999], 2,     "port 9999 should have 2 packets");
runtime.assert.equal(dstPortCounts[8080], 1,     "port 8080 should have 1 packet");
runtime.assert.equal(dstPortCounts[443],  2,     "port 443 should have 2 packets");

// Top port by volume: both 9999 and 443 have count 2; just confirm winner >= 2.
const topCount = topPorts[0]?.[1] ?? 0;
runtime.assert.ok(topCount >= 2, "top port count should be >= 2");

runtime.log("--- all assertions satisfied ---");

// ── filter check ─────────────────────────────────────────────────────────────
// Re-read with a "udp" filter: only UDP frames should be dispatched.
let filteredUdp = 0;
let filteredTcp = 0;
await net.capture.openFile(pcapPath, (pkt: any) => {
  if (pkt.udp) filteredUdp++;
  if (pkt.tcp) filteredTcp++;
}, { filter: "udp" });

runtime.assert.equal(filteredUdp, 3, "filter 'udp' should pass 3 frames");
runtime.assert.equal(filteredTcp, 0, "filter 'udp' should drop TCP frames");
runtime.log("filter check: 'udp' passed", filteredUdp, "UDP frames, blocked", filteredTcp, "TCP frames");

runtime.log("PASS");

// ── live capture reference (NOT run here) ────────────────────────────────────
// To capture live traffic instead of a file, use net.capture.open — it needs
// root / CAP_NET_RAW (Linux) or /dev/bpf access (macOS):
//
//   const cap = await net.capture.open({ iface: "eth0", promisc: true,
//                                         snaplen: 65536, filter: "tcp port 443" },
//                                       (pkt: any) => {
//     runtime.log("live:", pkt.ip?.src, "->", pkt.ip?.dst,
//                 pkt.tcp ? "tcp:" + pkt.tcp.dstPort : "");
//   });
//   setTimeout(() => cap.close(), 5000);
