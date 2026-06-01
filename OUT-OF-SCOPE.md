# Out of scope / backlog

Ideas, follow-ups, and known gaps that aren't implemented yet. Promotion
from this list to a real issue or commit is the only way these become
"real" work. Keep entries terse; expand into a spec or plan once picked up.

This file is organised by **implementation difficulty** at the top level
and by the original **topical groups** at the second level. Difficulty
buckets are:

- **Trivial** — stdlib or one tiny well-known lib; mostly type
  conversions and `vm.Set`. A few hours of work.
- **Easy** — established 3rd-party lib (>1k stars or de facto standard);
  integration is mechanical: wire to `PromisifyAsync` for I/O, map
  types, done. Half a day to a day.
- **Moderate** — established lib BUT non-trivial wiring: stateful handle
  objects exposed to JS, lifetime concerns, event-loop integration,
  type-mapping work, or design choices. A day to a few days.
- **Hard** — substantial design work, missing prior art, requires new
  abstractions in the engine itself, needs writing-from-scratch in Go,
  or depends on an external CLI without a stable Go equivalent.
  Multi-day to weeks.

Library picks honour the project constraints: **pure Go, no cgo;
stdlib first; trustworthy and maintained; no heavy frameworks**.

A bucket with no remaining work is omitted rather than left empty — the
**Trivial** and **Easy** buckets have shipped out entirely (most recently the
editor-autocomplete tooling and the SQL engines), so only **Moderate**,
**Hard**, and **Deferred** carry open items today.

A fifth bucket, **Deferred**, lives at the bottom of this file. Items
land there when there's a concrete reason to put the work down rather
than rank it by effort — no trustworthy pure-Go library exists, the
feature depends on an external runtime we don't want to require yet,
the design space is unsettled, or it conflicts with current direction.
Each Deferred entry names the reason so it's easy to re-promote when
the situation changes.

## Moderate

Most Moderate items have shipped. The original surface — the `.d.ts`
JSDoc generator, `text.preg` / `text.preg2`, the full `crypto.jwt` +
`crypto.encrypt` surfaces, barcode decode + quiet-zone,
`net.http.request`, the `net.probe` family, `net.netstatus`,
`net.browser`, `db.sqlite`, `db.redis` / `db.memcached` / `db.ldap` /
`db.dict`, `services.ai`, the `--watch` CLI flag with module-graph
invalidation, the `Options.ModuleLoader` hook, and robust import
parsing — shipped across v0.5.0 – v0.5.30 (re-bucketed under v0.8.0's
9-category surface and promoted to top-level globals, dropping the
`api.` prefix, in v0.9.0). Canonical (stable-key-order) JSON for
object-returning bindings landed later, across v0.16.0 (json-tagged
structs for fixed-shape results) and v0.20.0 (`scriptengine.Ordered`
for conditional / dynamic / decoded-JSON keys; `text.jq` is the lone
exception, since `gojq` discards key order internally). `shell_stream(cmd,
cb)` shipped in v0.28.0 as `services.exec.stream` (line-streaming a
subprocess's stdout/stderr to a JS callback). `codec.xml` shipped in v0.32.0
(value ↔ XML via the shared dump IR, `@`-attribute + `#text` convention).
**No open Moderate items remain** — the networking subsection below is a
shipped-record whose one residual gap (route-table enumeration) is parked
under Deferred.

### Networking — clients & raw sockets

Read/write client sockets shipped in v0.22.0: `net.tcp.connect`,
`net.udp.open` (connected + bound), and `net.icmp.open` (raw ICMP,
needs root / CAP_NET_RAW) — all with a push/callback read model
(`onData`/`onMessage` + `onClose`/`onError`), on the shared
`socket_common.go` scaffold. `net.icmp` `send` gained raw (non-Echo)
message bodies in v0.27.0 (`body` → `icmp.RawBody`, marshalled verbatim,
for hand-built messages such as destination-unreachable). No open gaps
remain in this subsection:

- **Interface / address enumeration shipped** as `net.capture.interfaces`
  (v0.24.0 — stdlib `net.Interfaces`, returning `{ name, addresses, up,
  loopback }` per interface). A general `net.interfaces` alias outside the
  capture namespace could wrap the same call on demand. Route-table
  enumeration is the one piece still missing and is parked under
  **Deferred → Networking — clients & raw sockets** (no portable stdlib
  route API).

## Hard

### Networking — servers

The server foundation — `LoopCallable` (loop-bound callback marshalling)
and `Engine.HoldRun` (long-lived `Run` keep-alive) — shipped in v0.10.0.
Built on it: HTTP / HTTPS (router, middleware, static-file mount, WebSocket
upgrade — v0.10.0), SMTP (`server.smtp.listen` plus the outbound
`net.email.send` sender — v0.11.0), raw TCP / UDP listeners
(`server.tcp.listen` / `server.udp.listen`, each reusing the v0.22.0 client
push-socket handle — v0.23.0), and the raw ICMP listener
(`server.icmp.listen` with `reply()`, root / CAP_NET_RAW — v0.31.0). Nothing
remains open in this subsection.

Application-protocol servers (IMAP, FTP, POP3) and additional protocols
(e.g. MQTT) are parked under **Deferred → Networking — servers** with their
specific reasons — each its own brainstorm → plan → ship cycle atop the
`server.tcp` accept loop.

### Agent-browser automation

`agent-browser` is recon's headless-Chrome driver. The recon script
bindings are extensive and worth a dedicated namespace
(e.g. `services.agentBrowser.*` — the `services.*` bucket already
holds the external-CLI wrappers like git, gh, ai). All of these
require the `agent-browser` CLI on `PATH`; gate calls on an
`agentBrowser.available` boolean and surface a clean error otherwise.
The hard rating reflects the orchestration scope, not any single call.

- **Navigation** — `open(url, opts?)`, `close()`, `close_all()`,
  `back()`, `forward()`, `reload()` (script: `agent-browser-navigation.rhai`).
  **Library:** `os/exec` (stdlib) + JSON request/response; no Go
  agent-browser SDK exists, so the entire command surface has to be
  modelled as subprocess calls. (A pure-Go CDP alternative is
  `github.com/chromedp/chromedp`, but that is a different product, not
  recon's `agent-browser`.)
- **Locating** — `find(selector, opts?)` by role / text / label /
  placeholder / testid (script: `agent-browser-find.rhai`).
  **Library:** same `os/exec`-based bridge as Navigation; the design
  work is in exposing the locator handle to JS so subsequent
  Interaction calls can reference it.
- **Interaction** — `click`, `dblclick`, `hover`, `fill`, `type_text`,
  `press`, `keyboard_type`, `keyboard_insert`, `check`, `uncheck`,
  `scroll`, `scrollintoview`, `focus` (script: `agent-browser-interaction.rhai`).
  **Library:** subprocess bridge; depends on a working locator handle.
- **Inspection** — `get(selector, key?)`, `is_visible`, `is_enabled`,
  `is_checked`, `eval_js(code)` (script: `agent-browser-inspect.rhai`).
  **Library:** subprocess bridge; `eval_js` in particular needs careful
  JSON round-tripping.
- **Capture** — `screenshot()`, `snapshot(include_interactive?)`,
  `pdf(opts?)` (scripts: `agent-browser-screenshot.rhai`,
  `agent-browser-snapshot.rhai`, `agent-browser-pdf.rhai`).
  **Library:** subprocess bridge; binary payloads (`Uint8Array`) over
  JSON need a base64 hop.
- **Defaults / escape hatch** — `default_options`, `clear_default_options`,
  `set_default_options`, `cmd(command, args)` for cookies / storage /
  tabs / network / console (scripts: `agent-browser-options.rhai`,
  `agent-browser-cmd.rhai`). **Library:** subprocess bridge plus a
  persistent options bag held by the namespace.

## Deferred

Items here aren't ranked by difficulty — they're parked for a stated
reason. Move them back into Trivial / Easy / Moderate / Hard once the
reason resolves.

### Encoding / decoding / barcodes

- **PDF417 decoder.** v0.5.4 shipped what is now `codec.barcode.decode`
  over gozxing, which doesn't cover PDF417 (the encoder still works via
  boombuler). A pure-Go PDF417 reader would close the symmetry.
  **Reason:** no maintained pure-Go PDF417 reader exists — the realistic
  path is porting ZXing's Java PDF417 reader, which is a from-scratch
  effort with no demand signal. Re-promote if a library appears or
  someone actually needs PDF417 round-tripping.

### Archives & document handling

- **`pdf_export_page(src, page, dest_or_opts?, opts?)`** — Render one
  PDF page to PNG/JPEG/WEBP (script: `pdf.rhai`).
  **Reason:** no trustworthy pure-Go PDF renderer exists today —
  `github.com/unidoc/unipdf` is commercial-licensed, MuPDF requires
  cgo, Poppler is C. The realistic implementations are (a) shell out
  to `pdftoppm` / `mutool` and parse output, or (b) accept a cgo
  dependency. Both conflict with current direction (no-cgo, minimise
  external CLI dependencies). Re-promote if a trustworthy pure-Go
  renderer appears, or if we decide to allow optional CLI fallbacks
  for niche features.

### Databases

The native pure-Go SQL engines have all shipped — `db.sqlite`,
`db.postgres`, `db.mysql`, `db.mssql`, `db.clickhouse`, `db.oracle`
(plus the non-SQL `db.redis` / `db.memcached` / `db.ldap` / `db.dict`).
Adding another `database/sql` engine is now a known, mechanical step
(driver import + DSN builder onto the shared handle in
`cmd/sercon/db_sql.go`), so new engines are promoted on demand rather
than tracked here. The two parked below have a concrete reason to wait.

- **Snowflake.** Fits the same shared-handle pattern, but its driver
  `github.com/snowflakedb/gosnowflake` drags in large AWS/Azure/GCS
  cloud SDKs, which conflicts with the project's "no heavy frameworks"
  rule. **Reason:** dependency weight, no demand signal. Re-promote if
  someone needs Snowflake and accepts the transitive-dependency cost.
- **ODBC connectivity.** A generic ODBC bridge would reach any engine
  with a system DSN, but the only real Go option,
  `github.com/alexbrainman/odbc`, links the platform ODBC driver
  manager (unixODBC on Linux/macOS) via **cgo** — and needs that
  manager plus a per-engine ODBC driver installed at runtime. That
  conflicts with the no-cgo constraint on the platforms that matter
  most here. **Reason:** no pure-Go ODBC implementation exists.
  Re-promote if one appears, or skip ODBC entirely — the native
  pure-Go drivers already cover the engines people actually ask for.

### Networking — clients & raw sockets

- **Route-table enumeration.** Listing the host's routing table (gateway,
  destination, interface per route) has no portable stdlib API — `net`
  exposes interfaces and addresses but not routes. The realistic path is
  per-OS: `golang.org/x/net/route` on BSD/macOS, parsing `/proc/net/route`
  (or netlink) on Linux, and the IP Helper API on Windows — three distinct
  implementations behind one binding. **Reason:** no single pure-Go
  cross-platform route API; the per-OS effort outweighs current demand.
  Interface + address enumeration already ships as `net.capture.interfaces`.
  Re-promote when route inspection is actually needed.

### Networking — servers

- **HTTP/3.** stdlib's HTTP server auto-upgrades HTTP/1.1 to HTTP/2
  over TLS; H2 ships with the v0.10.0 HTTPS listener for free. H3
  would need `quic-go` and explicit handling. **Reason:** new
  dependency without a current demand signal. Re-promote when
  there's a clear request.
- **Let's Encrypt autocert.** `golang.org/x/crypto/acme/autocert`
  brings stateful disk caching, renewal coordination, and a
  side-effecting registration step — useful for production
  deployments but heavy for an embeddable script engine.
  **Reason:** sercon is CLI-first and the cert is best owned by
  the supervisor (caddy, traefik, nginx in front). Defer until
  clear demand.
- **Self-signed dev certificate generation.** A `cert: "self-signed"`
  magic option for `server.https.listen` would be a convenience for
  local dev but adds key-management complexity (cache the key, or
  regenerate every run?). **Reason:** small payoff for the design
  cost. Users can `openssl req -x509 …` once locally.
- **Server-Sent Events (SSE).** Could be a small helper on top of
  `server.http.listen` (set headers, flush on every write). YAGNI
  for now. **Reason:** no asks. Add when someone wants it.
- **Custom error pages / `server.http.onError(handler)`.** Handler
  throws today produce a stock `500 Internal Server Error` and a
  log line. A future hook for custom rendering can be added.
  **Reason:** no design pressure yet; the per-route try/catch
  pattern covers the common case.
- **Server-side IMAP, FTP.** Planned as separate sub-spec cycles
  built on the v0.10.0 `LoopCallable` + `HoldRun` foundation.
  (Server-side SMTP shipped in v0.11.0 as `server.smtp.listen`,
  with the outbound `net.email.send` sender.) Each remaining
  protocol gets its own brainstorm → plan → ship cycle. Promote
  from Deferred once the spec lands.
- **Server-side POP3.** Same foundation as the above, but
  **Reason:** no mature pure-Go POP3 server library exists today.
  Re-promote when one appears or when there's enough demand to
  justify writing one in tree.

### Packet capture

Sniffing shipped in v0.24.0 as `net.capture` — live capture
(`net.capture.open`, promiscuous mode), pcap/pcapng read (`openFile`) +
write (`toFile`), interface enumeration (`interfaces`), and full
layer decode (eth/ip/tcp/udp/icmp + payload) — all pure-Go via the
`gopacket` subset (`layers`/`pcapgo`/`bsdbpf`/`pcapgo.EthernetHandle`),
never the cgo `gopacket/pcap`. Live capture is Linux (AF_PACKET) + macOS
(BPF) only and needs root / CAP_NET_RAW / `/dev/bpf`. The parked pieces,
all blocked on the no-cgo rule or missing pure-Go prior art:

- **Windows live capture.** Needs Npcap → **cgo**. **Reason:** breaks the
  `CGO_ENABLED=0` build + static-binary releases. File read/write + decode
  already work on Windows; only live sniffing is stubbed out there.
- **Kernel-level BPF filtering.** A post-decode tcpdump-*syntax* `filter`
  shipped in v0.25.0 (`net.capture.open`/`openFile` `filter: "tcp and port
  80"`) — a pure-Go predicate evaluated in userspace after decode (subset:
  proto / host / port with src·dst, and·or·not·parens, implicit-and). What
  remains is true **kernel-level** drop: compiling the expression to a cBPF
  program (`x/net/bpf` can assemble instructions, but no pure-Go
  *expression→BPF compiler* exists — we'd write one) and attaching it at the
  socket. **Reason:** Linux could attach via `afpacket.SetBPF` /
  `SO_ATTACH_FILTER`, but macOS `bsdbpf` exposes no filter-attach API (would
  need forking it or raw `BIOCSETF`), so it's Linux-mostly + a from-scratch
  compiler. Re-promote if high-pps kernel drop is actually needed.
- **Filter grammar extensions.** `net X/Y` (CIDR) and `portrange A-B` are
  not in the v0.25.0 subset. Cheap to add to the post-decode evaluator on
  demand.
- **Deeper / exotic decode.** Only common layers map to fields today;
  other protocols surface as `bytes`. Extend `decodePacket` on demand.
