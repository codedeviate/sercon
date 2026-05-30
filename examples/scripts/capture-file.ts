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

// A hand-built Ethernet/IPv4/UDP frame, dst port 9999, payload "hi".
// (Scripts can't easily synthesise frames at runtime, so we embed the raw
// bytes; gopacket decodes them the same as a captured frame would be.)
const frame = new Uint8Array([
  2, 0, 0, 0, 0, 2, 2, 0, 0, 0, 0, 1, 8, 0, 69, 0, 0, 30, 0, 0, 0, 0,
  64, 17, 124, 205, 127, 0, 0, 1, 127, 0, 0, 1, 16, 146, 39, 15, 0, 10,
  0, 0, 104, 105,
]);

const path = "/tmp/sercon-capture-demo.pcap";

// Write the frame into a .pcap file, then close (flush) it.
const w = net.capture.toFile(path, { snaplen: 65536 });
w.write(frame, { ts: 1700000000000 });
await w.close();
runtime.log("wrote", frame.length, "byte frame to", path);

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
  );
  if (pkt.udp) dstPort = pkt.udp.dstPort;
});

runtime.assert.equal(seen, 1, "expected exactly one packet");
runtime.assert.equal(dstPort, 9999, "decoded UDP dst port mismatch");
runtime.log("capture file round-trip ok: 1 packet, udp.dstPort = 9999");
