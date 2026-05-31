package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func netDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"http.get": {
			Summary: "Perform an HTTP GET with a 5-second default timeout. Returns { status, body }.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "Absolute request URL (http:// or https://)."},
			},
			Returns: "Promise<{ status: number, body: string }> — the HTTP status code and the response body as a string. Redirects are followed by the default client.",
			Errors:  "Rejects on transport errors (DNS failure, connection refused, TLS handshake) or if the 5s context deadline is exceeded. 4xx/5xx responses do NOT reject — they surface via status.",
			Example: `const r = await net.http.get("https://example.com"); runtime.log(r.status);`,
		},
		"http.post": {
			Summary: "Perform an HTTP POST with a 5-second default timeout. Returns { status, body }.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "Absolute request URL (http:// or https://)."},
				{Name: "body", Type: "string", Optional: true, Desc: "Request body sent verbatim; omit or pass empty for no body. No Content-Type header is set automatically."},
			},
			Returns: "Promise<{ status: number, body: string }> — the HTTP status code and the response body as a string.",
			Errors:  "Rejects on transport errors (DNS failure, connection refused, TLS handshake) or if the 5s context deadline is exceeded. 4xx/5xx responses do NOT reject.",
			Example: `const r = await net.http.post("https://api.example.com/x", JSON.stringify({ a: 1 }));`,
		},
		"http.request": {
			Summary: "Full HTTP client: method, url, opts {headers, body, timeout, retry, follow, username, password}. Returns {status, ok, headers, body, url}. 4xx/5xx dont throw; retry covers transport errors + 5xx.",
			Params: []scriptengine.Param{
				{Name: "method", Type: "string", Desc: "HTTP method (GET, POST, PUT, …); upper-cased internally. Required."},
				{Name: "url", Type: "string", Desc: "Absolute request URL. Required."},
				{Name: "opts", Type: "{ headers?: Record<string, string>, body?: string, timeout?: number, retry?: number, follow?: boolean, username?: string, password?: string }", Optional: true, Desc: "headers sets request headers; body is the raw request body; timeout is the per-attempt client timeout in ms (default 30000); retry is the number of extra attempts (default 0) applied only to transport errors and 5xx with linear backoff capped at 1s; follow toggles redirect following (default true — false stops at the first 3xx); username/password set HTTP Basic auth."},
			},
			Returns: "Promise<{ status: number, ok: boolean, headers: Record<string, string>, body: string, url: string }> — status is the final status code; ok is status in [200,400); headers is a lower-cased name → value map (last value wins, alphabetically ordered); body is the response text; url is the final URL after redirects.",
			Errors:  "Rejects on transport errors (DNS, connection refused, TLS) or context deadline, and after exhausting retries on a persistent transport error / 5xx. A malformed method or URL rejects immediately (not retried). 4xx/5xx that succeed at the transport level resolve normally.",
			Example: `const r = await net.http.request("POST", "https://api.example.com", { headers: { "content-type": "application/json" }, body: "{}", retry: 2 });`,
		},
		"probe.tcp": {
			Summary: "Dial a TCP target and report latency + resolved IP. Default timeout 5s.",
			Params: []scriptengine.Param{
				{Name: "target", Type: "string", Desc: "host:port to dial; a bare host uses opts.port (default 80)."},
				{Name: "opts", Type: "{ timeout?: number, port?: string }", Optional: true, Desc: "timeout is the dial timeout in ms (default 5000); port is the fallback port when target has no :port (default \"80\")."},
			},
			Returns: "Promise<{ host: string, port: number, ip: string, latencyMs: number }> — the parsed host, port, the resolved remote IP, and the connect latency in milliseconds.",
			Errors:  "Rejects if the dial fails (refused, unreachable, name resolution failure, or timeout).",
			Example: `const r = await net.probe.tcp("example.com:443"); runtime.log(r.latencyMs);`,
		},
		"probe.dns": {
			Summary: "Look up A / AAAA / MX / TXT / CNAME / NS records. Default: all five.",
			Params: []scriptengine.Param{
				{Name: "host", Type: "string", Desc: "The hostname to resolve."},
				{Name: "opts", Type: "{ types?: string[] }", Optional: true, Desc: "types restricts the lookup to a subset (case-insensitive: 'a','aaaa','mx','txt','cname','ns'); omit to query all."},
			},
			Returns: "Promise<{ a?: string[], aaaa?: string[], mx?: { preference: number, host: string }[], txt?: string[], cname?: string, ns?: string[] }> — each key is present only when that record type returned at least one entry, so use `\"mx\" in result` to test presence.",
			Errors:  "Resolves with an object omitting record types that errored or were empty; per-type lookup failures are swallowed (not thrown). Always resolves.",
			Example: `const r = await net.probe.dns("example.com", { types: ["a", "mx"] });`,
		},
		"probe.tls": {
			Summary: "Open a TLS connection (InsecureSkipVerify; for probing only) and return the cert chain summary.",
			Params: []scriptengine.Param{
				{Name: "target", Type: "string", Desc: "host:port to dial; a bare host uses port 443. The host is sent as SNI."},
				{Name: "opts", Type: "{ timeout?: number }", Optional: true, Desc: "timeout is the dial timeout in ms (default 5000)."},
			},
			Returns: "Promise<{ cn: string, issuer: string, notBefore: string, notAfter: string, daysRemaining: number, dnsNames: string[], serialNumber: string, fingerprintSha256: string }> — leaf-certificate fields: common name, issuer CN, validity bounds (RFC3339), days until expiry, SAN DNS names, decimal serial, and the SHA-256 fingerprint as hex. Verification is skipped, so expired / mismatched certs still report.",
			Errors:  "Rejects if the dial / handshake fails, the connection is not TLS, or no peer certificates are presented.",
			Example: `const c = await net.probe.tls("example.com:443"); runtime.log(c.daysRemaining);`,
		},
		"probe.ntp": {
			Summary: "Query an NTPv4 server (UDP 123) and report offset, RTT, stratum, root delay / dispersion.",
			Params: []scriptengine.Param{
				{Name: "host", Type: "string", Desc: "The NTP server hostname or IP."},
				{Name: "opts", Type: "{ timeout?: number, port?: number | string }", Optional: true, Desc: "timeout is the query timeout in ms (default 5000); port overrides the default UDP port 123."},
			},
			Returns: "Promise<{ serverTime: string, offsetMs: number, rttMs: number, stratum: number, referenceTime: string, rootDelayMs: number, rootDispersionMs: number }> — the server's time and reference time (RFC3339 nanos), clock offset and round-trip in ms, the stratum, and the root delay / dispersion in ms.",
			Errors:  "Rejects if the NTP query fails (unreachable server, timeout, malformed response).",
			Example: `const r = await net.probe.ntp("pool.ntp.org"); runtime.log(r.offsetMs);`,
		},
		"probe.whois": {
			Summary: "Two-hop WHOIS via the IANA referral, returning the parsed record plus the raw response text.",
			Params: []scriptengine.Param{
				{Name: "domain", Type: "string", Desc: "The domain (or IP / ASN) to look up."},
				{Name: "opts", Type: "{ timeout?: number }", Optional: true, Desc: "timeout is the wire-level WHOIS client timeout in ms (default 10000)."},
			},
			Returns: "Promise<{ raw: string, domain?: { name: string, punycode: string, whoisServer: string, nameServers: string[], status: string[], dnssec: boolean, createdDate: string, updatedDate: string, expirationDate: string }, registrar?: { name: string } }> — raw is always the full WHOIS text; domain and registrar are best-effort parsed fields, omitted for TLDs the parser doesn't recognise.",
			Errors:  "Rejects if the WHOIS query itself fails (no referral, connection error, timeout). A parse failure is non-fatal — only raw is returned.",
			Example: `const w = await net.probe.whois("example.com"); runtime.log(w.domain?.expirationDate);`,
		},
		"probe.ping": {
			Summary: "Reachability probe. mode tcp (default; dials host:port) or icmp (needs raw-socket privileges). Returns { sent, received, lossPercent, minMs, avgMs, maxMs }. Unreachable = received 0, no throw.",
			Params: []scriptengine.Param{
				{Name: "host", Type: "string", Desc: "The target host. Required."},
				{Name: "opts", Type: "{ mode?: \"tcp\" | \"icmp\", count?: number, timeout?: number, port?: string }", Optional: true, Desc: "mode selects the probe (default 'tcp' — opens count TCP connections; 'icmp' sends real ICMP echo and needs raw-socket privileges); count is the number of probes (default 4); timeout is the per-probe timeout in ms (default 5000); port is the TCP target port (default \"80\", tcp mode only)."},
			},
			Returns: "Promise<{ host: string, ip: string, mode: string, sent: number, received: number, lossPercent: number, minMs: number, avgMs: number, maxMs: number }> — the resolved IP, the mode used, packets sent/received, loss percentage, and min/avg/max RTT in ms. A fully unreachable host resolves with received:0 and lossPercent:100 rather than rejecting.",
			Errors:  "Rejects if host is empty, mode is neither 'tcp' nor 'icmp', DNS resolution fails (tcp mode), or the ICMP run fails (typically missing raw-socket privileges). Individual lost packets are counted, not thrown.",
			Example: `const p = await net.probe.ping("example.com", { count: 3 }); runtime.log(p.lossPercent);`,
		},
		"probe.smtp": {
			Summary: "SMTP capability probe (no mail sent). EHLO + parse extensions. Returns { banner, ehloDomain, extensions, starttls, authMechanisms, sizeLimit }. Connection failures throw.",
			Params: []scriptengine.Param{
				{Name: "host", Type: "string", Desc: "The SMTP server host. Required."},
				{Name: "opts", Type: "{ port?: string, timeout?: number, ehloName?: string }", Optional: true, Desc: "port is the SMTP port (default \"25\"); timeout bounds the whole conversation in ms (default 10000); ehloName is the domain sent in EHLO (default \"localhost\")."},
			},
			Returns: "Promise<{ host: string, port: string, banner: string, ehloDomain: string, extensions: string[], starttls: boolean, authMechanisms: string[], sizeLimit: number }> — the greeting banner, the server's EHLO greeting line, the raw advertised extension lines, whether STARTTLS is offered, the upper-cased AUTH mechanism names, and the SIZE limit (0 if unadvertised). No mail is sent.",
			Errors:  "Rejects if host is empty, the dial fails, or the greeting / EHLO cannot be read. A server that simply omits STARTTLS or AUTH reports them as false / empty — a finding, not an error.",
			Example: `const s = await net.probe.smtp("mail.example.com"); runtime.log(s.starttls, s.authMechanisms);`,
		},
		"probe.wss": {
			Summary: "WebSocket handshake probe. Opens ws://wss:// connection, optional ping/pong RTT. Returns { connected, subprotocol, status, handshakeMs, pingMs }. Failed handshake throws.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "The WebSocket URL (ws:// or wss://). Required."},
				{Name: "opts", Type: "{ timeout?: number, ping?: boolean }", Optional: true, Desc: "timeout bounds the handshake and ping in ms (default 10000); ping toggles the ping/pong RTT measurement (default true)."},
			},
			Returns: "Promise<{ url: string, connected: boolean, subprotocol: string, status: number, handshakeMs: number, pingMs: number }> — connected is true on a successful upgrade, subprotocol is the negotiated subprotocol (or empty), status is the HTTP status of the 101 upgrade, handshakeMs is the handshake time in ms, and pingMs is the ping/pong RTT (or -1 when the ping was skipped or unanswered). The connection is closed immediately.",
			Errors:  "Rejects if url is empty or the handshake fails (non-101, refused, bad URL). A failed ping leaves pingMs at -1 rather than rejecting.",
			Example: `const w = await net.probe.wss("wss://echo.websocket.org"); runtime.log(w.handshakeMs);`,
		},
		"netstatus.check": {
			Summary: "Run DNS / TCP / TLS / HTTP against one host concurrently. Returns { reachable, dns, tcp, tls, http } — each sub-probe ok+error; reachable = dns.ok AND tcp.ok. Sub-failures are data, not throws.",
			Params: []scriptengine.Param{
				{Name: "host", Type: "string", Desc: "The host to check. Required."},
				{Name: "opts", Type: "{ port?: string, timeout?: number }", Optional: true, Desc: "port is the TCP/TLS port (default \"443\"); timeout bounds all four sub-probes in ms (default 10000)."},
			},
			Returns: "Promise<{ host: string, port: string, elapsedMs: number, reachable: boolean, dns: { ok: boolean, ips: string[], error?: string }, tcp: { ok: boolean, latencyMs: number, error?: string }, tls: { ok: boolean, daysRemaining: number, error?: string }, http: { ok: boolean, status: number, error?: string } }> — the four sub-probe results plus elapsed time. reachable is dns.ok AND tcp.ok; TLS/HTTP are reported but don't gate it. Each sub-probe carries its own error string instead of failing the call.",
			Errors:  "Rejects only if host is empty. Sub-probe failures are captured as data (ok:false + error) rather than thrown.",
			Example: `const s = await net.netstatus.check("example.com"); runtime.log(s.reachable);`,
		},
		"tcp.connect": {
			Summary: "Open a TCP client socket: net.tcp.connect(host, port, opts?) → Promise<handle>. Push/callback read model — handle.onData(cb)/onClose(cb)/onError(cb) register listeners; handle.write(data) sends (string→UTF-8 / Uint8Array); handle.remote/local are the peer/local addresses; handle.close() shuts down. opts { timeout?, readBuffer? }.",
			Params: []scriptengine.Param{
				{Name: "host", Type: "string", Desc: "The remote host to dial."},
				{Name: "port", Type: "string | number", Desc: "The remote port."},
				{Name: "opts", Type: "{ timeout?: number, readBuffer?: number }", Optional: true, Desc: "timeout is the dial timeout in ms (default 10000); readBuffer is the inbound channel capacity (default 64)."},
			},
			Returns: "Promise<{ remote: string, local: string, write(data: string | Uint8Array): Promise<void>, onData(cb: (ev: { bytes: Uint8Array, text: string }) => void): void, onClose(cb: () => void): void, onError(cb: (err: string) => void): void, close(): void }> — a connected-socket handle. remote/local are the peer/local addresses. write resolves once the bytes are written. onData fires per inbound chunk with both a Uint8Array and a UTF-8 text view; onClose fires when the stream ends; onError forwards non-EOF read/transport errors. close() tears down the connection.",
			Errors:  "The returned Promise rejects if the dial fails (refused, unreachable, timeout). After connect, write rejects on a write error and throws synchronously if called after close; transport read errors surface via the onError callback, not as rejections.",
			Example: `const sock = await net.tcp.connect("example.com", 80);
sock.onData(ev => runtime.log(ev.text));
await sock.write("GET / HTTP/1.0\r\n\r\n");`,
		},
		"udp.open": {
			Summary: "Open a UDP socket: net.udp.open(opts) → Promise<handle>. Connected mode { host, port } exposes send(data); bound mode { bind: ':9999' } exposes sendTo(data, host, port) and tags inbound events with { address, port }. Push/callback model — onMessage(cb)/onClose(cb)/onError(cb); handle.local is the bound address; handle.close() shuts down. opts also takes readBuffer?.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ host?: string, port?: string | number, bind?: string, readBuffer?: number }", Desc: "Selects the mode: connected mode needs { host, port } (net.DialUDP to that peer); bound mode needs { bind } (e.g. ':9999', net.ListenUDP on that local address). readBuffer is the inbound channel capacity (default 64). Provide exactly one of the two modes."},
			},
			Returns: "Promise<handle> — connected mode resolves to { local: string, send(data: string | Uint8Array): Promise<void>, onMessage, onClose, onError, close(): void }; bound mode resolves to { local: string, sendTo(data: string | Uint8Array, host: string, port: string | number): Promise<void>, send (throws), onMessage, onClose, onError, close(): void }. onMessage fires per datagram with { bytes: Uint8Array, text: string } plus { address, port } in bound mode. local is the bound address.",
			Errors:  "Rejects if neither { bind } nor { host, port } is supplied, or the dial / listen fails. send/sendTo reject on a write error and throw synchronously after close; calling send on a bound socket throws (use sendTo). Read errors surface via onError (a clean close ends silently).",
			Example: `const u = await net.udp.open({ host: "1.1.1.1", port: 53 });
u.onMessage(ev => runtime.log(ev.bytes.length));
await u.send(query);`,
		},
		"icmp.open": {
			Summary: "Open a raw ICMP socket: net.icmp.open(opts?) → Promise<handle>. Requires root / CAP_NET_RAW (open rejects otherwise). opts { network?: 'ip4'|'ip6' (default 'ip4'), readBuffer? }. handle.send({ to, type?, code?, id?, seq?, payload? }) builds an Echo-shaped body (non-Echo bodies not modelled); push/callback model — onMessage(cb) events carry { address, type, code }; onClose(cb)/onError(cb); handle.network/local; handle.close().",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ network?: \"ip4\" | \"ip6\", readBuffer?: number }", Optional: true, Desc: "network selects the IP version (default 'ip4'); readBuffer is the inbound channel capacity (default 64)."},
			},
			Returns: "Promise<{ network: string, local: string, send(opts: { to: string, type?: number, code?: number, id?: number, seq?: number, payload?: string | Uint8Array }): Promise<void>, onMessage(cb: (ev: { bytes: Uint8Array, text: string, address: string, type: number, code: number }) => void): void, onClose, onError, close(): void }> — a raw-ICMP handle. send builds an ICMP message (Echo-shaped body; type defaults to the network's echo request, to is the destination address). onMessage fires per received packet with the marshalled body plus { address, type, code } meta.",
			Errors:  "Rejects if the raw socket can't be opened — typically because it needs root / CAP_NET_RAW. send rejects on resolve/marshal/write errors, throws synchronously after close, and throws if opts.to is missing. Read errors surface via onError.",
			Example: `const p = await net.icmp.open();
p.onMessage(ev => runtime.log(ev.address, ev.type));
await p.send({ to: "8.8.8.8", id: 1, seq: 1, payload: "ping" });`,
		},
		"capture.interfaces": {
			Summary:    "List the host's network interfaces synchronously: net.capture.interfaces() → array of { name, addresses: string[], up, loopback }. Pure-Go (no privileges, all platforms).",
			ReturnType: "{ name: string; addresses: string[]; up: boolean; loopback: boolean }[]",
			Returns:    "{ name: string, addresses: string[], up: boolean, loopback: boolean }[] — one entry per interface with its name, assigned addresses (CIDR strings), and up / loopback flags. Synchronous (not a Promise).",
			Errors:     "Throws if interface enumeration fails.",
			Example:    `for (const i of net.capture.interfaces()) runtime.log(i.name, i.up);`,
		},
		"capture.open": {
			Summary: "Live packet capture: net.capture.open({ iface, promisc?, snaplen?, filter? }, pkt => {…}) → Promise<{ iface, link, close() }>. Linux + macOS only (Windows rejects); needs root / CAP_NET_RAW (Linux) or /dev/bpf access (macOS). promisc defaults true. The handler is called per frame with a decoded packet { ts, length, captureLength, link, eth?, ip?, tcp?, udp?, icmp?, payload?, bytes }. Optional filter is a tcpdump-like expression string (e.g. 'tcp and port 80'), evaluated post-decode in userspace — NOT a kernel BPF program, so it skips the JS callback for non-matching packets but does not avoid the kernel→userspace copy. Supports tcp/udp/icmp/ip/ip6, host/src host/dst host, port/src port/dst port, and/or/not + parens, implicit-and between juxtaposed primaries. No CIDR (net X/Y) or portrange yet; a malformed expression makes open reject. close() returns Promise<void>. Pure-Go gopacket (no libpcap/cgo).",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ iface: string, promisc?: boolean, snaplen?: number, filter?: string }", Desc: "iface is the interface name to capture on (required); promisc enables promiscuous mode (default true); snaplen caps the per-packet capture length in bytes (default 262144); filter is an optional tcpdump-like expression (e.g. 'tcp and port 80') applied post-decode in userspace — supports tcp/udp/icmp/ip/ip6, host/src host/dst host, port/src port/dst port, and/or/not + parens; no CIDR or portrange."},
				{Name: "onPacket", Type: "(pkt: { ts: number, length: number, captureLength: number, link: string, eth?: { src: string, dst: string, type: string }, ip?: { version: number, src: string, dst: string, protocol: string, ttl: number }, tcp?: { srcPort: number, dstPort: number, seq: number, ack: number, flags: { syn: boolean, ack: boolean, fin: boolean, rst: boolean, psh: boolean, urg: boolean } }, udp?: { srcPort: number, dstPort: number, length: number }, icmp?: { type: number, code: number }, payload?: Uint8Array, bytes: Uint8Array }) => void", Desc: "Called once per matching frame with the decoded packet. ts is epoch ms; layer keys are present only when that layer decoded; bytes is always the raw frame; payload is the application-layer bytes when present."},
			},
			Returns: "Promise<{ iface: string, link: string, close(): Promise<void> }> — a live-capture handle. link is the link-type name; close() stops the capture and resolves when the source is torn down. The handler keeps firing until close() is called or the source errors.",
			Errors:  "Rejects if iface is missing, the filter expression is malformed, the platform is unsupported (Windows), or the capture can't be opened (missing root / CAP_NET_RAW on Linux, /dev/bpf access on macOS). Throws synchronously if onPacket is not a function.",
			Example: `const cap = await net.capture.open({ iface: "en0", filter: "tcp and port 443" }, pkt => {
  runtime.log(pkt.ip?.src, "→", pkt.ip?.dst);
});
await cap.close();`,
		},
		"capture.openFile": {
			Summary: "Read a .pcap / .pcapng file: net.capture.openFile(path, pkt => {…}, opts?) → Promise<void>. Calls the handler once per decoded packet (same shape as capture.open) and resolves at EOF. Offline; no privileges. opts is an optional trailing arg { filter? } — the 2-arg form still works; filter is the same tcpdump-like expression string as capture.open (post-decode/userspace, not kernel BPF; no CIDR/portrange; malformed → rejects).",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Path to a .pcap or .pcapng file; the format is auto-detected from the magic bytes."},
				{Name: "onPacket", Type: "(pkt: { ts: number, length: number, captureLength: number, link: string, eth?: object, ip?: object, tcp?: object, udp?: object, icmp?: object, payload?: Uint8Array, bytes: Uint8Array }) => void", Desc: "Called once per decoded packet (same shape as capture.open's handler)."},
				{Name: "opts", Type: "{ filter?: string }", Optional: true, Desc: "filter is the same tcpdump-like expression as capture.open, applied post-decode in userspace; omit (2-arg form) to deliver every packet."},
			},
			Returns: "Promise<void> — resolves at end-of-file after dispatching every (matching) packet to the handler.",
			Errors:  "Rejects if the filter expression is malformed, the file can't be opened or parsed, or the handler throws. Throws synchronously if onPacket is not a function.",
			Example: `await net.capture.openFile("/tmp/dump.pcap", pkt => runtime.log(pkt.tcp?.dstPort), { filter: "tcp" });`,
		},
		"capture.toFile": {
			Summary: "Write raw frames to a .pcap file: net.capture.toFile(path, { linkType?, snaplen? }) → { write(bytes, { ts? }), close() }. write appends a raw frame (Uint8Array); ts (ms) overrides the timestamp. close() flushes and returns Promise<void>. Offline; no privileges.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Path of the .pcap file to create (overwritten if it exists)."},
				{Name: "opts", Type: "{ snaplen?: number, linkType?: number }", Optional: true, Desc: "snaplen is the pcap global-header snap length (default 262144); linkType is the numeric pcap link-type written into the header (default Ethernet)."},
			},
			ReturnType: "{ write(bytes: string | Uint8Array, opts?: { ts?: number }): void; close(): Promise<void> }",
			Returns:    "{ write(bytes, opts?): void, close(): Promise<void> } — a writer handle (returned synchronously, not a Promise). write appends one raw frame to the file (opts.ts in ms overrides the timestamp, defaulting to now) and returns undefined. close() flushes and closes the file, resolving when done.",
			Errors:     "Throws synchronously if the file can't be created or the header can't be written, and on a per-frame write error. write throws if called after close. close() rejects on a close error.",
			Example: `const w = net.capture.toFile("/tmp/out.pcap");
w.write(frameBytes, { ts: Date.now() });
await w.close();`,
		},
		"email.spf": {
			Summary: "Query TXT(<domain>) for SPF, return record + parsed mechanisms + all-policy.",
			Params: []scriptengine.Param{
				{Name: "domain", Type: "string", Desc: "The domain whose apex TXT records are queried for an SPF (v=spf1) record."},
			},
			Returns: "Promise<{ present: false } | { present: true, record: string, mechanisms: string[], allPolicy: string }> — when found, record is the raw SPF string, mechanisms is the tokenised list after v=spf1, and allPolicy summarises the trailing all-style mechanism (pass / fail / softfail / neutral). Missing record resolves to { present: false }.",
			Errors:  "Rejects only on a DNS lookup error other than NXDOMAIN; an absent record resolves to { present: false }.",
			Example: `const r = await net.email.spf("example.com"); if (r.present) runtime.log(r.allPolicy);`,
		},
		"email.dmarc": {
			Summary: "Query TXT(_dmarc.<domain>) and parse policy / pct / rua / ruf tags.",
			Params: []scriptengine.Param{
				{Name: "domain", Type: "string", Desc: "The domain whose _dmarc.<domain> TXT record is queried for a v=DMARC1 record."},
			},
			Returns: "Promise<{ present: false } | { present: true, record: string, tags: Record<string, string>, policy: string, subdomain: string, percent: string, rua: string, ruf: string }> — tags is the full parsed tag map; policy/subdomain/percent/rua/ruf surface the common p/sp/pct/rua/ruf tags. Missing record resolves to { present: false }.",
			Errors:  "Rejects only on a DNS lookup error other than NXDOMAIN; an absent record resolves to { present: false }.",
			Example: `const r = await net.email.dmarc("example.com"); runtime.log(r.present && r.policy);`,
		},
		"email.mtaSts": {
			Summary: "Probe MTA-STS: TXT(_mta-sts.<domain>) plus the fetched policy file.",
			Params: []scriptengine.Param{
				{Name: "domain", Type: "string", Desc: "The domain whose _mta-sts.<domain> TXT record and well-known policy file are probed."},
			},
			Returns: "Promise<{ present: false } | { present: true, record: string, txt: { v: string, id: string }, policy?: { version?: string, mode?: string, mx?: string[], maxAge?: number | string }, policyError?: string }> — txt carries the versioned id from the TXT marker; policy is the parsed well-known file (mode + mx + maxAge), or policyError holds the fetch/parse error string when the file couldn't be retrieved. Missing TXT resolves to { present: false }.",
			Errors:  "Rejects only on a DNS lookup error other than NXDOMAIN; an absent record resolves to { present: false }. A policy-file fetch failure is captured in policyError, not thrown.",
			Example: `const r = await net.email.mtaSts("example.com"); runtime.log(r.present && r.policy?.mode);`,
		},
		"email.tlsRpt": {
			Summary: "Probe TLS-RPT: TXT(_smtp._tls.<domain>) and parse rua.",
			Params: []scriptengine.Param{
				{Name: "domain", Type: "string", Desc: "The domain whose _smtp._tls.<domain> TXT record is queried for a v=TLSRPTv1 record."},
			},
			Returns: "Promise<{ present: false } | { present: true, record: string, tags: Record<string, string>, rua: string }> — tags is the parsed tag map; rua surfaces the report-URI tag. Missing record resolves to { present: false }.",
			Errors:  "Rejects only on a DNS lookup error other than NXDOMAIN; an absent record resolves to { present: false }.",
			Example: `const r = await net.email.tlsRpt("example.com"); runtime.log(r.present && r.rua);`,
		},
		"email.bimi": {
			Summary: "Probe BIMI: TXT(<selector>._bimi.<domain>); selector defaults to 'default'.",
			Params: []scriptengine.Param{
				{Name: "domain", Type: "string", Desc: "The domain whose <selector>._bimi.<domain> TXT record is queried for a v=BIMI1 record."},
				{Name: "opts", Type: "{ selector?: string }", Optional: true, Desc: "selector names the BIMI selector to query (default 'default')."},
			},
			Returns: "Promise<{ present: false, selector: string } | { present: true, selector: string, record: string, tags: Record<string, string>, l: string, a: string }> — selector echoes the queried selector; when found, l is the logo URL tag and a is the assertion (VMC) tag. Missing record resolves to { present: false, selector }.",
			Errors:  "Rejects only on a DNS lookup error other than NXDOMAIN; an absent record resolves to { present: false, selector }.",
			Example: `const r = await net.email.bimi("example.com", { selector: "v1" }); runtime.log(r.present && r.l);`,
		},
		"email.all": {
			Summary: "Run all five email probes in parallel — five-way handshake aggregate.",
			Params: []scriptengine.Param{
				{Name: "domain", Type: "string", Desc: "The domain to run all five email-auth probes against."},
			},
			Returns: "Promise<{ domain: string, spf: object, dmarc: object, mtaSts: object, tlsRpt: object, bimi: object }> — domain echoes the input; each probe key holds that probe's result (the same shape its individual binding returns) or { error: string } when that single probe failed. A per-probe failure doesn't fail the aggregate.",
			Errors:  "Resolves even when individual probes fail (their failure surfaces under <probe>.error). Always resolves.",
			Example: `const a = await net.email.all("example.com"); runtime.log(a.spf, a.dmarc);`,
		},
		"email.send": {
			Summary: "Send an outbound email: net.email.send({to, from, subject, body, html?, attachments?, headers?, server: {host, port?, auth?, tls?}, timeout?}) → Promise<{accepted: string[], rejected: [{address, reason}]}>. One TCP connection per call; per-recipient outcome captured. Transport failures throw; per-RCPT rejections surface in the result. TLS modes: starttls (default), tls, none.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ to: string | string[], from: string, subject?: string, body?: string, html?: string, attachments?: { filename: string, contentType?: string, bytes: Uint8Array | ArrayBuffer }[], headers?: Record<string, string>, server: { host: string, port?: number, auth?: { username: string, password: string }, tls?: \"starttls\" | \"tls\" | \"none\" }, timeout?: number }", Desc: "to (string or array) and from are required, as is server.host. subject/body/html shape the message: body alone → text/plain, body+html → multipart/alternative, any attachments → multipart/mixed. attachments carry raw bytes (contentType defaults to application/octet-stream). headers adds custom headers (CR/LF stripped). server.port defaults to 587; server.auth enables PLAIN auth (skipped when tls is 'none'); server.tls picks the transport: 'starttls' (default), implicit 'tls', or 'none'. timeout is the dial / connection timeout in ms (default 30000)."},
			},
			Returns: "Promise<{ accepted: string[], rejected: { address: string, reason: string }[] }> — accepted lists recipients the server accepted at RCPT TO; rejected pairs each refused address with the server's reason. The DATA body is sent only when at least one recipient was accepted.",
			Errors:  "Rejects on missing required fields (to / from / server.host), transport / protocol failures (dial, HELO, STARTTLS unavailable when requested, AUTH, MAIL FROM, DATA), or timeout. Per-recipient RCPT rejections are returned in rejected, not thrown.",
			Example: `const r = await net.email.send({
  to: "to@example.com", from: "from@example.com", subject: "hi", body: "hello",
  server: { host: "smtp.example.com", port: 587, auth: { username: "u", password: "p" } },
});
runtime.log(r.accepted, r.rejected);`,
		},
		"browser.open": {
			Summary: "Open a stateful HTTP session: { setUserAgent, setHeader, get, post, cookies }. Cookie jar + default headers persist across requests (like a browser).",
			Returns: "Promise<{ setUserAgent(ua: string): void, setHeader(name: string, value: string): void, get(url: string): Promise<{ status: number, ok: boolean, headers: Record<string, string>, body: string, url: string }>, post(url: string, body?: string): Promise<{ status: number, ok: boolean, headers: Record<string, string>, body: string, url: string }>, cookies(url: string): Promise<{ name: string, value: string }[]> }> — a session handle backed by an http.Client with an automatic cookie jar (public-suffix scoped). setUserAgent/setHeader register default headers replayed on every request; get/post return the same result shape as net.http.request; cookies lists the jar's cookies for a URL.",
			Errors:  "browser.open rejects only if the cookie jar can't be created. get/post reject if the URL is empty or the request fails (transport error); cookies rejects if the URL is empty or unparseable. 4xx/5xx responses resolve normally.",
			Example: `const b = await net.browser.open();
b.setUserAgent("my-bot/1.0");
await b.post("https://site/login", "user=x&pass=y");
const home = await b.get("https://site/home");
runtime.log(await b.cookies("https://site/"));`,
		},
	}
}
