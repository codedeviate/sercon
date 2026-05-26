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

## Moderate

### CLI

- **Module-graph invalidation in `--watch`.** v0.5.9 shipped
  watch mode with a coarse-grained policy — every watched-file
  change re-runs every entry script. A smarter loop would build
  the module graph during the first Run, then on a file change
  re-run only the entries whose graph includes the touched file.
  **Approach:** capture each `require` call into a per-script
  graph; intersect against the changed-file set on each tick.
  Engine-level work, no new library.

### Transpile / entry rewriter

- **Robust import parsing.** `rewriteEntryESMToCJS` is a line scanner
  with a handful of regexes. It handles the cases in the test suite,
  but multi-line imports with comments, complex destructuring, or unusual
  whitespace are not guaranteed. A small AST-based parser (using
  `esbuild` Parse output, or a tiny hand-rolled one) would be more
  durable. **Library:** `github.com/evanw/esbuild` Parse API is the
  obvious lever; alternatively a hand-rolled tokenizer in a single Go
  file. Either way the design — what we extract and how we feed it back
  into the rewriter — is the moderate part.

### Require / module loading

- **Custom `PathResolver`.** Currently relies on
  `require.DefaultPathResolver`. Hosts that want sandboxed or virtualised
  module trees (in-memory FS, network sources) need to fall back to
  registering their own `Registry`, bypassing parts of the engine.
  **Library:** extends `github.com/dop251/goja_nodejs/require`; design
  work is around the resolver signature and how it interacts with the
  existing TS loader.

### Protocol probes & connectivity

- **`ping(host, count?)`** — TCP or ICMP ping with RTT min/avg/max
  and packet loss (script: `ping.rhai`). **Library:**
  `github.com/prometheus-community/pro-bing` (pure Go, maintained
  successor to `sparrc/go-ping`). Moderate because ICMP needs raw
  sockets / privileges on most platforms; TCP-ping fallback is
  straightforward `net.Dial`.
- **`smtp(url, opts?)`** — SMTP capability probe, STARTTLS availability,
  AUTH mechanisms (script: `smtp.rhai`). **Library:** `net/smtp`
  (stdlib) for the wire protocol; moderate because we need EHLO
  capability parsing and a clean output shape rather than a send
  pipeline.
- **`wss(url)`** — WebSocket handshake with ping/pong round-trip
  (script: `ws.rhai`). **Library:** `nhooyr.io/websocket` (now
  `github.com/coder/websocket`, pure Go, modern) or
  `github.com/gorilla/websocket` (classic). Moderate because exposing
  ping/pong timing through a one-shot JS call needs a small handle
  object lifecycle.
- **`netstatus::check()`** — Aggregate connectivity probe set (script:
  `netstatus.rhai`). **Approach:** orchestration on top of the other
  probes (`tcp`, `dns`, `tls`, `http`); no new library, but coordinates
  several bindings and concurrency.

### Remote services & caching

- **`redis(url, command?)`** — Redis RESP protocol client (PING or
  custom commands) (script: `redis.rhai`). **Library:**
  `github.com/redis/go-redis/v9` (pure Go, official); moderate because
  exposing arbitrary RESP commands to JS with sane argument coercion
  and result shaping needs design.
- **`memcached(url)`** — Memcached text protocol, version and stats
  (script: `memcached.rhai`). **Library:**
  `github.com/bradfitz/gomemcache/memcache` (pure Go, de facto
  standard).
- **`ldap(url)`** — Anonymous LDAP bind + RootDSE attribute query
  (script: `ldap.rhai`). **Library:**
  `github.com/go-ldap/ldap/v3` (pure Go, well-maintained).
- **`dict(url, word?)`** — RFC 2229 DICT protocol word lookup (script:
  `dict.rhai`). **Library:** no popular pure-Go DICT client; hand-roll
  the protocol over `net.Dial` (it is a simple line-based protocol).

### TLS / encryption / signing


### Encoding / decoding / barcodes

- **PDF417 decoder.** v0.5.4 shipped `api.barcode.decode` over
  gozxing, which doesn't cover PDF417 (the encoder still works via
  boombuler). A pure-Go PDF417 reader would close the symmetry. No
  obvious maintained library exists — porting ZXing's Java PDF417
  reader would be the realistic path. Defer until someone actually
  needs PDF417 round-tripping.

### Browser-like HTTP session

- **`browser()`** — Stateful HTTP session with automatic cookie jar
  and header replay. Methods: `set_user_agent`, `set_header`, `get`,
  `post` (Maps auto-serialised to JSON), `cookies` (script: `browser.rhai`).
  **Library:** `net/http` + `net/http/cookiejar` +
  `golang.org/x/net/publicsuffix` (stdlib-ish). Moderate because we are
  exposing a stateful session handle to JS with mutable headers and a
  cookie jar, which is more than a single function call.

### AI agent integrations

- **`ai::request()`** — Builder chain: `.system()`, `.context()`,
  `.prompt()`, `.timeout()`, `.send()` (script: `ai.rhai`). Requires
  one of `claude` / `codex` / `copilot` / `gemini` on `PATH`.
  **Library:** `os/exec` + `encoding/json` (stdlib). Moderate (rather
  than easy) because of (a) the builder API, (b) provider detection /
  selection logic, and (c) line-buffered streaming output if `.send()`
  ever grows a streaming variant.

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

### Agent-browser automation

`agent-browser` is recon's headless-Chrome driver. The recon script
bindings are extensive and worth a dedicated namespace
(e.g. `api.agentBrowser.*`). All of these require the
`agent-browser` CLI on `PATH`; gate calls on an
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
