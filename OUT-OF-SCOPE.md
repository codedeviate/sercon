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

A fifth bucket, **Deferred**, lives at the bottom of this file. Items
land there when there's a concrete reason to put the work down rather
than rank it by effort — no trustworthy pure-Go library exists, the
feature depends on an external runtime we don't want to require yet,
the design space is unsettled, or it conflicts with current direction.
Each Deferred entry names the reason so it's easy to re-promote when
the situation changes.

## Trivial

### Correctness / API hygiene

- **`runtime.assert.equal` does reference equality, not deep equality.**
  Its JSDoc (`cmd/sercon/docs.go`) claims "deep equality on objects", but
  `valuesEqual` (`cmd/sercon/main.go`) is just goja `StrictEquals`, so two
  distinct objects with identical contents never compare equal. Surfaced
  while writing the dump-codec example (v0.12.0), which had to work around
  it with a `JSON.stringify` projection. Either implement structural deep
  equality (recursive over arrays/objects, the more useful fix) or correct
  the JSDoc to say reference equality. Pick a direction and align doc +
  impl. **Library:** stdlib / goja only.

### Script ergonomics

- **`console` object pretty-printing.** The `console` shim (v0.14.0) and
  `runtime.log` stringify each argument via goja `.String()`, so
  `console.log({a:1})` prints `[object Object]` rather than the
  browser-style expansion. Optionally pretty-print non-primitive args
  (JSON, or a shallow inspect) for the browser/Node-porting use case —
  while keeping primitives byte-identical and guarding against circular
  refs. Decide whether `runtime.log` follows suit or only `console.*`.
  **Library:** stdlib / goja only.

## Easy

### Tooling / developer experience

- **Editor autocomplete wiring (VSCode / Zed / any tsserver editor).**
  The pieces already exist: `sercon -emit-dts` emits the ten reserved
  top-level globals (`codec`, `crypto`, `db`, `fs`, `net`, `runtime`,
  `server`, `services`, `text`, `tui`) as ambient `declare const` blocks with
  JSDoc on every member, so any TypeScript language-server-backed
  editor (VSCode, Zed, Neovim+coc, Sublime LSP, …) gives completion +
  hover docs once the `.d.ts` is in its program. No per-editor plugin
  needed. The gap is the *glue* that makes editors pick it up without
  manual setup:
  - ship a `jsconfig.json` (or `tsconfig.json`) in `examples/scripts/`
    that includes `sercon.d.ts` — one file covers every tsserver editor;
  - document a `sercon -emit-dts sercon.d.ts` + tiny jsconfig recipe for
    users' own script directories;
  - optionally an `sercon init <dir>` helper that drops both in.
  Per-file `/// <reference path="./sercon.d.ts" />` is the no-config
  fallback. **Library:** stdlib only (file emit already exists).

### Databases

`db.sqlite` already proves the pattern: `database/sql` + a pure-Go
driver + an `open()`→handle shape (`exec` / `query` / `queryValue`,
see `cmd/sercon/sqlite.go`). Every other SQL engine below is the
same handle wired to a different `database/sql` driver and a DSN, so
the marginal cost per engine is small. Open question to settle once
(not blocking): one DSN-driven `db.open(driver, dsn)` vs. a
sibling namespace per engine inside `db` (`db.mysql` /
`db.postgres` / …). All drivers named are **pure Go (no cgo)**.

- **MySQL / MariaDB.** One driver covers both (MariaDB speaks the
  MySQL wire protocol). **Library:** `github.com/go-sql-driver/mysql`
  (the de facto standard).
- **PostgreSQL.** **Library:** `github.com/jackc/pgx` via its
  `stdlib` `database/sql` adapter (modern, maintained; `lib/pq` is the
  older alternative). CockroachDB and other Postgres-wire engines come
  along for free.
- **Microsoft SQL Server.** **Library:**
  `github.com/microsoft/go-mssqldb` (pure Go).
- **Other easy wins (same pattern, pure-Go drivers exist).**
  ClickHouse (`github.com/ClickHouse/clickhouse-go`), Oracle
  (`github.com/sijms/go-ora` — pure Go, unlike cgo-bound godror), and
  Snowflake (`github.com/snowflakedb/gosnowflake`). Add on demand.


## Moderate

### Correctness / output stability

- **Canonical (stable-key-order) JSON for the remaining object-returning
  bindings.** goja derives a JS object's property order from Go map
  iteration (randomized per process), so a binding returning a
  `map[string]any` shuffles `JSON.stringify` output run-to-run — breaking
  callers that hash a canonical serialization (payment signing, webhook
  signatures). **Done so far:** `text.preg`/`text.preg2` (v0.11.2,
  ordered `*goja.Object`) and the *unconditional-key* results
  `net.probe.tcp`/`ping`/`smtp`/`wss`, `db.dict.*`, `services.gh.authStatus`,
  `services.git.*` (converted to json-tagged structs — struct field order is
  deterministic and the engine already sets `goja.TagFieldNameMapper`).
  **Why the rest is Moderate, not Easy:** structs only work when *every*
  key is always present. goja exposes **every** struct field as a JS
  property regardless of `json:",omitempty"` (omitempty affects only
  `JSON.stringify`, not `"key" in obj` or `obj.key`), so a struct breaks
  the conditional-presence contract (e.g. `if ("mx" in dnsResult)`) — see
  `TestNetDNS_TypesFilter`. The remaining bindings have conditional,
  dynamic, or decoded-JSON keys and need **ordered on-loop object
  construction** (`vm.NewObject()` + `.Set()` only the present keys), which
  for the async ones means threading an ordered structure through
  `PromisifyAsync` (it currently resolves with `vm.ToValue`, off-loop-built):
  - conditional presence: `net.probe.dns`/`whois`, `net.netstatus`,
    `server.smtp` envelope optionals;
  - dynamic keys: `db.ldap` entries (attribute names), `db.sqlite` query
    rows (column names), `server.smtp` message `headers`;
  - decoded external JSON (needs order-preserving decode): `crypto.jwt.view`
    header/payload, `text.jq`, `services.gh` pr-list/repo-view,
    HTTP JSON response bodies.
  **Library:** stdlib / goja only.

Every other Moderate item shipped across v0.5.0 – v0.5.30 (the
`.d.ts` JSDoc generator, `text.preg` / `text.preg2`, the
full `crypto.jwt` + `crypto.encrypt` crypto surfaces,
barcode decode + quiet-zone, `net.http.request`, the
`net.probe` family, `net.netstatus`, `net.browser`,
`db.sqlite`, `db.redis` / `db.memcached` /
`db.ldap` / `db.dict`, `services.ai`, the `--watch` CLI
flag with module-graph invalidation, the `Options.ModuleLoader`
hook, and robust import parsing — all originally shipped under flat
paths, re-bucketed under v0.8.0's 9-category surface, and promoted
to top-level globals (dropping the `api.` prefix) in v0.9.0). The
remaining open items:

### Encoding / decoding / barcodes

- **PDF417 decoder.** v0.5.4 shipped what is now
  `codec.barcode.decode` over gozxing, which doesn't cover
  PDF417 (the encoder still works via boombuler). A pure-Go PDF417
  reader would close the symmetry. No obvious maintained library
  exists — porting ZXing's Java PDF417 reader would be the realistic
  path. Defer until someone actually needs PDF417 round-tripping.

### Networking — clients & raw sockets

Today's `net.probe.*` family is **connect-probe** oriented, not
a general socket surface (see `cmd/sercon/probe.go`). The gap is
read/write client sockets exposed to scripts.

- **TCP client sockets.** `net.probe.tcp` only reports
  reachability / latency; a real client would expose a connection
  handle with `write` / `read` (or a data callback) and `close`.
  **Library:** stdlib `net`. Moderate for the usual reason — a
  stateful handle with lifetime and event-loop concerns, not the
  dial itself.
- **UDP client sockets.** No binding today. **Library:** stdlib
  `net` (`net.DialUDP` / `ListenUDP` for the reply socket).
- **ICMP client.** `net.probe.ping` already does an ICMP echo
  round trip; a general send/receive ICMP surface (other message
  types, custom payloads) is the extension. **Library:**
  `golang.org/x/net/icmp` (pure Go), but raw ICMP sockets need
  elevated privileges on most platforms — worth noting in the API.
- **General Go `net` access.** Direct dial / lookup / interface
  enumeration primitives beyond the curated probes. Mostly already
  covered piecemeal by `net.*`; promote only if a script needs
  raw access the probe family doesn't expose.


## Hard

### Engine

- **Source-map-aware error mapping.** `transpile.go` claims this in its
  package comment but esbuild's source maps are not yet wired into
  `*goja.Exception` stack frames. Errors in `.ts` scripts currently point
  at the transpiled JS line numbers. **Library:**
  `github.com/go-sourcemap/sourcemap` is already in `go.sum`, but the
  hard part is wiring it through goja's exception path and rewriting
  every frame consistently, not parsing the map itself.
- **Top-level export capture.** `Engine.Run` resolves with whatever
  `__resolve` receives, which is always `undefined` today. Wiring the
  entry-script body so its trailing expression flows into the resolve
  call would let hosts get a return value back. **Approach:** engine
  internals — no library; needs design for both ESM `export default`
  and bare trailing-expression cases.
- **True `RegisterConstructor` runtime semantics.** The d.ts emitter
  produces `declare class`, but at runtime the constructor is treated
  like a plain `vm.Set`. Hooking it up so `new Foo(...)` works in JS
  and respects the returned Go type's methods is open work.
  **Approach:** goja `Runtime.ToValue` + prototype wiring + reflect; no
  drop-in library exists for this.

### Networking — servers & broad protocol coverage

The two engine primitives that earlier made server work Hard —
`LoopCallable` (loop-bound callback marshalling for repeated
invocations) and `Engine.HoldRun` (long-lived `Run` keep-alive) —
shipped in v0.10.0 alongside the first protocol family (HTTP/HTTPS).
Per-protocol surface area remains in this list; each is its own
sub-spec cycle built on the existing foundation.

- **TCP / UDP / ICMP servers (listeners).** Accept loops that hand
  each connection or packet to a JS callback. The HTTP listener
  exercises the TCP path implicitly; raw TCP / UDP / ICMP server
  surfaces are not exposed to scripts. **Library:** stdlib `net`
  for TCP/UDP; `golang.org/x/net/icmp` for ICMP (raw sockets, so
  elevated privileges). Listener lifecycle uses the same
  `HoldRun` + `LoopCallable` pattern as the HTTP server.
- **Client + server for the common internet protocols.** Umbrella
  goal: broad protocol coverage on both sides. Clients already ship
  for several (`net.probe.{dns,tls,ntp,whois,smtp,wss}`,
  `net.http`, plus `db.redis` / `db.memcached` /
  `db.ldap` / `db.dict`); the HTTP/HTTPS server (with router,
  middleware, static-file mount, and WebSocket upgrade) shipped in
  v0.10.0 and the SMTP server (`server.smtp.listen`) plus outbound
  sender (`net.email.send`) shipped in v0.11.0; IMAP, FTP, and POP3
  servers are planned as separate sub-spec cycles using the v0.10.0
  foundation. Additional
  protocols (e.g. MQTT) and broader client coverage stay rated Hard
  for the aggregate scope — promote individual protocols as they're
  actually needed.

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

### External tool integrations

- **`agent-browser`.** Headless-Chrome automation; see the dedicated
  section above. Optional dependency, gated on `agentBrowser.available`.
  **Library:** `os/exec` (stdlib) — but rated Hard because of the scope
  of the surface area, not because any single call is difficult.
- **`shell_stream(cmd, cb)`** — Stream stdout/stderr line by line into
  a JS callback (script: `shell.rhai`). Needed as a building block for
  the recon-fallback path. **Library:** `os/exec` (stdlib) + `bufio`.
  Now Moderate rather than Hard: v0.10.0 introduced
  `scriptengine.NewLoopCallable` (loop-bound callback marshalling
  for repeated invocations) and `Engine.HoldRun` (keeps the loop
  alive while the subprocess is in flight) — the engine work is done
  and only the binding remains.

## Deferred

Items here aren't ranked by difficulty — they're parked for a stated
reason. Move them back into Trivial / Easy / Moderate / Hard once the
reason resolves.

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

- **ODBC connectivity.** A generic ODBC bridge would reach any engine
  with a system DSN, but the only real Go option,
  `github.com/alexbrainman/odbc`, links the platform ODBC driver
  manager (unixODBC on Linux/macOS) via **cgo** — and needs that
  manager plus a per-engine ODBC driver installed at runtime. That
  conflicts with the no-cgo constraint on the platforms that matter
  most here. **Reason:** no pure-Go ODBC implementation exists.
  Re-promote if one appears, or skip ODBC entirely once the native
  pure-Go drivers (MySQL / Postgres / MSSQL / Oracle / …, see the
  Databases group under Easy) cover the engines people actually ask
  for — which they largely do.

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
