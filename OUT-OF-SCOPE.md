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

## Easy

### Tooling / developer experience

- **Editor autocomplete wiring (VSCode / Zed / any tsserver editor).**
  The pieces already exist: `sercon -emit-dts` emits the top-level globals
  (`runtime`, `fs`, `http`, `net`, `db`, `crypto`, `text`, `codec`,
  `services`, `tui`, …) as ambient `declare const` blocks with JSDoc on
  every member, so any TypeScript
  language-server-backed editor (VSCode, Zed, Neovim+coc, Sublime LSP,
  …) gives completion + hover docs once the `.d.ts` is in its program.
  No per-editor plugin needed. The gap is the *glue* that makes editors
  pick it up without manual setup:
  - ship a `jsconfig.json` (or `tsconfig.json`) in `examples/scripts/`
    that includes `api.d.ts` — one file covers every tsserver editor;
  - document a `sercon -emit-dts api.d.ts` + tiny jsconfig recipe for
    users' own script directories;
  - optionally an `sercon init <dir>` helper that drops both in.
  Per-file `/// <reference path="./api.d.ts" />` is the no-config
  fallback. **Library:** stdlib only (file emit already exists).
### Databases

`db.sqlite` already proves the pattern: `database/sql` + a pure-Go
driver + an `open()`→handle shape (`exec` / `query` / `queryValue`,
see `cmd/sercon/api_sqlite.go`). Every other SQL engine below is the
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

Every other Moderate item shipped across v0.5.0 – v0.5.30 (the
`.d.ts` JSDoc generator, `text.preg` / `text.preg2`, the
full `crypto.jwt` + `crypto.encrypt` crypto surfaces,
barcode decode + quiet-zone, `net.http.request`, the
`net.probe` family, `net.netstatus`, `net.browser`,
`db.sqlite`, `db.redis` / `db.memcached` /
`db.ldap` / `db.dict`, `services.ai`, the `--watch` CLI
flag with module-graph invalidation, the `Options.ModuleLoader`
hook, and robust import parsing — all originally shipped under flat
paths and re-bucketed under v0.8.0's 9-category surface). The
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
a general socket surface (see `cmd/sercon/api_probe.go`). The gap is
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

All server work shares one genuinely hard problem: a
request/connection/packet arrives on a Go goroutine and has to invoke
a JS handler **back on goja's event loop**, the same marshalling
`shell_stream` needs (generalised for repeated callbacks, not a
one-shot resolve). That part is intrinsic and stays Hard.

The *other* concern — that `Engine.Run` builds a fresh event loop per
call and exits when `jobCount` hits zero, so a long-lived listener
needs a keep-alive (cf. `PromisifyAsync`'s sentinel) plus shutdown —
is largely a **library** concern, and sercon is CLI-first with library
use unsupported (see project memory). A dedicated `serve`-style command
can simply own the process: keep the loop alive trivially and exit on
Ctrl-C / signal, no embeddability gymnastics. So the lifecycle work is
smaller than it looks; the HTTP server in particular is closer to
Moderate once the callback-marshalling helper exists. ICMP raw sockets
(privileges) and the broad-protocol umbrella keep this group as a whole
in Hard.

- **HTTP / HTTPS server with optional router.** Serve script-defined
  handlers; optional path routing. **Library:** stdlib `net/http`
  with `http.ServeMux` (Go 1.22+ pattern routing covers most needs;
  a small router lib only if richer matching is wanted). Needs TLS
  config, graceful shutdown, and JS handler dispatch on the loop.
- **TCP / UDP / ICMP servers (listeners).** Accept loops that hand
  each connection or packet to a JS callback. **Library:** stdlib
  `net` for TCP/UDP; `golang.org/x/net/icmp` for ICMP (raw sockets,
  so elevated privileges). Listener lifecycle within the per-Run loop
  is the crux.
- **Client + server for the common internet protocols.** Umbrella
  goal: broad protocol coverage on both sides. Clients already ship
  for several (`net.probe.{dns,tls,ntp,whois,smtp,wss}`,
  `net.http`, plus `db.redis` / `db.memcached` /
  `db.ldap` / `db.dict`); the gaps are server-side
  counterparts and additional protocols (e.g. FTP, IMAP / POP3,
  MQTT). Rated Hard for the aggregate scope, not any single
  protocol — promote individual protocols as they're actually needed
  rather than tackling the umbrella wholesale.

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
  Hard rather than easy because invoking a JS callback from a Go
  goroutine has to be marshalled back onto goja's event loop safely
  (this is the same machinery `PromisifyAsync` already uses, but
  generalised for repeated callbacks rather than a one-shot resolve).

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
