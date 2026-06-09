// Packet capture file API (net.capture.toFile / openFile) — a fully OFFLINE
// round-trip. Live capture (net.capture.open) is privileged (root /
// CAP_NET_RAW on Linux, /dev/bpf on macOS) and Linux/macOS-only, so it is
// NOT exercised here; this demo only touches the pcap file I/O, which needs
// no privileges and runs anywhere.
//
// net.capture.interfaces() is also fully offline — list the host's NICs:
const ifaces: any[] = net.capture.interfaces();
const lo = ifaces.find((i) => i.loopback);
runtime.log("interfaces:", ifaces.length, "loopback:", lo?.name ?? "(none)");

// net.capture.routes() — also offline/unprivileged; the host's IP routing
// table (Linux /proc/net/route, macOS/BSD routing socket). Find the default.
const routes: any[] = net.capture.routes();
const def = routes.find((r) => r.destination === "0.0.0.0/0" || r.destination === "::/0");
runtime.log("routes:", routes.length, "default via:", def ? `${def.gateway} on ${def.interface}` : "(none)");
runtime.assert.ok(Array.isArray(routes) && routes.length > 0, "routes() returns the routing table");

// Two hand-built frames over loopback IPv4: a UDP datagram (dst port 9999,
// payload "hi") and a TCP SYN (dst port 443). (Scripts can't easily
// synthesise frames at runtime, so we embed the raw bytes; gopacket decodes
// them the same as a captured frame would be.)
const udpFrame = new Uint8Array([
  2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0, 69, 0, 0, 30, 0, 0, 0, 0,
  64, 17, 124, 205, 127, 0, 0, 1, 127, 0, 0, 1, 16, 146, 39, 15, 0, 10,
  0, 0, 104, 105,
]);
const tcpFrame = new Uint8Array([
  2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0, 69, 0, 0, 40, 0, 0, 0, 0,
  64, 6, 124, 206, 127, 0, 0, 1, 127, 0, 0, 1, 4, 210, 1, 187, 0, 0,
  0, 0, 0, 0, 0, 0, 80, 2, 255, 255, 0, 0, 0, 0,
]);

const path = "/tmp/sercon-capture-demo.pcap";

// Write both frames into a .pcap file, then close (flush) it.
const w = net.capture.toFile(path, { snaplen: 65536 });
w.write(udpFrame, { ts: 1700000000000 });
w.write(tcpFrame, { ts: 1700000000001 });
await w.close();
runtime.log("wrote 2 frames (udp + tcp) to", path);

// Read it back: openFile decodes every packet and calls the handler, then
// resolves at EOF.
let seen = 0;
let dstPort = -1;
await net.capture.openFile(path, (pkt: any) => {
  seen++;
  runtime.log(
    "packet", seen, "link:", pkt.link,
    "ip:", pkt.ip?.src, "->", pkt.ip?.dst,
    "udp.dstPort:", pkt.udp?.dstPort,
    "tcp.dstPort:", pkt.tcp?.dstPort,
  );
  if (pkt.udp) dstPort = pkt.udp.dstPort;
});

runtime.assert.equal(seen, 2, "expected exactly two packets");
runtime.assert.equal(dstPort, 9999, "decoded UDP dst port mismatch");
runtime.log("capture file round-trip ok: 2 packets, udp.dstPort = 9999");

// Now read the SAME file with a tcpdump-like filter in the trailing opts
// arg. The filter runs post-decode in userspace (not a kernel BPF program),
// so the handler only fires for matching packets. "udp" selects the one
// UDP frame; the TCP SYN is filtered out.
let udpSeen = 0;
let tcpSeen = 0;
await net.capture.openFile(path, (pkt: any) => {
  if (pkt.udp) udpSeen++;
  if (pkt.tcp) tcpSeen++;
}, { filter: "udp" });

runtime.assert.equal(udpSeen, 1, "filter 'udp' should select the UDP frame");
runtime.assert.equal(tcpSeen, 0, "filter 'udp' should drop the TCP frame");
runtime.log("filtered read ok: filter 'udp' matched 1 packet (tcp dropped)");

// CIDR (net X/Y) and portrange A-B filters. Both frames are 127.0.0.1, so
// `net 127.0.0.0/8` matches both; a `portrange` narrows by transport port:
// 9000-10000 selects the UDP frame (dst 9999), 400-500 selects the TCP SYN
// (dst 443).
let cidrSeen = 0;
await net.capture.openFile(path, () => cidrSeen++, { filter: "net 127.0.0.0/8" });
runtime.assert.equal(cidrSeen, 2, "filter 'net 127.0.0.0/8' should match both loopback frames");

let rangeUdp = 0;
let rangeTcp = 0;
await net.capture.openFile(path, () => rangeUdp++, { filter: "portrange 9000-10000" });
await net.capture.openFile(path, () => rangeTcp++, { filter: "tcp and dst portrange 400-500" });
runtime.assert.equal(rangeUdp, 1, "portrange 9000-10000 should match the UDP frame (dst 9999)");
runtime.assert.equal(rangeTcp, 1, "dst portrange 400-500 should match the TCP SYN (dst 443)");
runtime.log("CIDR + portrange filters ok: net/8 matched 2, portranges matched 1 each");
