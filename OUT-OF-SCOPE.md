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
The one open Moderate item is **WebDriver / Selenium** (below); the
networking subsection further down is a shipped-record whose one residual
gap (route-table enumeration) is parked under Deferred.

### Browser automation — WebDriver / Selenium

> **v1 SHIPPED (v0.40.0, 2026-06-07).** `services.webdriver` is live: the
> connect-or-start-installed driver model, `available` (sync) + `probe({url})`
> (async) detection, stateful element handles, and the core automation loop
> (navigation, find/findAll, page source/screenshot, executeScript, cookies,
> waits). Sessions quit + started drivers stop on Run end; a per-session mutex
> serialises commands. Files: `cmd/sercon/webdriver*.go`. Design +
> v1 plan: `~/Development/Starweb/superpowers/sercon/specs/2026-06-07-webdriver-selenium-design.md`
> and the sibling v1 plan.
>
> **Phase 2 remains** (advanced, deferred — its own plan when picked up):
> window/tab handles + `switchToWindow`, frame switching, alerts, action chains
> (hover/drag/key-chords), file upload, window resize/maximize, and returning
> element handles from `executeScript`. The sketch below is retained as
> background.

A `db`-style stateful client for the W3C WebDriver protocol, complementing
the `services.agentBrowser` CLI bridge with a standards-based driver that
talks to any conforming endpoint (chromedriver, geckodriver, a
selenium-server grid, or a cloud provider's hub).

- **Library:** `github.com/tebeka/selenium` — the de-facto pure-Go WebDriver
  client (no cgo; speaks HTTP/JSON to a driver). It also ships helpers to
  start/stop a local `chromedriver`/`geckodriver`/`selenium-server`, but we
  would NOT bundle or auto-download any of those.
- **Feature-detected, trap when absent.** WebDriver needs a *running* driver
  or grid — not just a binary on `PATH` — so plain `exec.LookPath` isn't
  enough. Expose a `services.webdriver.available` (or a probe like
  `services.webdriver.probe({url})`) that checks for a reachable endpoint /
  a discoverable driver binary, and make every other binding trap with a
  clean thrown error when the prerequisite is missing — mirroring the
  `services.agentBrowser.available` gate. Decide whether "available" means
  "a driver binary is present" vs "a WebDriver URL responds", or both.
- **Shape (sketch, settle in brainstorm):** `services.webdriver.connect({url,
  browser, capabilities})` → a session handle (quit on Run end via
  `Engine.AddRunCleanup`, like the agentBrowser registry); element handles
  returned from `find*` and chained (`el.click()`, `el.text()`, `el.sendKeys()`);
  navigation / script-exec / screenshot / cookies / waits. Element-handle
  lifetime and the find→act model are the main design calls — reuse the
  agentBrowser locator decisions where they fit.
- **Why not Hard:** the Go client is stable and the agentBrowser work is
  prior art for the stateful-handle + feature-gate + Run-end-cleanup
  pattern, so the wiring is mechanical-with-design-choices, not from-scratch.
  The external *runtime* dependency (a live driver/grid) is what the
  feature-detection trap is for.

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

`agent-browser` is recon's headless-Chrome driver, exposed as
`services.agentBrowser.*` (alongside the other external-CLI wrappers git,
gh, ai). All calls require the `agent-browser` CLI on `PATH`; they gate on
the `services.agentBrowser.available` boolean and throw a clean error
otherwise. The bridge is `os/exec` + `--json` (no Go SDK exists; chromedp
is a different product). The original entry modelled the surface on the full
modern CLI; the design + phasing live in
`~/Development/Starweb/superpowers/sercon/specs/2026-06-07-agent-browser-automation-design.md`
and the Phase 1 plan alongside it.

**Phase 1 shipped (v0.36.0):** the subprocess bridge
(`abRun`/`abRunChecked`/`parseJSON`/`buildGlobalArgs`), synchronous
`launch(opts?)` returning a handle, per-Run session tracking with best-effort
close on Run end (`Engine.AddRunCleanup`), and the core loop — navigation
(`open`/`back`/`forward`/`reload`/`wait`/`connect`/`close`), interaction
(`click`/`dblclick`/`hover`/`focus`/`fill`/`type`/`press`/`check`/`uncheck`/
`select`/`scroll`/`scrollIntoView`/`drag`/`upload`/`download` +
`keyboard.*`/`mouse.*`), inspection (`get`/`isVisible`/`isEnabled`/`isChecked`/
`eval`/`snapshot`/`console`/`errors`/`highlight`), and locators (`find`
one-shot + `locator(spec)` handle). agent-browser wraps every result in a
`{ success, data, error }` envelope, surfaced verbatim.

**Phase 2 shipped (v0.37.0):** capture (`screenshot(path?,opts?)` /
`pdf(path?)`, path-first with opt-in in-memory bytes returned as a JS
`number[]`), `set.{viewport,device,geo,offline,headers,credentials,media}`,
`record.{start,stop}`, the namespace-level defaults bag
(`defaultOptions`/`setDefaultOptions`/`clearDefaultOptions`) merged into
`launch()`, and the flat one-shot shortcuts
(`screenshot(url,…)`/`pdf(url,…)`/`snapshot(url,…)`/`eval(url,js)`).

**Phase 3 shipped (v0.38.0):** network interception/monitoring
(`network.route`/`unroute`/`requests`/`request`/`har`), cookies
(`cookies.get`/`set`/`clear`), web storage (`storage.local`/`session`
get/set/clear), tab management (`tabs.list`/`new`/`close`/`select`), and page
diffing (`diff.snapshot`/`screenshot`/`url`) — all on a shared `runJSON`
handle helper. Also added a per-call subprocess timeout
(`launch({ timeout })`, default 30 s, `0` disables; `close()` bounded at 10 s)
so a wedged `agent-browser` command throws instead of hanging the script.

**Phase 4 shipped (v0.39.0) — feature complete:** debug/perf
(`trace`/`profiler`/`inspect`/`clipboard`/`vitals`/`pushstate`), React DevTools
(`react.tree`/`inspect`/`renders`/`suspense`, needs
`launch({ enable: "react-devtools" })`), live streaming
(`stream.enable`/`disable`/`status`), AI `chat(message, { model })`, the escape
hatch (`cmd(command, ...args)` / `batch(cmds, { bail })`), and the auth vault
(namespace `auth.save`/`list`/`show`/`delete` — passwords fed via
`--password-stdin`, never argv — plus handle `auth.login`).

**The `services.agentBrowser` feature is complete (Phases 1-4, v0.36.0–v0.39.0).**
New agent-browser subcommands can be reached today via the `cmd()` escape hatch;
promote a first-class binding here only if one is frequently used.

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
