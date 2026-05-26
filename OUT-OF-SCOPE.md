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

Every other Moderate item shipped across v0.5.0 – v0.5.30 (the
`.d.ts` JSDoc generator, `api.preg` / `api.preg2`, the full `api.jwt`
+ `api.encrypt` crypto surfaces, barcode decode + quiet-zone,
`api.http.request`, the `api.net` probe family, `api.netstatus`,
`api.browser`, `api.sqlite`, `api.redis` / `api.memcached` /
`api.ldap` / `api.dict`, `api.ai`, the `--watch` CLI flag with
module-graph invalidation, the `Options.ModuleLoader` hook, and
robust import parsing). The one remaining item has no library:

### Encoding / decoding / barcodes

- **PDF417 decoder.** v0.5.4 shipped `api.barcode.decode` over
  gozxing, which doesn't cover PDF417 (the encoder still works via
  boombuler). A pure-Go PDF417 reader would close the symmetry. No
  obvious maintained library exists — porting ZXing's Java PDF417
  reader would be the realistic path. Defer until someone actually
  needs PDF417 round-tripping.


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
