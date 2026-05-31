package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func netDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"http.get":           {Summary: "Perform an HTTP GET with a 5-second default timeout. Returns { status, body }."},
		"http.post":          {Summary: "Perform an HTTP POST with a 5-second default timeout. Returns { status, body }."},
		"http.request":       {Summary: "Full HTTP client: method, url, opts {headers, body, timeout, retry, follow, username, password}. Returns {status, ok, headers, body, url}. 4xx/5xx dont throw; retry covers transport errors + 5xx."},
		"probe.tcp":          {Summary: "Dial a TCP target and report latency + resolved IP. Default timeout 5s."},
		"probe.dns":          {Summary: "Look up A / AAAA / MX / TXT / CNAME / NS records. Default: all five."},
		"probe.tls":          {Summary: "Open a TLS connection (InsecureSkipVerify; for probing only) and return the cert chain summary."},
		"probe.ntp":          {Summary: "Query an NTPv4 server (UDP 123) and report offset, RTT, stratum, root delay / dispersion."},
		"probe.whois":        {Summary: "Two-hop WHOIS via the IANA referral, returning the parsed record plus the raw response text."},
		"probe.ping":         {Summary: "Reachability probe. mode tcp (default; dials host:port) or icmp (needs raw-socket privileges). Returns { sent, received, lossPercent, minMs, avgMs, maxMs }. Unreachable = received 0, no throw."},
		"probe.smtp":         {Summary: "SMTP capability probe (no mail sent). EHLO + parse extensions. Returns { banner, ehloDomain, extensions, starttls, authMechanisms, sizeLimit }. Connection failures throw."},
		"probe.wss":          {Summary: "WebSocket handshake probe. Opens ws://wss:// connection, optional ping/pong RTT. Returns { connected, subprotocol, status, handshakeMs, pingMs }. Failed handshake throws."},
		"netstatus.check":    {Summary: "Run DNS / TCP / TLS / HTTP against one host concurrently. Returns { reachable, dns, tcp, tls, http } — each sub-probe ok+error; reachable = dns.ok AND tcp.ok. Sub-failures are data, not throws."},
		"tcp.connect":        {Summary: "Open a TCP client socket: net.tcp.connect(host, port, opts?) → Promise<handle>. Push/callback read model — handle.onData(cb)/onClose(cb)/onError(cb) register listeners; handle.write(data) sends (string→UTF-8 / Uint8Array); handle.remote/local are the peer/local addresses; handle.close() shuts down. opts { timeout?, readBuffer? }."},
		"udp.open":           {Summary: "Open a UDP socket: net.udp.open(opts) → Promise<handle>. Connected mode { host, port } exposes send(data); bound mode { bind: ':9999' } exposes sendTo(data, host, port) and tags inbound events with { address, port }. Push/callback model — onMessage(cb)/onClose(cb)/onError(cb); handle.local is the bound address; handle.close() shuts down. opts also takes readBuffer?."},
		"icmp.open":          {Summary: "Open a raw ICMP socket: net.icmp.open(opts?) → Promise<handle>. Requires root / CAP_NET_RAW (open rejects otherwise). opts { network?: 'ip4'|'ip6' (default 'ip4'), readBuffer? }. handle.send({ to, type?, code?, id?, seq?, payload? }) builds an Echo-shaped body (non-Echo bodies not modelled); push/callback model — onMessage(cb) events carry { address, type, code }; onClose(cb)/onError(cb); handle.network/local; handle.close()."},
		"capture.interfaces": {Summary: "List the host's network interfaces synchronously: net.capture.interfaces() → array of { name, addresses: string[], up, loopback }. Pure-Go (no privileges, all platforms)."},
		"capture.open":       {Summary: "Live packet capture: net.capture.open({ iface, promisc?, snaplen?, filter? }, pkt => {…}) → Promise<{ iface, link, close() }>. Linux + macOS only (Windows rejects); needs root / CAP_NET_RAW (Linux) or /dev/bpf access (macOS). promisc defaults true. The handler is called per frame with a decoded packet { ts, length, captureLength, link, eth?, ip?, tcp?, udp?, icmp?, payload?, bytes }. Optional filter is a tcpdump-like expression string (e.g. 'tcp and port 80'), evaluated post-decode in userspace — NOT a kernel BPF program, so it skips the JS callback for non-matching packets but does not avoid the kernel→userspace copy. Supports tcp/udp/icmp/ip/ip6, host/src host/dst host, port/src port/dst port, and/or/not + parens, implicit-and between juxtaposed primaries. No CIDR (net X/Y) or portrange yet; a malformed expression makes open reject. close() returns Promise<void>. Pure-Go gopacket (no libpcap/cgo)."},
		"capture.openFile":   {Summary: "Read a .pcap / .pcapng file: net.capture.openFile(path, pkt => {…}, opts?) → Promise<void>. Calls the handler once per decoded packet (same shape as capture.open) and resolves at EOF. Offline; no privileges. opts is an optional trailing arg { filter? } — the 2-arg form still works; filter is the same tcpdump-like expression string as capture.open (post-decode/userspace, not kernel BPF; no CIDR/portrange; malformed → rejects)."},
		"capture.toFile":     {Summary: "Write raw frames to a .pcap file: net.capture.toFile(path, { linkType?, snaplen? }) → { write(bytes, { ts? }), close() }. write appends a raw frame (Uint8Array); ts (ms) overrides the timestamp. close() flushes and returns Promise<void>. Offline; no privileges."},
		"email.spf":          {Summary: "Query TXT(<domain>) for SPF, return record + parsed mechanisms + all-policy."},
		"email.dmarc":        {Summary: "Query TXT(_dmarc.<domain>) and parse policy / pct / rua / ruf tags."},
		"email.mtaSts":       {Summary: "Probe MTA-STS: TXT(_mta-sts.<domain>) plus the fetched policy file."},
		"email.tlsRpt":       {Summary: "Probe TLS-RPT: TXT(_smtp._tls.<domain>) and parse rua."},
		"email.bimi":         {Summary: "Probe BIMI: TXT(<selector>._bimi.<domain>); selector defaults to 'default'."},
		"email.all":          {Summary: "Run all five email probes in parallel — five-way handshake aggregate."},
		"email.send":         {Summary: "Send an outbound email: net.email.send({to, from, subject, body, html?, attachments?, headers?, server: {host, port?, auth?, tls?}, timeout?}) → Promise<{accepted: string[], rejected: [{address, reason}]}>. One TCP connection per call; per-recipient outcome captured. Transport failures throw; per-RCPT rejections surface in the result. TLS modes: starttls (default), tls, none."},
		"browser.open":       {Summary: "Open a stateful HTTP session: { setUserAgent, setHeader, get, post, cookies }. Cookie jar + default headers persist across requests (like a browser)."},
	}
}
