# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
See [CLAUDE.md](./CLAUDE.md) for the project's commit-message conventions.

## [Unreleased]

## [0.85.0] — 2026-07-03

### Added
- Size-cap guards against decompression-/decode-bomb resource exhaustion:
  `net.http.request` and every `web.*.load` accept `maxBytes` (response body
  cap, default 256 MB); `server.http.listen` / `server.https.listen` accept
  `maxBodyBytes` (inbound request body cap, default 32 MB, `413` on exceed);
  `codec.compression.decompress` accepts `maxBytes` (decompressed output cap,
  default 512 MB); `fs.archive.extract` accepts `maxTotalBytes` (default
  1 GB) and `maxEntries` (default 100000).
- `image.decode` / `image.open` (and `codec.barcode.decode`, which decodes
  through the same path) now reject a source whose declared width × height
  exceeds a hard 64-megapixel cap before allocating the pixel buffer; not
  configurable.
- kcov3 payment-provider mutations (`acknowledge`, `capturePayment`,
  `refundPayment`, `cancelPayment`, `releaseRemainingAuthorization`) accept
  an optional `idempotencyKey` so a caller can pin a stable key across
  retries instead of getting a fresh, non-deduping key per call.

### Fixed
- `codec.php` / `codec.perl` dump decoders now cap recursion depth instead
  of stack-overflow crashing on deeply nested untrusted input.
- `sercon serve` now performs a *real* graceful shutdown on SIGTERM/SIGINT:
  every active listener's close hook (HTTP/HTTPS `Server.Shutdown`, SMTP
  `Server.Close`, TCP/UDP/ICMP socket close) runs concurrently within
  `--shutdown-timeout` before the run context is force-cancelled as a
  fallback — previously the handler just waited out the timer without ever
  closing the listeners.
- SMTP listener close is now safe against a concurrent double-close.
- The favro library is now importable under `sercon run` / `sercon serve`
  (previously failed to resolve).
- A retried kcov3 capture/refund could double-charge because each call
  generated its own fresh idempotency key by default; see the `idempotencyKey`
  option above.
- `PromisifyAsync` work now observes the current Run's context (timeout and
  cancellation) instead of `context.Background()`, so async bindings abort
  in step with the rest of the script.
- Documentation accuracy: the false "`Run`'s return value is always
  `undefined`" claim (§3.3); `services.pdf.available`'s gating binary
  (`pdftoppm`, not `pdfinfo`); the missing `-emit-reference PATH` flag in the
  §4 flags table; `services.doctor`'s tool list and `requires` categories
  omitting `pdf`/poppler; the §5.7 `net.capture` summary omitting `routes()`;
  the §3.1 `Options` struct block omitting `ProgramName`/`WatchMode`; a
  §4.6 "Recipes" heading nested one level too deep; the `version.go` header
  comment still describing an automated release-please bump instead of the
  manual `make release-prep` flow; and CLAUDE.md's async-iterator polyfill
  key (`Symbol.for("Symbol.asyncIterator")`, not `"@@asyncIterator"`).

## [0.84.3] — 2026-07-03

### Documentation
- Backlog polish sweep: normalized the net/services/web recipe cross-links to
  the canonical §17 binding reference (matching the other recipes); aligned the
  KCO/Klarna idempotency header casing in the docs to the code's actual
  lowercase `klarna-idempotency-key`; and scoped the §6 chapter-intro `port: 0`
  claim to the raw TCP/UDP listeners (HTTP/HTTPS/SMTP require an explicit port).

## [0.84.2] — 2026-07-03

### Documentation
- Manual cookbook expansion (continued): Concepts primers + Recipes added
  across the data & crypto reserved globals — `crypto` (§5.3), `text` (§5.4),
  `codec` (§5.5), `db` (§5.8), `image` (§5.11) — and CLI & workflow patterns
  (§4.6). Recipes are illustrative and derived from the runnable example
  scripts; signatures cross-link the generated §17 reference.
- New getting-started/orientation layer in §2 Quickstart: a §2.1 Concepts
  primer (mental model + a map table of every reserved global → purpose →
  deep-section link) and §2.2 Recipes (first script, arguments/environment, an
  HTTP call, file I/O, and discovering the API surface).
- Corrected the reserved-global count from "twelve" to "thirteen" (the `audio`
  global, §5.13, was added after the count was written) in §2/§4.1/§5 and the
  `registerSurface` comment, and completed the stale §4.3 declaration list
  (added `image`, `web`, `audio`).

## [0.84.1] — 2026-07-03

### Documentation
- Manual cookbook expansion: task-oriented Concepts primers + Recipes added
  across the manual, following a new per-section recipe convention (recorded in
  CLAUDE.md). Covers the embedded libraries — `favro` (§16.2) and
  `paymentproviders` (§16.1) — and the I/O & network reserved globals: `fs`
  (§5.6), `net` (§5.7), `server` (§6), `web` (§5.12), and `services` (§5.9).
  Recipes are illustrative and derived from the runnable example scripts.

## [0.84.0] — 2026-07-02

### Added
- `net.http.request` now accepts a binary `body` (`Uint8Array` / `ArrayBuffer`, sent byte-for-byte) and a `multipart` option for `multipart/form-data` uploads (text fields and file parts, assembled in-process; sets the `Content-Type` header). `body` and `multipart` are mutually exclusive.
- Embedded `favro` library: `import { client } from "favro"` — a full client for the Favro API (organizations, collections, widgets, columns, cards, comments, tasks, tasklists, tags, custom fields, groups, webhooks) with auto-pagination (`list`/`listAll`/`iterate`), token-first Basic auth + `organizationId`, bounded opt-out 429 retry, `FavroError`, and multipart attachment upload.

## [0.83.0] — 2026-06-30

### Added
- `codec.dotenv.parse` / `codec.dotenv.stringify` — pure dotenv
  parser/serializer (round-trip-safe), and `runtime.env.load(path, { override? })`
  — read a .env file and apply it to the process environment (async; already-set
  vars win unless override). All reuse the `--env-file` parser. Pure-Go.

## [0.82.0] — 2026-06-30

### Added
- New `codec.doc` namespace: extract text from PDF, DOCX, DOC (Word 97–2003),
  RTF, and ODT (`codec.doc.read` → `{ format, text, paragraphs }`), and write
  DOCX/RTF/ODT (`codec.doc.write`). PDF and DOC are read-only (writing throws).
  `codec.doc.formats()` reports the read/write matrix. Pure-Go.

## [0.81.2] — 2026-06-28

### Added
- `xlsx-workbook.ts` example recipe: combine `sales.csv` + `regions.tsv` into a
  multi-sheet XLSX with typed cells, then read it back to confirm the sheets and
  numeric types survive (which CSV/TSV can't represent).

## [0.81.1] — 2026-06-28

### Added
- Example recipes for the new features: `stego-capacity.ts` (multi-bit LSB
  capacity), `stego-detect.ts` (steganalysis via `detect`/`analyze`), and
  `sheet-legacy-convert.ts` (read a legacy SYLK export and convert it to XLSX),
  plus a `legacy.slk` sample-data file.

## [0.81.0] — 2026-06-28

### Added
- `codec.sheet` reads three legacy formats **read-only** — XLS (Excel
  97–2003, via pure-Go extrame/xls), SYLK (`.slk`), and DIF (`.dif`) — for
  extracting data and converting up. Writing them throws a read-only error.
- `codec.sheet.formats()` returns the read/write capability matrix.

## [0.80.0] — 2026-06-28

### Added
- `image.stego` and `audio.stego` gain a `bits` option (1..4, default 1) for
  multi-bit LSB embedding — up to 4× capacity. The depth is stored in the stego
  header, so `extract` auto-detects it; existing 1-bit payloads are unaffected.
- `*.stego.capacity` accepts `{ bits }` and returns `{ bytes, bits }`.
- `image.stego.detect`/`analyze` report the declared `bits`; `analyze` adds a
  generalized chi-square at depths 1..4 (`chiSquareByBits`), per-plane entropy
  (`entropyByPlane`), and a coarse `estimatedBits` hint (best-effort; needs
  substantial coverage — the header's declared `bits` is authoritative for
  sercon payloads).

## [0.79.0] — 2026-06-28

### Added
- `audio.info` / `audio.decode` / `audio.encode` / `audio.convert` — read WAV/FLAC/MP3/OGG/AIFF and write WAV/FLAC/AIFF via a canonical 16-bit PCM model. Pure-Go.

## [0.78.0] — 2026-06-28

### Added
- `text.stego.embed` / `text.stego.extract` — hide payloads in cover text via zero-width characters (U+200B/U+200C), optional AES-256-GCM.
- New `audio` global: `audio.stego.embed` / `extract` / `capacity` — WAV PCM (8/16-bit) LSB steganography, optional AES-256-GCM. Pure-Go, no new dependency.

## [0.77.0] — 2026-06-28

### Added
- `image.stego.detect` / `image.stego.analyze` / `image.stego.bitplane` — read-only LSB steganalysis: sercon-payload detection, per-channel chi-square + LSB entropy + RS embedding-rate, a suspicion verdict, and bit-plane PNG visualization. Pure-Go, no new dependency.

## [0.76.0] — 2026-06-27

### Changed
- The script runner is now quiet on success across all modes (bare `sercon a.ts b.ts`, `sercon run`, and `--watch`): the per-script `PASS <name> (<time>)` line prints only with `-v`/`--verbose`. Failures still print `FAIL …` by default.

### Added
- `-v` gained a `--verbose` long alias; new `--silent` flag suppresses the runner's PASS/FAIL status lines (exit code and script output unaffected).
- `codec.toml.parse` / `codec.toml.stringify` — TOML ↔ object (pure-Go, `pelletier/go-toml/v2`).
- `examples/data/` sample-data corpus and `examples/recipes/` — eight task-shaped example scripts (sales report, config read, image pipeline, format convert, inventory, log scan, stego, barcode batch) that consume the corpus.

## [0.75.0] — 2026-06-27

### Added
- `image.stego.embed` / `image.stego.extract` / `image.stego.capacity` — LSB steganography in PNG (1 bit per R/G/B channel, alpha untouched), with optional AES-256-GCM (PBKDF2) payload encryption. Pure-Go, no new dependency.

## [0.74.0] — 2026-06-26

### Added
- `codec.sheet` ODS (OpenDocument Spreadsheet) read + write — typed cells (numbers, booleans, ISO-8601 date strings on read), pure-Go (no new dependency).

## [0.73.0] — 2026-06-26

### Added
- `codec.sheet.read` / `codec.sheet.write` — tabular CSV/TSV + XLSX, typed cells.

## [0.72.0] — 2026-06-26

### Added
- `image.decodeFrames` / `image.encodeFrames` — animated GIF + APNG: decode all frames into a normalized frame model (`AnimDoc`), encode a frame set back to GIF (256-color, Floyd–Steinberg dithering) or full-color APNG.

## [0.71.0] — 2026-06-26

### Added
- `image` handle `orient(n)` — apply one of 8 EXIF pixel orientations (1..8); throws `TypeError` on invalid `n`. `{ autoOrient: true }` option on `image.open` / `image.decode` reads the source EXIF `Orientation` tag and returns upright pixels (no-op when absent, never throws); strip-on-save is automatic.

## [0.70.0] — 2026-06-25

### Added
- `image.exif.read` / `image.exif.write` / `image.exif.replace` / `image.exif.clear` — synchronous EXIF read and write for JPEG/PNG (write targets); read is broad (JPEG/PNG/TIFF full + HEIC/AVIF/RAW curated). Tags grouped by IFD (`image`/`exif`/`gps`/`thumbnail`); rationals as `[num,den]`, GPS as signed decimals, binary values as base64.

## [0.69.1] — 2026-06-25

### Changed
- `MANUAL.md`: consistent hierarchical heading numbering across the whole
  manual (`N` / `N.M` / `N.M.K` / `N.M.K.L`) so the table of contents reflects
  the chapter hierarchy. Previously only chapter 6 numbered its subsections.
  The generated binding reference (§17) is numbered by the generator;
  chapters 1–16 are hand-numbered.
- `MANUAL.md`: `Bundled libraries` is now numbered `§16` and the generated
  `Binding reference` becomes `§17` (it was an unnumbered chapter wedged
  between §15 and §16). Dropped the stale "Migration from v0.8.0" subsection
  that interrupted the reserved-globals chapter (the v0.8.0→v0.9.0 rename is
  still recorded in the `[0.9.0]` entry).

### Added
- `Engine.WriteReferenceNumbered(w, sectionPrefix)` — emits the markdown
  binding reference with hierarchical section numbers (`### <prefix>.<N>`,
  `#### <prefix>.<N>.<M>`). `WriteReference` is unchanged (unnumbered).

## [0.69.0] — 2026-06-25

### Added
- `services.pdf.*` — poppler-backed PDF render/extract (`available`, `backend`,
  `tools`, `version()`, `info()`, `toImage()`, `toText()`, `toHtml()`),
  feature-detected and enrichment-only. Backed by poppler-utils (pdftoppm /
  pdftotext / pdftohtml / pdfinfo); registered in `services.doctor` under a
  `pdf` category.
- Shared external-CLI helper (`runTool` / `toolAvailable` / `safePathArgs`):
  no-shell exec with timeout, output cap, and a `--` flag-injection guard.

### Changed
- `services.typst` now runs through the shared external-CLI helper
  (behaviour-preserving internal refactor).

## [0.68.0] — 2026-06-24

### Added

- `text.str.base64UrlEncode` / `text.str.base64UrlDecode` — URL-safe base64
  (RFC 4648 §5, `-`/`_` alphabet). Encode emits no padding (safe in URLs,
  filenames, JWT segments); decode accepts both padded and unpadded input.
  Complements the standard-alphabet `base64Encode`/`base64Decode`.

### Fixed

- `server` WebSocket: a peer close frame's code and reason are now surfaced on
  the socket object as `ws.closeCode` / `ws.closeReason` once the message
  iterator ends (previously discarded).
- `codec.barcode.decode` auto-detect (no `format` hint) now also tries `upce`,
  which was decodable only via an explicit hint before.

## [0.67.0] — 2026-06-24

### Changed

- Manual rendered via recon's **typst** engine: a `--cover` title page, page-number
  footer, page-numbered table of contents, and a sans-serif body (recon-native
  **IBM Plex Sans**, no vendored font). `make manual` no longer needs `--unsafe-html`;
  MANUAL.md's raw-HTML cover and hand-curated TOC were removed, and the file is piped
  through `scripts/typst-safe.awk` at render (escapes prose angle brackets + strips
  HTML comments outside code) so the source stays tooling-friendly. `version-check`/
  `release-prep` rework to a single footer version marker.
- Whole-manual documentation sweep: every binding member across all reserved-global
  namespaces (+ console, server) now carries full structured docs (params, return
  type, returns, errors, example) — enriching both the §16 reference and the `.d.ts`
  hovers. `paymentproviders` gained a full per-provider reference. A permanent
  `TestDocsComplete` guard keeps every member documented.

## [0.66.0] — 2026-06-23

### Added

- `web` — a new reserved global for fetching & parsing web documents, each from a
  string or a URL: `web.feed` (RSS/Atom/JSON normalized to one model + `.raw`
  escape hatch), `web.sitemap` (urlset/sitemapindex, transparent gzip,
  `{expand:true}` bounded single-level expansion), and `web.html` (lenient HTML parse —
  real-world tag soup welcome — with chainable nodes queryable by CSS
  `find`/`findAll` and XPath `xpath`/`xpathAll`). `load()` reuses the `net.http`
  option surface with a default `sercon-web/<version>` User-Agent and throws on
  non-2xx. New deps: gofeed, cascadia, antchfx/htmlquery (all pure-Go).

## [0.65.0] — 2026-06-23

### Added

- `paymentproviders` Cycle 3 — `swedbankpayv2` + `swedbankpayv3` (SwedbankPay
  Checkout v2/v3): Bearer auth + HAL/hypermedia operations. `createPaymentOrder`,
  `getPaymentOrder`, an `operation(paymentOrderOrUrl, rel, body?)` primitive, and
  `capturePayment`/`refundPayment`/`cancelPayment` (resolve the operation href and
  POST). Adds `core/hal.ts` (findOperation) and absolute-URL support in the core
  HTTP helper. Creds from `SWEDBANKPAY_*` env. Completes the planned provider set.

## [0.64.0] — 2026-06-23

### Added

- `paymentproviders` Cycle 2 — three more providers on the shared core (now with a
  pluggable per-request auth signer): `netsv1` (Nexi/Nets Checkout Payment API v1,
  secret-key header), `sveacheckout2` (Svea Checkout, SHA512 signature + Timestamp
  header), `qlirov2` (Qliro One, `Qliro base64(SHA256(body+secret))`). Versioned
  namespaces (`<provider><version>`) so new API versions ship alongside. Creds from
  `NETS_*`/`SCO_*`/`QLIRO_*` env.

## [0.63.0] — 2026-06-23

### Added

- `paymentproviders` — a TypeScript payments library compiled into the binary,
  imported as `import { kcov3 } from "paymentproviders"`. Cycle 1 ships **KCO v3**
  (Kustom): `getPayment`/`acknowledge`/`capturePayment`/`refundPayment`/
  `cancelPayment`/`releaseRemainingAuthorization` + bonus `createCheckout`/
  `getCheckout`, over a shared core (HTTP via `net.http`, Basic auth, idempotency
  keys, `PaymentError`). Credentials from `KCO_MERCHANT_ID`/`KCO_SHARED_SECRET`/
  `KCO_ENV` (env or `.env`). Nets/Svea/Qliro/SwedbankPay planned as later cycles.

## [0.62.0] — 2026-06-21

### Added

- `fs` file primitives (all async): `fs.writeText`, `fs.writeBytes`, `fs.readText`,
  `fs.readBytes` (→ Uint8Array), `fs.mkdir` (mkdir -p), `fs.exists`, `fs.remove`
  (file or tree), `fs.stat` (`{ size, isDir, modifiedMs }`). Writes fail if the
  parent dir is missing (Node-like). Enables building reports/artifacts from a
  script — see the new `fs-report.ts` example (per-step screenshot report).

## [0.61.0] — 2026-06-21

### Added

- `services.webdriver` `cdpClick` now activates elements inside true
  out-of-process iframes (OOPIFs) — cross-*site* iframes such as a Klarna
  Checkout — by dispatching input over a browser-level CDP connection. (v0.60.0
  only handled same-process iframes; a different port is same-site and stays
  in-process.)
- `services.webdriver` session `targets()` and `attach(target)` →
  `{ targetId, sessionId, cdp(method, params?), detach() }` — a scriptable
  browser-level CDP target/session API (Chrome-only).

## [0.60.0] — 2026-06-21

### Added

- `services.webdriver` session `cdpClick(by, value, opts?)` — a trusted click on
  an element inside a nested cross-origin iframe (the Klarna "Pay order" case),
  where the W3C Element Click hit-tests to the parent iframe and is intercepted.
  Locates across the pierced frame tree and dispatches a CDP
  `Input.dispatchMouseEvent` at the element's true viewport coords. Chrome-only.
- `services.webdriver` session `cdp(command, params?)` — raw Chrome DevTools
  Protocol escape hatch (Chrome-only).

## [0.59.0] — 2026-06-21

### Added

- `services.webdriver` `clickWhenReady(by, value, opts?)` — wait for an element
  in the active frame to be present (and, by default, visible + enabled) within a
  timeout, then issue a native (trusted) click that fires React handlers. With
  `switchToFrame`/`frameChain` this reliably drives buttons that render or enable
  asynchronously inside nested cross-origin iframes (e.g. Klarna Checkout). Also:
  `waitFor` gains an `enabled` option (wait past a disabled→enabled transition).
  (Investigating feedback 0004 confirmed `find` already follows the active frame
  — the reported "find doesn't follow the chain" was a timing race, not a frame
  bug.) New `webdriver-wait-click.ts` example.

## [0.58.1] — 2026-06-20

### Docs
- `HISTORY.md`: record the `doctor` feature — `--doctor` in §2 (CLI) and
  `services.doctor(requires?)` in §4 (services), incl. the chromedriver↔Chrome
  compatibility check and the `requires` assertion model; span line advanced to
  v0.58.0.

## [0.58.0] — 2026-06-20

### Added

- `doctor` — external-requirements diagnostics. `sercon --doctor` prints a
  category-grouped report of every optional external tool (git, gh, AI providers,
  agent-browser, chromedriver/geckodriver, typst, recon/curl, clipboard/image
  tools): installed?, version, purpose — and validates the chromedriver↔Chrome
  major-version match. Exits 0 normally (missing tools are fine — they're
  optional) and 5 on a detected compatibility conflict. `services.doctor(requires?)`
  returns the same report as `{ ok, satisfied, unmet, tools }`; pass an array of
  feature names (`"webdriver"`, `"typst"`, `"ai"`, …) or specific binaries to
  assert a script's prerequisites (`unmet` lists what's absent/conflicted; an
  unknown name throws). New `doctor.ts` example.

## [0.57.0] — 2026-06-20

### Added
- Browser iframe support (feedback 0003). `services.webdriver` gains first-class
  nested-frame addressing: `switchToFrame` now also accepts a **CSS selector**
  (find-then-switch), and a new `frameChain([...])` switches from the top
  document through each nested level in one call (queries are frame-scoped after
  a switch). Uses the W3C `/frame` protocol, so **cross-origin** nested frames
  (e.g. a Klarna Checkout inner iframe) work. `services.agentBrowser` launch
  handles gain `frame(target)` (CSS selector / `@ref` / `"main"`) for a CDP frame
  switch — **single-level only** (agent-browser resolves the selector against the
  main document and can't descend into nested frames; use WebDriver for nesting).
  New `webdriver-frames.ts` / `agent-browser-frames.ts` examples.

### Fixed
- `services.webdriver` `switchToFrame` by element handle (and the new selector
  form) now actually switches. It previously posted a `/frame` body with only
  the W3C element key, which chromedriver accepts (2xx) but silently ignores, so
  the switch no-op'd (only the frame-index form worked) — this matches the
  long-standing "switchToFrame behaved ambiguously" report. The element/selector
  paths now switch via a dual-key web-element reference / tebeka `SwitchFrame`.

## [0.56.0] — 2026-06-20

### Added
- `--env-file PATH` (repeatable) — load `KEY=VALUE` pairs from a `.env` file into
  the environment before running, so `runtime.env.get` and any spawned subprocess
  see them. Parses `KEY=VALUE`, `#` comments, blank lines, an optional leading
  `export`, and optional surrounding quotes (no shell expansion). A variable
  already in the real environment always wins; among multiple files a later file
  overrides an earlier one. Replaces the `set -a; source .env; set +a` ritual and
  makes shebang test scripts self-sufficient. (Feature request 0001.)
- `bodyBytes: Uint8Array` on `net.http.get` / `net.http.post` / `net.http.request`
  responses — the raw, undecoded response bytes alongside the UTF-8 `body`. Lets a
  script byte-verify or charset-decode non-UTF-8 content (e.g.
  `text.charset.decode(r.bodyBytes, "ISO-8859-1")`) instead of losing å/ä/ö to
  U+FFFD. (Feature request 0002.)

## [0.55.0] — 2026-06-20

### Added

- `runtime.setDeadline(ms)` / `runtime.clearDeadline()` / `runtime.getDeadline()`
  — control the running script's own wall-clock timeout (the `-timeout` deadline)
  at runtime: `setDeadline(ms)` moves the kill deadline to now + ms (ms<=0
  disables), `clearDeadline()` removes it, `getDeadline()` returns ms remaining
  or null. Distinct from the JS global `setTimeout` (which schedules a callback).
  Backed by new exported `Engine.SetRunTimeout` / `Engine.RunTimeoutRemaining`
  and a resettable Run watcher. New `deadline.ts` example.

## [0.54.0] — 2026-06-18

### Added
- `services.typst` — external-CLI binding to the Typst compiler (feature-detected
  via `available`): `version()`, `fonts()`, `compile(opts)` (inline `source` or a
  `.typ` `input` → PDF bytes, or write PDF/PNG/SVG to `output`; `root`/`inputs`/
  `ppi`/`fontPaths`), and `query(opts)` (selector → JSON). Throws cleanly when
  `typst` isn't installed. New `typst.ts` example. (Embedding `typst-as-lib` is
  impossible under no-cgo, so this is the external-CLI path.)

## [0.53.0] — 2026-06-18

### Added
- `image` — a new top-level global for image I/O and manipulation. Decode
  PNG/JPEG/GIF/TIFF/BMP/WebP (and rasterize an SVG subset), a chainable,
  synchronous `Image` handle (`resize`/`fit`/`thumbnail`/`crop`/`rotate*`/
  `flip*`/`brightness`/`contrast`/`gamma`/`saturation`/`sharpen`/`blur`/
  `grayscale`/`invert`/`overlay`/`paste`), and encode via `bytes(format)` /
  `save(path)` (PNG/JPEG/GIF/TIFF/BMP/WebP). Pure-Go (imaging + x/image +
  nativewebp + oksvg). New `image.ts` example.

### Fixed
- `make manual`: recon 0.101.0 changed the `--md-to-pdf` default engine to
  typst, which rejects `--unsafe-html` (needed for MANUAL.md's HTML cover /
  page-breaks) — breaking the manual render and every release cut. Pin
  `--pdf-engine chrome` to restore the Chrome render path.

## [0.52.4] — 2026-06-17

### Docs
- `OUT-OF-SCOPE.md`: track verifying the Windows `runtime.clipboard` paths
  (text + image) as a follow-up — implemented but not yet executed (macOS and
  Linux X11/Wayland are verified end-to-end). Windows isn't a focus right now.

## [0.52.3] — 2026-06-17

### Docs
- `HISTORY.md`: record the v0.52.0 `runtime.clipboard` PNG image support (and the
  v0.52.1/v0.52.2 fixes) and `net.load.http` in §4; advance the span line to
  v0.52.2.

## [0.52.2] — 2026-06-17

### Fixed
- `runtime.clipboard` write hung on Linux. `xclip` / `wl-copy` fork a daemon to
  own the X11/Wayland selection, and that child inherited our captured-stderr
  **pipe**, so `cmd.Wait()` blocked until the (never-exiting) daemon closed it —
  every `write()` / `writeImage()` stalled until the timeout. Route clipboard
  write subprocesses' stdout/stderr to `os.DevNull` (an `*os.File`, so `os/exec`
  spawns no copier goroutine and `Wait` returns when the parent exits). Verified
  end-to-end on Linux in Docker: X11 (`xclip` under Xvfb) and Wayland (`wl-copy`
  under headless sway) both round-trip text **and** PNG images. macOS
  (`pbcopy`/`pngpaste`/`osascript`) unaffected and re-verified.

## [0.52.1] — 2026-06-17

### Fixed
- `examples/scripts/clipboard.ts`: the image round-trip used a 1×1 PNG, which
  fails on macOS — the clipboard re-encodes via CoreGraphics, and
  `CGImageDestinationFinalize` can't produce a PNG from a degenerate 1×1 image
  (so `pngpaste` read nothing). Use a real 16×16 PNG instead. Verified end-to-end
  on macOS with `pngpaste`: `writeImage` → `readImage` returns a valid PNG. The
  `runtime.clipboard.readImage`/`writeImage` bindings themselves were correct.

## [0.52.0] — 2026-06-16

### Added
- `net.load.http(opts)` — an authorized HTTP load / resilience self-test
  harness: worker-pool load at a given `concurrency` for a `requests` count or
  `duration`, optional `rps` cap, returning a report (`sent`/`completed`/`failed`,
  achieved `rps`, `errorRate`, latency `min/mean/p50/p90/p95/p99/max`,
  `statusCounts`, `errors`). Dual-use guardrail: public targets are refused
  unless `confirm:true` (loopback/private hosts always allowed); concurrency
  capped at 1000. Defensive self-testing only. New `load.ts` example.
- `runtime.clipboard` image (PNG) support: `imageAvailable` (advisory),
  `readImage(): Promise<Uint8Array | null>`, `writeImage(png): Promise<void>`
  (PNG validated on write). Backends: macOS `pngpaste` (read) + `osascript`
  (write), Linux `wl-clipboard`/`xclip` (`-t image/png`), Windows PowerShell.
  Feature-detected; throws cleanly when no image backend is present. macOS image
  read requires `pngpaste`.

## [0.51.2] — 2026-06-16

### Docs
- `HISTORY.md`: record the v0.51.0 `runtime.clipboard` capability in §4
  (`runtime`) and advance the span line to v0.51.1.

## [0.51.1] — 2026-06-16

### Docs
- `MANUAL.md` §4: harmonize the `runtime.clipboard` entry with `runtime.secrets`
  — bullet + nested member sub-bullets instead of a `####` subheading. No API
  change; the code example remains in the §16 reference / `sercon.d.ts`.

## [0.51.0] — 2026-06-16

### Added
- `runtime.clipboard` — host OS system clipboard text I/O: `available`
  (advisory), `read(): Promise<string>`, `write(text): Promise<void>`. An
  external-CLI fallback (macOS `pbcopy`/`pbpaste`, Linux `wl-clipboard` or
  `xclip`/`xsel`, Windows `clip` + PowerShell), feature-detected on PATH; throws
  cleanly and reports `available: false` when no backend is installed. Text
  only; image support is deferred (see OUT-OF-SCOPE.md). New `clipboard.ts`
  example.

## [0.50.2] — 2026-06-16

### Docs
- `HISTORY.md`: backfill the v0.42–v0.49 capabilities into their subsystem
  sections — `net.capture` filter grammar + `routes()`, `server.https`
  self-signed cert, `server.http`/`https` `onError`, webdriver `commandTimeout`,
  the d.ts/§16 async `ReturnType` fix, `runtime.secrets` + `--secrets-prefix`,
  and `db.valkey`. The header span now reads through v0.50.1 with no
  coverage-gap caveat.

## [0.50.1] — 2026-06-16

### Docs
- `HISTORY.md`: record the v0.50.0 capabilities — `res.sse()` in §5 (Servers)
  and the ARP/VLAN/DNS + TCP-options decode enrichment in the §4 `net.capture`
  entry. The header notes that v0.42–v0.49 are not yet woven in (see this
  changelog for their per-version detail).

## [0.50.0] — 2026-06-16

### Added
- `res.sse(opts?)` on the HTTP/HTTPS listener: a one-way Server-Sent Events
  (`text/event-stream`) stream with `send()` (a string, or
  `{event, data, id, retry}` with object data JSON-encoded), `close()`, a
  `closed` Promise (resolves on close or client disconnect), and optional
  `keepAlive` / `retry`. Unlike `res.upgradeWebSocket` the connection isn't
  hijacked, so the request dispatcher parks until the stream closes; a pump
  goroutine owns the writer and flushes each event. New `server-sse.ts` example.

- Deeper packet decode in `net.capture` / `net.raw` handlers: the decoded
  packet object now surfaces `arp` (operation + sender/target MAC & IP), `vlan`
  (802.1Q id/priority/drop/inner-type), and `dns` (id, qr, opcode, rcode,
  `questions[]`, `answers[]` with type-aware `data`) as structured fields, and
  the `tcp` layer gains `window`, `checksum`, and a parsed `options` object
  (`mss`, `windowScale`, `sackPermitted`, `timestamps`). All additive — keys
  appear only when that layer decodes. `packet-analysis.ts` exercises ARP + DNS.

### Docs
- Four new advanced example scripts: `advanced/sse-stream.ts` (live metrics
  over `res.sse`), `advanced/sqlite-migration.ts` (idempotent versioned
  migration + three-table JOIN/aggregation), `advanced/webdriver-actions.ts`
  (raw W3C `performActions` pointer + key sequences), and
  `advanced/webdriver-grid.ts` (remote / Selenium Grid WebDriver via
  `connect({url})`). The first two run in `make demo` (the migration also in
  CI); the WebDriver pair self-skips without a driver / grid URL.
- Network-dependent demos are now resilient to a flaky/offline network: a new
  shared `examples/scripts/helpers/netskip.ts` recognises transport/DNS/TLS and
  proxy-HTML failure signatures, and the external-host demos (`async`,
  `net-probe`, `exec-http`, `email-auth`, `http-request`, `browser`) self-skip
  (exit 0) on those while re-throwing genuine errors — so `make demo` stays
  green when the network is unreachable but still catches real regressions.

## [0.49.1] — 2026-06-11

### Added
- Opt-in integration tests for the networked `db.*` bindings (postgres, mysql,
  mariadb, clickhouse, redis, valkey, memcached, ldap) against the
  [dbplayground](https://github.com/codedeviate/dbplayground) fleet, gated on
  `SERCON_TEST_*` env vars (unset → skip; set-but-unreachable → fail). New
  `make test-integration` brings the fleet up, runs them, and tears it down. A
  CI workflow (`.github/workflows/integration.yml`) runs it on push / PR to
  master, with `ci` + `integration` status badges in the README. Test/CI-only —
  the shipped binary and script API are unchanged from 0.49.0.

## [0.49.0] — 2026-06-11

### Added
- `db.valkey` — a client for Valkey, the RESP-compatible open-source Redis
  fork. Same `open(url) → { do, ping, close }` surface as `db.redis` (it reuses
  the same pure-Go go-redis client), and additionally accepts the
  Valkey-idiomatic `valkey://` / `valkeys://` URL schemes (normalised to
  `redis://` / `rediss://`). Added ahead of Valkey support in the planned
  Docker DB test stack.

## [0.48.1] — 2026-06-11

### Fixed
- `runtime.secrets.get/set/delete` now validate their arguments: a missing or
  empty `name`, a missing `account` (pass `""` for a single-secret name), or a
  missing `secret` (set) rejects with a clear error instead of silently keying
  the literal keystore service `"<prefix>undefined"`. An explicit empty
  `account` remains valid.

## [0.48.0] — 2026-06-11

### Added
- `runtime.secrets` — read/write string credentials in the OS keystore (macOS
  Keychain, Linux Secret Service / libsecret, Windows Credential Manager),
  pure-Go (no cgo) via `zalando/go-keyring`. `available` (advisory bool),
  `get(name, account) → Promise<string | null>`, `set(name, account, secret) →
  Promise<void>`, `delete(name, account) → Promise<boolean>`. All operations
  are confined to a prefix namespace (keystore service = `PREFIX + name`),
  with `PREFIX` from `--secrets-prefix` > `SERCON_SECRETS_PREFIX` > default
  `sercon/`. The sws6 example now prefers the keystore over `DEVSHOP_PASSWORD`.
- `--secrets-prefix` CLI flag (and `SERCON_SECRETS_PREFIX` env) to set the
  `runtime.secrets` namespace prefix.

### Fixed
- `.d.ts` / §16 reference: the §16 binding-reference generator now honours
  `MemberDoc.ReturnType` for async (`PromisifyAsync`) bindings, completing the
  v0.47.0 fix (which had covered only the d.ts emitter). Async bindings with a
  documented return type — e.g. `services.webdriver.connect`,
  `net.probe.traceroute`, `runtime.secrets.get` — now render their rich
  `Promise<…>` type in MANUAL.md §16 instead of `Promise<unknown>`.

## [0.47.0] — 2026-06-09

### Fixed
- `.d.ts` generation now honours `MemberDoc.ReturnType` for `PromisifyAsync`
  (async) bindings. Previously the emitter always wrapped the binding's marker
  type — usually `Promise<unknown>` — discarding the documented return shape,
  so async bindings like `services.webdriver.connect`, `net.probe.traceroute`,
  and others rendered as `Promise<unknown>` in `examples/scripts/sercon.d.ts`
  even though the rich type was present in the §16 reference. The documented
  `ReturnType` is now emitted (verbatim when already `Promise<…>`-wrapped,
  wrapped otherwise); undocumented async bindings still fall back to the
  marker type. Closes the tracked "`.d.ts` AsyncBinding `ReturnType` gap".

## [0.46.0] — 2026-06-09

### Added
- `services.webdriver.connect` accepts `commandTimeout` (ms, default 30000):
  a per-request deadline on the low-level W3C command client.

### Fixed
- WebDriver raw commands no longer hang indefinitely. Each `s.command` request
  now carries a context with the `commandTimeout` deadline, and `quit()` /
  Run-end cleanup cancels any in-flight command — so a driver blocked behind
  an open alert or an unreachable endpoint fails promptly instead of wedging
  the call. (`net.probe.*` already threaded `context.WithTimeout`, so the rest
  of the tracked "no timeout / no cancellation" follow-up was already covered;
  the item is now closed.)

## [0.45.0] — 2026-06-09

### Added
- `net.capture.routes()` — synchronous, unprivileged snapshot of the host's
  IP routing table: an array of `{ destination, gateway, interface, family,
  metric }`. `destination` is a CIDR (`0.0.0.0/0` / `::/0` for defaults),
  `gateway` is the next-hop IP or `""` for directly-connected routes. Pure-Go,
  no cgo: Linux parses `/proc/net/route` + `/proc/net/ipv6_route`; macOS/BSD
  read the routing socket via `golang.org/x/net/route`; Windows is stubbed
  (throws). Sits beside `net.capture.interfaces()`. Closes the
  "Route-table enumeration" backlog item.

## [0.44.0] — 2026-06-09

### Added
- `server.http.listen` / `server.https.listen` accept an optional
  `onError(err, req, res)` handler, invoked when a route handler or
  middleware throws or rejects, in place of the stock `500 Internal Server
  Error`. Render any response via the usual `res.*` terminals; the handler
  may be `async`. If it settles without finalizing a response, or itself
  throws/rejects, sercon falls back to the stock 500 — a buggy error handler
  can't wedge the request. `err` carries the original thrown value (so
  `err.message` matches `throw new Error(...)`). Closes the "Custom error
  pages / server.http.onError" backlog item.

## [0.43.0] — 2026-06-09

### Added
- `server.https.listen` accepts `cert: "self-signed"` to mint an ephemeral
  P-256 certificate in-process (no openssl, no committed PEM, nothing written
  to disk; `key` is then optional). SANs cover `localhost` / `127.0.0.1` /
  `::1` plus the listen host. Self-signed certs fail normal client
  verification by design — a local-dev convenience, not a production path
  (own production certs in your supervisor). `examples/scripts/advanced/
  https-server.ts` now uses it instead of an embedded PEM. Closes the
  "Self-signed dev certificate generation" backlog item.

## [0.42.0] — 2026-06-09

### Added
- `net.capture` filter grammar: `net X/Y` (IPv4/IPv6 CIDR prefix, with
  optional `src`/`dst`) and `portrange A-B` (inclusive, with optional
  `src`/`dst`). Both compose with the existing `and`/`or`/`not` + parens
  and implicit-and. Applies to `capture.open`, `capture.openFile`, and
  `net.raw.open`'s `filter`. Still a pure-Go, post-decode userspace
  predicate (not kernel BPF). Closes the "Filter grammar extensions"
  backlog item.
- `examples/scripts/advanced/` — a curated set of 12 in-depth, end-to-end
  example scripts composing multiple bindings into realistic workflows:
  `load-resilience` (authorized load/resilience self-test), `http-api`
  (middleware + auth + CRUD + WebSocket-less API), `smtp-pipeline`,
  `tcp-proxy`, `https-server` (inline self-signed cert), `sqlite-etl`,
  `crypto-pipeline`, `codec-interop`, `packet-analysis`, `recon-host-report`
  (self-skips offline), `webdriver-login-flow` (self-skips without a driver),
  and `tui-dashboard` (manual). Self-contained ones run in `make demo`; the
  three deterministic ones are also in the CI offline subset.
- `examples/scripts/sws6/` — reality-based `services.webdriver` flows against an
  internal dev storefront (search, browse-category, filter-sort, login,
  add-to-cart, view-cart, checkout-payment + a `shop.ts` helper). Secrets
  (credentials, payment test data) are read from the environment via a
  gitignored `.env` (see `sws6/.env.example`); `connectShop()` uses a normal
  desktop UA so the shop issues its session cookie. Self-skip when no driver or
  host; not in `make demo`/CI (internal host).

## [0.41.0] — 2026-06-08

### Added
- `services.webdriver` Phase 2: window/tab handles (`windowHandles`/`currentWindow`/
  `switchToWindow`/`newWindow`/`closeWindow` with auto-switch to a survivor),
  frame switching (`switchToFrame` by index or element, `switchToParentFrame`,
  `switchToDefaultContent`), alert handling (`acceptAlert`/`dismissAlert`/
  `alertText`/`sendAlertText`), window rect (`maximize`/`minimize`/`fullscreen`/
  `setWindowRect`/`getWindowRect`), real W3C action chains (`hover`/`dragAndDrop`/
  `keyChord`/`performActions`/`releaseActions`, plus element `hover()`/`dragTo()`),
  and `executeScript`/`executeScriptAsync` returning element handles (top-level
  and top-level-array refs). Built on an internal raw-W3C-command primitive,
  since tebeka/selenium's legacy mouse API is rejected by modern W3C drivers.

## [0.40.1] — 2026-06-08

### Fixed
- `services.webdriver.connect` with no `url` (start an installed local driver)
  failed for Chrome with `got content type "text/plain", expected
  "application/json"`. tebeka's `NewChromeDriverService` starts chromedriver
  with `--url-base=wd/hub`, so it only answers under `/wd/hub`, but the dial URL
  was built without that prefix — the `POST /session` 404'd as `text/plain`.
  The dial URL now matches each driver's url-base (`/wd/hub` for chromedriver,
  root for geckodriver). Firefox was unaffected (geckodriver uses no url-base).
- `webElement.getAttribute(name)` now returns `null` for an absent attribute
  instead of throwing `nil return value`. tebeka maps the W3C "attribute
  absent → JSON null" response to that sentinel error; sercon now restores the
  DOM `getAttribute`-returns-null semantics. This unbreaks Firefox scripts that
  read an attribute Chrome would expose as a property (e.g. an input's typed
  `value` — read live properties via `executeScript`).

### Changed
- Manual: documented the underlying external-tool surface for
  `services.agentBrowser` and `services.webdriver` so §5 is a self-contained
  one-stop reference. For agentBrowser, added the argument vocabularies passed
  through to the CLI — `get` what-values (`text`/`html`/`value`/`attr`/`title`/
  `url`/`count`/`box`/`styles`/`cdp-url`), the `is*` states, the `find`
  locators (`role`/`text`/`label`/`placeholder`/`alt`/`title`/`testid`/`first`/
  `last`/`nth`) and act-verbs, `snapshot` opts, and `@ref` selectors — plus a
  pointer to the version-matched `agent-browser --help` / `skills` reference and
  the `cmd()` escape hatch (current for agent-browser 0.27.1). For webdriver,
  documented the browser-flag (`args`) and W3C `capabilities` surface (standard
  capability keys, `goog:chromeOptions`/`moz:firefoxOptions` blocks, and the
  last-wins merge semantics) with a link to the W3C spec.

## [0.40.0] — 2026-06-07

### Added
- `services.webdriver` — a W3C WebDriver client (via `github.com/tebeka/selenium`,
  pure Go). `available`/`probe({url})` feature detection; `connect(opts?)`
  attaches to a running driver/grid url or starts an installed local
  `chromedriver`/`geckodriver`; stateful element handles from `find`/`findAll`;
  navigation, page source/screenshot, `executeScript`, cookies, and waits.
  Sessions quit on Run end. Drivers are never bundled or downloaded.

## [0.39.0] — 2026-06-07

### Added
- `services.agentBrowser` Phase 4 (final): debug/perf (`trace`, `profiler`,
  `inspect`, `clipboard`, `vitals`, `pushstate`), React DevTools (`react.tree`/
  `inspect`/`renders`/`suspense`, with `launch({ enable: "react-devtools" })`),
  live streaming (`stream.enable`/`disable`/`status`), AI `chat`, the escape
  hatch (`cmd(command, ...args)` / `batch(cmds, { bail })`), and the auth vault
  (namespace `auth.save`/`list`/`show`/`delete` with passwords fed via stdin,
  plus handle `auth.login`).

## [0.38.0] — 2026-06-07

### Added
- `services.agentBrowser` Phase 3: network interception/monitoring
  (`network.route`/`unroute`/`requests`/`request`/`har`), cookies
  (`cookies.get`/`set`/`clear`), web storage (`storage.local`/`session`
  get/set/clear), tab management (`tabs.list`/`new`/`close`/`select`), and
  page diffing (`diff.snapshot`/`screenshot`/`url`).
- Per-call subprocess timeout for `services.agentBrowser`: `launch({ timeout: <ms> })`
  (default 30 000 ms, `0` disables) so a wedged `agent-browser` command throws
  instead of hanging the script. Session `close()` is independently bounded at
  10 s regardless of this setting.

## [0.37.0] — 2026-06-07

### Added
- `services.agentBrowser` Phase 2: capture (`screenshot`/`pdf`, path-first
  with opt-in in-memory bytes), `set.*` settings
  (viewport/device/geo/offline/headers/credentials/media), `record.*` video,
  a namespace-level `defaultOptions`/`setDefaultOptions`/`clearDefaultOptions`
  bag merged into `launch()`, and flat one-shot shortcuts
  (`screenshot(url,…)`/`pdf(url,…)`/`snapshot(url,…)`/`eval(url,js)`).

## [0.36.0] — 2026-06-07

### Added
- `services.agentBrowser` — bridge to the `agent-browser` headless-Chrome
  CLI. Phase 1: `available`/`version`, synchronous `launch(opts?)` handle
  with best-effort session close on Run end, navigation
  (open/back/forward/reload/wait/connect), interaction
  (click/fill/type/press/check/select/scroll/drag/upload/download +
  keyboard/mouse), inspection (get/is*/eval/snapshot/console/errors/
  highlight), and locators (find one-shot + locator handle).

## [0.35.1] — 2026-06-03

### Added
- TUI mouse mode: a left-click now focuses the pane under the cursor.

### Fixed
- TUI mouse mode: the wheel now scrolls the pane under the cursor instead of
  the focused pane, and no longer changes focus while scrolling.

## [0.35.0] — 2026-06-02

### Added
- `services.exec.shell` accepts `{ pty: true }` (Unix) to run the command
  under a pseudo-terminal so it emits color/progress — rendered into a pane
  or captured into `stdout`. The general alternative to per-tool force-color
  flags. Windows falls back to the pipe path (no color).
- Per-pane `tui` layout options: `wrap` (`"char"` | `"word"` | `"off"`,
  default `"char"`) controls line wrapping, and `color` (boolean, default
  true) strips a pane's ANSI to plain text when set to `false`. Both affect
  TTY rendering only.
- `tui` panes now autoscroll to follow the tail by default; opt a pane out
  with `{ autoscroll: false }` on its leaf.
- `tui.layout({ mouse: true })` enables mouse-wheel scrolling of panes
  (disables native click-drag selection while active).
- `tui.waitKey()` resolves with the next keypress; `tui.onKey(handler)`
  registers a persistent per-keypress callback returning an unsubscribe
  function. Both require a TTY.
- `Engine.AbortRun()` cancels the in-flight Run via the engine's
  interrupt path (used to wire the TUI's Ctrl-C).

### Fixed
- TUI `Ctrl-C` now aborts the script on a single press; previously the
  first press only tore down the screen and a second was needed to exit.
- Terminal detection now uses `term.IsTerminal` instead of an
  `os.ModeCharDevice` check, so a non-terminal character device such as
  `/dev/null` correctly takes the non-TTY fallback path. Previously a TUI
  script with stdout redirected to `/dev/null` was misclassified as
  interactive and segfaulted in the tcell mouse path.

## [0.34.0] — 2026-06-01

### Added
- **`net.raw.open({ iface?, filter?, readBuffer? })`.** Raw IPv4 packet
  engine: craft and send TCP (arbitrary flags), UDP, or arbitrary-IP-protocol
  packets with full IP-header control (source IP, TTL, IP-ID), and receive
  decoded replies via a tcpdump-filtered capture. Needs root / CAP_NET_RAW;
  Linux + macOS only.
- **`net.raw.tcp(host, port, opts?)`.** One-shot raw TCP probe: send a crafted
  segment (default SYN) and resolve with the first correlated reply packet
  (SYN/ACK = open, RST = closed) or null on timeout.

## [0.33.0] — 2026-06-01

### Added
- **`net.probe.traceroute(host, opts?)`.** Trace the network path with ICMP,
  UDP, or TCP probes (TTL-stepped), reporting each responding router as
  `{ ttl, address, rttsMs, reached }`. Replies are correlated to probes via the
  quoted packet inside each ICMP `time-exceeded`. Needs root / CAP_NET_RAW.
- **`net.probe.ping` `mode: "udp"`.** A UDP datagram to a closed port whose
  ICMP `port-unreachable` proves reachability (needs root / CAP_NET_RAW),
  alongside the existing `tcp`/`icmp` modes.

## [0.32.0] — 2026-06-01

### Added
- **`codec.xml.encode(value, opts?)` / `codec.xml.decode(xml)`.** Value ↔ XML
  codec using the `@`-prefix (attributes) + `#text` (text) convention: child
  elements are plain keys, arrays become repeated sibling elements, a text-only
  element collapses to a bare string, and an empty/self-closing element ↔
  `null`. `opts.rootName` / `indent` / `declaration` on encode; decoded values
  are strings; object key order and namespace prefixes are preserved.
  Mismatched tags, multiple roots, and malformed XML throw. Built on the shared
  dump IR (order preservation + cycle detection).

## [0.31.1] — 2026-06-01

### Fixed
- Removed the documented-but-ignored `opts.readBuffer` from
  `server.icmp.listen` (the listener uses a direct read loop with no buffered
  channel, like `server.udp`). The d.ts and MANUAL no longer advertise a
  no-op option.

## [0.31.0] — 2026-06-01

### Added
- **`server.icmp.listen(opts?, (msg, reply) => …)`.** Raw ICMP listener
  counterpart to the `net.icmp` client: receives all host ICMP traffic (needs
  root / CAP_NET_RAW) and `reply(opts?)` sends an ICMP message back to the
  sender (or `opts.to`), reusing the `net.icmp` Echo/raw-body send options.
  Synchronous bind + `{ address, close() }` handle like `server.tcp`/`udp`;
  emits a READY line under `sercon serve`.

## [0.30.0] — 2026-06-01

### Added
- **Entry `export default` capture.** An entry script's `export default <expr>`
  is now the value `Engine.Run` resolves to, and the `sercon` CLI prints it as
  JSON to stdout (scripts without a default export are unchanged). `export`
  statements in the entry no longer error.
- **`RegisterConstructor` runtime semantics.** `new Foo(...)` now runs the
  registered Go constructor, coerces JS arguments to its parameter types,
  exposes the result's methods/fields, and throws when the constructor returns
  a non-nil error.

### Fixed
- Entry imports that follow an esbuild helper preamble (emitted when the entry
  declares a top-level `function`) are now converted to `require()` instead of
  leaking a raw ESM `import` that goja rejected.

## [0.29.1] — 2026-05-31

### Fixed

- **Empty / comment-only entry scripts no longer fail to run.** v0.29.0
  attached an inline source map to the entry script unconditionally; an
  empty, whitespace-only, or comment-only script transpiles to a body with
  no source-map segments, and goja rejects an attached map with empty
  mappings (`mappings are empty`). The map is now skipped when there is
  nothing to map.

## [0.29.0] — 2026-05-31

### Changed

- **Source-mapped error positions.** Runtime errors in `.ts` scripts now
  report TypeScript line/column numbers instead of transpiled-JS positions —
  for both the entry script and imported `.ts`/`.tsx` modules, and for both
  synchronous throws and async (top-level-`await`) rejections. Implemented
  with inline source maps that goja consumes natively; the entry script's
  ESM→CJS rewrite is line-shift-aware. If a map is unavailable, traces fall
  back to transpiled-JS positions (never worse).
- Script error strings now include the **full** stack: previously async
  rejections were message-only and synchronous throws showed only the top
  frame. `errors.As(err, **goja.Exception)` still works (the wrapper unwraps
  to the underlying `*goja.Exception`).

## [0.28.0] — 2026-05-31

### Added

- **`services.exec.stream(cmd, onLine, opts?)`.** Run a subprocess and stream
  its stdout/stderr to `onLine(line, stream)` line by line as output arrives,
  instead of buffering like `exec.shell`. Resolves
  `{ exitCode, success, durationMs }` on exit (non-zero exit → `success:false`;
  spawn failure / timeout reject; a non-function `onLine` throws). `cmd`,
  `cwd`, `env`, and `stdin` match `exec.shell`; `timeout` has **no default**
  (0 / absent = run until exit), since streaming targets long-running output.
  Hand-rolled on `scriptengine.NewLoopCallable` + `Engine.HoldRun`.

## [0.27.0] — 2026-05-31

### Added

- **`net.icmp` raw (non-Echo) message bodies.** `handle.send()` gains a
  raw-body mode: pass `body` (`Uint8Array | string`) and the message is
  marshalled verbatim via `icmp.RawBody` instead of being forced into an
  Echo (`id`/`seq`/`payload`) layout, so scripts can hand-build non-Echo
  messages such as a crafted destination-unreachable. In raw mode `type` is
  required and `body` is mutually exclusive with `id` / `seq` / `payload`;
  omitting `body` preserves the existing Echo behaviour exactly. Opts
  parsing + validation is factored into the privilege-free
  `parseICMPSendOpts` so the rules are unit-tested without a raw socket.

## [0.26.0] — 2026-05-31

### Added

- **Structured binding-doc model + generated reference (all namespaces).**
  `scriptengine` gains `MemberDoc{Summary, Params, ReturnType, Returns,
  Errors, Example}` as the single source of truth for binding docs. The
  d.ts emitter now renders real signatures + `@param`/`@returns` from it
  (e.g. `crypto.hash.sha256(input: string): string` instead of `(...args)`),
  and a new `sercon --emit-reference` / `Engine.WriteReference` generates a
  markdown reference spliced into MANUAL.md's `## 16. Binding reference`
  section (via `make reference`, which `make manual` now runs). **All
  eleven namespaces** (`runtime`, `crypto`, `text`, `codec`, `fs`, `net`,
  `db`, `server`, `services`, `tui`, `console`) are fully documented —
  every function's parameters (name, type, optional, meaning), return
  shape, thrown errors, and an example — cross-checked against the
  implementations. `docs.go` is split into per-namespace `docs_<ns>.go`
  files; `SetMemberDocs(map[string]string)` still works (wraps as
  `MemberDoc{Summary}`).

## [0.25.0] — 2026-05-31

### Added

- **`net.capture` `filter` option — tcpdump-syntax packet filtering.**
  Both `net.capture.open({ iface, …, filter? }, pkt => {…})` and
  `net.capture.openFile(path, pkt => {…}, { filter? })` (the `filter` lives
  in a **new trailing opts argument** on `openFile`; the two-argument form
  still works) now accept an optional tcpdump-like expression string.
  Supported subset: protocols `tcp` / `udp` / `icmp` / `ip` / `ip6`;
  `host X` / `src host X` / `dst host X` (IPv4 or IPv6); `port N` /
  `src port N` / `dst port N`; `and` / `or` / `not` and parentheses; and
  implicit-and between juxtaposed primaries (`tcp port 80` ==
  `tcp and port 80`). The filter is evaluated **post-decode in userspace**
  — it is **not** compiled to a kernel BPF program, so it saves the
  JS-callback dispatch and object-conversion cost for non-matching packets
  but does **not** avoid the kernel→userspace copy. `net X/Y` (CIDR) and
  `portrange` are not supported yet; a malformed expression makes
  `open` / `openFile` reject. The `capture-file.ts` example now also does a
  filtered read.

## [0.24.0] — 2026-05-31

### Added

- **`net.capture` — packet capture + pcap file I/O.** Powered by pure-Go
  gopacket (no `libpcap`, no cgo). Four bindings:
  `net.capture.interfaces()` returns the host's network interfaces
  (`{ name, addresses, up, loopback }`) synchronously — pure-Go, no
  privileges, all platforms. `net.capture.open({ iface, promisc?, snaplen?
  }, pkt => {…})` starts a **live** capture (resolving to `{ iface, link,
  close() }`) — **Linux + macOS only** (Linux `AF_PACKET`, macOS BPF;
  Windows rejects) and requires **root / `CAP_NET_RAW` (Linux)** or
  **`/dev/bpf` access (macOS)**. `net.capture.openFile(path, pkt => {…})`
  reads a `.pcap` / `.pcapng` file (auto-detected) and resolves at EOF.
  `net.capture.toFile(path, { linkType?, snaplen? })` returns `{
  write(bytes, { ts? }), close() }` to write raw frames. The handlers
  receive a decoded packet `{ ts, length, captureLength, link, eth?, ip?,
  tcp?, udp?, icmp?, payload?, bytes }` (layer keys present only when that
  layer decodes). No BPF-expression filters — filter in the callback;
  common-layer decode only. New offline example `capture-file.ts`
  (interfaces() + a `toFile`/`openFile` round-trip on a hand-built frame).

### Fixed

- **`LoopCallable.Call` no longer hangs (leaking a goroutine/fd) when the
  event loop is terminated mid-call** — affects `net.capture` and all
  socket/listener dispatchers.

## [0.23.0] — 2026-05-30

### Added

- **`server.tcp.listen`, `server.udp.listen` — raw inbound listeners.**
  The server-side counterparts to the v0.22.0 `net.tcp.connect` /
  `net.udp.open` clients. `server.tcp.listen({ port, host?, readBuffer? },
  conn => {…})` runs the connection handler once per accepted socket, where
  `conn` is the **same handle shape** as `net.tcp.connect` — `onData(cb)`
  (cb gets `{ bytes, text }`), `onClose(cb)`, `onError(cb)`, `write(data)`
  (string or `Uint8Array`), `close()`, and `remote` / `local`.
  `server.udp.listen({ port, host? }, (msg, reply) => {…})` runs the handler
  once per inbound datagram, where `msg` is `{ bytes, text, address, port }`
  (the sender) and `reply(data)` (string or `Uint8Array`) sends a datagram
  back to that sender, returning a Promise. Both bind synchronously (throw
  on bind error), accept `port: 0` for an OS-chosen ephemeral port, and
  return a handle `{ address: "tcp|udp/host:port", close() }`. Like the
  other `server.*` listeners they hold the event loop open while bound and,
  under `sercon serve`, emit a `READY listening on tcp|udp/…` line and
  participate in graceful shutdown. New example `server-tcp.ts` (offline
  TCP echo server + client round-trip).

## [0.22.0] — 2026-05-30

### Added

- **`net.tcp.connect`, `net.udp.open`, `net.icmp.open` — raw client
  sockets.** Long-lived, bidirectional client sockets with a *push /
  callback* read model (distinct from the one-shot `net.probe.*`
  helpers). Each constructor resolves a handle sharing `onData(cb)` (TCP)
  / `onMessage(cb)` (UDP, ICMP), `onClose(cb)`, `onError(cb)`, and
  `close()`; inbound events carry `bytes` (`Uint8Array`) + `text`.
  `net.tcp.connect(host, port, opts?)` adds `write(data)` and `remote` /
  `local`. `net.udp.open(opts)` has connected mode (`{ host, port }` →
  `send(data)`) and bound mode (`{ bind }` → `sendTo(data, host, port)`,
  events tagged with `address` / `port`), plus `local`.
  `net.icmp.open(opts?)` requires root / `CAP_NET_RAW` (open rejects
  otherwise); `send({ to, type?, code?, id?, seq?, payload? })` builds an
  Echo-shaped body — `type` / `code` are customizable but non-Echo bodies
  are not modelled; events carry `address` / `type` / `code`. Sockets hold
  the event loop open while connected. New example `net-sockets.ts`
  (offline UDP loopback self-test).

## [0.21.0] — 2026-05-30

### Added

- **`db.clickhouse`, `db.oracle` — two more SQL engines.** ClickHouse
  (pure-Go `clickhouse-go` v2, `?` placeholders, `opts.secure` for TLS)
  and Oracle (pure-Go `go-ora`, no cgo, `:1`/`:2` binds, `opts.database`
  is the service name). Both reuse the shared `database/sql` handle
  (`exec`/`query`/`queryValue`/`begin`/`prepare`/`close`), pinged on open,
  the same per-engine-namespace pattern as `db.postgres`/`mysql`/`mssql`.

## [0.20.0] — 2026-05-30

### Changed

- **Stable JSON key order across the remaining object-returning bindings.**
  Bindings that returned a `map[string]any` shuffled `JSON.stringify`
  output run-to-run (Go randomizes map iteration), breaking callers that
  hash a canonical serialization. The conditional / dynamic / decoded-JSON
  keyed results now build a `scriptengine.Ordered` (insertion-ordered,
  constructed off-loop, converted to an ordered object on the loop) instead:
  `net.probe.dns`/`whois`/`tls`/`ntp`, `net.netstatus`, `net.http.request`
  (headers), `db.ldap`, `db.sqlite`/`postgres`/`mysql`/`mssql` query rows,
  `crypto.jwt.view`/`validate`, `services.gh` pr-list/repo-view, and
  `server.smtp` envelope/message objects. Key *order* is now deterministic;
  conditional-presence (`"mx" in r`) and values are unchanged. `text.jq` is
  the lone exception — `gojq` discards key order internally.

## [0.19.0] — 2026-05-30

### Changed

- **`runtime.assert.equal` is now deep.** It compared by reference (goja
  StrictEquals) despite its docs promising "deep equality on objects",
  so two distinct objects/arrays with identical contents never matched.
  It now does recursive structural comparison (key order irrelevant);
  primitive comparison is unchanged.
- **`console.*` and `runtime.log` render objects as JSON.** Object/array
  arguments now print as JSON (`console.log({a:1})` → `{"a":1}`) instead
  of `[object Object]`; primitives still print raw, and a circular value
  falls back to `[object Object]` rather than throwing.

## [0.18.0] — 2026-05-30

### Added

- **`db.postgres`, `db.mysql`, `db.mssql` — server SQL engines.**
  PostgreSQL (pure-Go `jackc/pgx`), MySQL/MariaDB (`go-sql-driver/mysql`),
  and Microsoft SQL Server (`microsoft/go-mssqldb`) join `db.sqlite` on a
  shared `database/sql` handle: `open()` (a driver DSN string or a
  `{ host, port, user, password, database, sslmode? }` options object)
  resolves to `{ exec, query, queryValue, begin, prepare, close }`, with
  transactions and prepared statements. The connection is pinged on open;
  bind parameters are positional (write your engine's placeholder syntax —
  `?` / `$1` / `@p1`). All drivers are pure Go (no cgo). The sqlite handle
  was refactored into this shared layer (no behavior change).

## [0.17.0] — 2026-05-30

### Added

- **`sercon init [dir]` — one-command editor autocomplete.** Drops
  `sercon.d.ts` (the binding declarations) plus a `jsconfig.json` into a
  directory, so any TypeScript-language-server editor (VSCode, Zed,
  Neovim+coc, Sublime LSP, …) gives completion + hover docs for the
  reserved globals with no plugin or manual config. Existing files are
  left untouched unless `--force`. The bundled `examples/scripts/` now
  ships a `jsconfig.json` so the demos autocomplete out of the box.

## [0.16.0] — 2026-05-30

### Changed

- **Stable JSON key order for several binding results.** `net.probe.tcp`/
  `ping`/`smtp`/`wss`, `db.dict.define`/`match`, `services.gh.authStatus`,
  and `services.git.*` (branch/status/add/commit/log/diffStat/runText) now
  return json-tagged structs instead of `map[string]any`. goja takes a JS
  object's property order from Go map iteration, which Go randomizes per
  process — so `JSON.stringify` of these results previously shuffled keys
  run-to-run, breaking canonical-serialization hashing (payment signing,
  webhook signatures). Struct field order is deterministic, so the output
  is now byte-stable; the emitted `sercon.d.ts` types also sharpen from
  `Record<string, unknown>` to precise object shapes. Result keys, values,
  and order are otherwise unchanged. (Bindings with conditionally-present
  or dynamic keys are tracked separately — see OUT-OF-SCOPE.md; a struct
  can't represent them because goja exposes every field regardless of
  `omitempty`.)

## [0.15.0] — 2026-05-30

### Added

- **`--help` / `--examples` are paged.** When stdout is a terminal, the
  long help and feature-walkthrough screens now pipe through `$PAGER`
  (falling back to `less` with `LESS=FRX`, ANSI color preserved), like
  git. A pipe/redirect, the new `--no-pager` flag, or `PAGER=cat` renders
  directly as before.

## [0.14.0] — 2026-05-30

### Added

- **`console` global (browser/Node compatibility).** `console.log` /
  `console.info` / `console.debug` print a clean, space-joined line to
  stdout; `console.warn` / `console.error` go to stderr. This replaces
  the goja default console (which routed everything through Go's logger
  — timestamped, all on stderr) with a stream-correct, prefix-free shim
  so scripts pasted from a browser or Node run unchanged. `runtime.log`
  remains the native stdout logger. The CLI sets `Options.DisableConsole`
  so its `console` is authoritative; library embedders still get the
  goja_nodejs console by default.

## [0.13.2] — 2026-05-30

### Fixed

- **MANUAL cover date no longer goes stale.** The `<div class="date">`
  line on the cover had no release marker, so `make release-prep` bumped
  the version but left the date frozen at an old cut date. release-prep
  now stamps the date with the cut date too, and `make version-check`
  cross-checks the cover date against the CHANGELOG entry for the current
  version — a stale date now fails the pre-release check.

## [0.13.1] — 2026-05-30

### Changed

- **Dropped the legacy `api` naming.** The v0.9.0 rewrite replaced the
  single `api` global with ten top-level globals; this removes the
  leftover `api_` prefix from every `cmd/sercon/*.go` file and renames
  the bundled declaration file `examples/scripts/api.d.ts` →
  `sercon.d.ts`. Regenerate your own with `sercon -emit-dts sercon.d.ts`
  and update any `/// <reference path="./api.d.ts" />` accordingly. No
  script-facing behavior change.
- **CI:** bumped GitHub Actions off the deprecated Node.js 20 runtime
  (`actions/checkout@v5`, `actions/setup-go@v6`,
  `goreleaser/goreleaser-action@v7`).

## [0.13.0] — 2026-05-30

### Added

- **Executable scripts via shebang.** A script may begin with a `#!`
  shebang line; it is stripped before transpile (blanked in place so
  transpile/syntax-error line numbers still match the source), so a
  `.ts`/`.tsx`/`.js` file — entry or required module — can be `chmod
  +x`'d and run directly. Previously a shebang line caused a
  `SyntaxError`.
- **`sercon run <script> [args...]` subcommand.** Runs exactly one
  script and passes every token after the script path to it as
  `runtime.argv[2:]` — no standalone `--` separator needed. This makes
  fully argument-capable executable scripts practical via
  `#!/usr/bin/env -S sercon run`. The default multi-script mode (every
  positional is a separate script; args after `--`) is unchanged.

## [0.12.0] — 2026-05-30

### Added

- `codec.php.*` (serialize/unserialize, varExport/parseVarExport,
  varDump/parseVarDump) and `codec.perl.*` (dumper/parseDumper): read and
  write PHP and Perl data dumps. JSON-style array mapping, a symmetric
  `__class` sentinel for class-bearing values, shared-reference resolution
  with cycle detection, and the JSON::XS::Boolean convention for Perl
  booleans. Decoded objects keep stable key order (canonical-JSON safe).

## [0.11.2] — 2026-05-30

### Fixed

- **`text.preg` / `text.preg2` match results now have a stable JSON key
  order.** The `{ match, groups, index }` result was built as a Go map,
  and goja derives a JS object's property order from Go map iteration —
  which Go randomizes per process — so `JSON.stringify(result)` emitted
  the keys in a different order on nearly every run. That breaks any
  caller that hashes a canonical serialization (payment-style request
  signing, webhook signature verification), where key order is part of
  the signed bytes. The result is now built as an ordered object
  (`match`, then `groups`, then `index`) shared by both engines, so the
  serialization is byte-stable. Values and shape are unchanged. Guarded
  by tests asserting the exact serialized string. (Note: other bindings
  that return objects via Go maps share this characteristic; only the
  regex bindings are addressed here.)

## [0.11.1] — 2026-05-30

### Fixed

- **TUI: second run no longer hangs / corrupts the terminal.** The TTY
  path double-initialised the tcell screen — `api_tui.go` called
  `screen.Init()` and then `Controller.StartScreen` → tview `SetScreen`
  init'd it again. tcell's `tty.Start` runs `term.MakeRaw`, which returns
  the termios as it was *before* the call, so the second Init saved the
  already-raw state as the restore target. The single `Fini()` on teardown
  then restored the terminal to raw mode (no echo, Ctrl-C as a keystroke),
  so the next `tui` run hung and only killing the terminal recovered it.
  The screen is now initialised exactly once (by tview's `SetScreen`, per
  its documented contract). Guarded by a regression test.

### Changed

- `make demo` now runs with `-timeout 90s` (was the 10s default). The
  `ai.ts` demo shells out to an AI CLI (`claude -p`, …) which can exceed
  the 10s engine ceiling when another AI session is active, killing the
  run before the script's own 60s timeout + try/catch could degrade
  gracefully. The larger ceiling only matters for `ai.ts`; every other
  demo finishes in milliseconds.

## [0.11.0] — 2026-05-30

### Added

- New binding `server.smtp.listen({…})` — inbound SMTP listener with
  per-stage callbacks (`onMail`/`onRcpt`/`onData`, async-capable),
  optional SASL AUTH (PLAIN + LOGIN), STARTTLS, and configurable limits.
  Messages parsed via jhillyerd/enmime into `{from, to, subject, headers,
  body.text/html, attachments, raw}`. See MANUAL.md §6.7.
- New binding `net.email.send({…})` — outbound SMTP sender with in-tree
  MIME composition (text / multipart-alternative / multipart-mixed),
  per-recipient outcome capture, three TLS modes (`starttls`/`tls`/`none`),
  and PLAIN auth. Returns `{accepted, rejected}`. One TCP connection per
  call.
- `sercon serve` now emits a per-stage SMTP access log line to stderr
  (AUTH/MAIL/RCPT/DATA/QUIT). Vanilla `sercon` stays silent.
- New deps: `github.com/emersion/go-smtp`, `github.com/jhillyerd/enmime`,
  `github.com/emersion/go-sasl` (all pure-Go, MIT).

### Migration

No script-side migration required — additive only.

## [0.10.0] — 2026-05-29

### Added

- New top-level global `server` with HTTP/HTTPS listeners, stdlib
  `http.ServeMux` pattern routing, onion-style middleware, a
  static-file mount helper, and WebSocket upgrade via async iterator.
  Reserved globals grow from 9 to 10. See MANUAL.md §6.
- New CLI subcommand `sercon serve script.ts` adds production
  niceties for long-running scripts: structured access log to
  stderr (format `ts remote method path status dur_µs`),
  `--shutdown-timeout` (default `30s`), `--port-override`, and a
  `READY listening on tcp/…` line on stdout per listener. Clean
  SIGTERM exits `0`. Vanilla `sercon script.ts` is unchanged.
- New engine API: `scriptengine.NewLoopCallable(loop, fn)` returns a
  wrapper that lets a captured JS Callable be invoked from any
  goroutine via `.Call(buildArgs)`. The on-loop variant
  `.CallOnLoop(vm, args...)` invokes synchronously when the caller
  is already on the loop.
- New engine API: `Engine.HoldRun(reason string) (release func())`.
  Refcounted sentinel-timer that keeps `loop.Run` alive until
  `release()` fires; cleanup-drained on Run end as a safety net.
  Multiple concurrent holds compose; `release` is idempotent.
- esbuild now lowers `for await (...)` and async generators (via the
  `Supported` flag) so they work in scripts running on goja, which
  doesn't parse them natively.
- New polyfill: every Run installs `Symbol.asyncIterator =
  Symbol.for("@@asyncIterator")` so esbuild's `__forAwait` helper
  and user code that does `obj[Symbol.asyncIterator] = ...` agree
  on the same key.

### Migration

No script-side migration required — additive only. Existing scripts
keep working without changes.

## [0.9.0] — 2026-05-29

### Changed

- **BREAKING:** The `api.*` wrapper is removed. The nine v0.8.0 buckets
  are now top-level globals: `runtime`, `crypto`, `text`, `codec`,
  `fs`, `net`, `db`, `services`, `tui`. Scripts must drop the `api.`
  prefix; the bucket and member shape inside each global is unchanged
  except for the three renames below.
- **BREAKING:** `api.tools.*` → `services.*` (was a poor description of
  CLI/service wrappers; `services` reads more naturally).
- **BREAKING:** `api.format.*` → `codec.*` (the bucket holds binary-format
  codecs: compression, barcode, checkdigit).
- **BREAKING:** `api.ui.tui.*` → `tui.*` (the `ui` wrapper was empty
  scaffolding; `tui` is hoisted to a top-level global).
- **BREAKING:** The `Sercon` global is removed. `Sercon.argv` is now
  `runtime.argv`; the layout (`[programName, scriptPath, …userArgs]`)
  is unchanged.

### Migration

Drop the `api.` prefix from every script call:
- `api.crypto.hash.sha256(x)` → `crypto.hash.sha256(x)`
- `api.net.http.get(url)` → `net.http.get(url)`
- `api.runtime.log("hi")` → `runtime.log("hi")`

Then apply the three renames (`tools→services`, `format→codec`,
`ui.tui→tui`) and replace `Sercon.argv` with `runtime.argv`.

## [0.8.0] — 2026-05-28

### Changed

- **BREAKING — `api.*` surface re-nested into 9 category buckets.**
  Every existing path moves to `api.<category>.<group>.<fn>` (or
  `api.<category>.<fn>` for bare functions like `log`). The 9
  categories are `runtime`, `crypto`, `text`, `format`, `fs`, `net`,
  `db`, `tools`, `ui`. Two leaf renames within the new layout:
  `api.net.*` (TCP/DNS/TLS/NTP/WHOIS/ping/smtp/wss probes) becomes
  `api.net.probe.*`, and `api.text.*` (charset detection) becomes
  `api.text.charset.*`. `path` and `archive` move into a new
  `api.fs.*` bucket. No deprecation aliases — old paths are removed
  in this release. See MANUAL.md §5 "Migrating from 0.7" for the
  full substitution table.

  The mechanical migration for user scripts is a single search-replace
  pass per script — see the table in MANUAL.md §5 for the full mapping.

## [0.7.0] — 2026-05-28

### Added

- `api.tui.*` — multi-pane terminal UI with focus + scrollback. Scripts
  declare a recursive split-tree layout (`api.tui.layout`), obtain Pane
  handles (`api.tui.pane("name")`) with `write`/`writeln`/`clear`/`title`,
  and route subprocess output via `api.exec.shell(cmd, { pane })`. ANSI
  colors pass through. When stdout is not a TTY, the same calls fall
  back to prefixed plain-text lines so scripts run in CI / pipes
  unchanged. Incompatible with `--watch` in v1.
- `Engine.AddRunCleanup(fn)` + `Options.WatchMode` — small engine hooks
  enabling host bindings (e.g. the TUI) to register per-Run teardown and
  refuse to start under `--watch`.

### Fixed

- `PromisifyAsync` snapshots `FunctionCall.Arguments` so concurrent async
  bindings (Promise.all over multiple `api.exec.shell` calls, for
  example) no longer race on goja's reusable arguments slice. The
  failure mode in practice was an argv element being a goja Object
  instead of a string. Pre-existing latent bug surfaced by the api.tui
  demo's parallel subprocesses.

### Changed

- **Release flow:** dropped release-please in favour of goreleaser-only.
  Maintainers now cut releases manually via `make release-prep VERSION=x.y.z`
  + `git tag` + `git push`; the tag-triggered `release.yml` runs goreleaser
  unchanged. The previous release-please-driven flow desynced state after
  the manual `v0.6.0` cut (which we had to do while a separate Actions
  permission was being granted) and re-anchoring it proved more work than
  running the cut by hand. Removed: `.github/workflows/release-please.yml`,
  `.github/workflows/sync-manual-pdf.yml`, `release-please-config.json`,
  `.release-please-manifest.json`, `CHANGELOG-AUTO.md`. `make release-prep`'s
  next-step checklist is the new source of truth.

## [0.6.0] — 2026-05-28

### Added

- `Sercon` runtime global exposing `Sercon.argv` to every script
  (Node/Bun layout `[programName, scriptPath, ...userArgs]`). CLI args
  after a standalone `--` form the argument tail
  (`sercon run.ts -- --port 8080`); the library API adds the `WithArgs`
  RunOption and `Options.ProgramName`.

### Fixed

- `api.exec.shell` now kills the whole subprocess group on a timeout or
  cancellation. Previously, when the shell forked the command (rather
  than exec-ing into it), the surviving grandchild held the stdout/stderr
  pipe open and `Wait` blocked for the command's full duration — so a
  short `timeout` did not return promptly. A `WaitDelay` backstop bounds
  the wait if a process escapes the group.

## [0.5.30] — 2026-05-26

Adds **module-graph invalidation to `--watch`** — a file change now
re-runs only the entry scripts that import the changed file, not
every entry. Library-internal additions to support it; no new
dependency. **This empties the Moderate bucket** of
`OUT-OF-SCOPE.md` (only the library-less PDF417 decoder remains).

### Added

- `Engine.SetResolveHook(fn)` — fires with the absolute path of
  every module file the require loader resolves during a Run.
  `--watch` uses it to capture each entry's import graph.
- `Engine.ResetModuleCache()` — discards the cached module registry
  so the next Run re-reads + re-compiles imported modules from
  source. `--watch` calls it before each re-run so an edited
  import's new source actually takes effect (the registry
  otherwise caches compiled bytecode across runs — without this,
  watch re-runs showed stale module content).
- The watch loop builds a per-entry import graph (entry file + all
  resolved deps), accumulates changed paths across the debounce
  window, and re-runs only the entries whose graph intersects the
  changed set. stdin entries and not-yet-graphed entries re-run
  unconditionally (conservative). `affectedEntries` is unit-tested
  (5 cases: shared-dep-hits-only-importer, entry's-own-file,
  unrelated-change-no-rerun, stdin-always, ungraphed-always).
- `TestModuleLoader_*` plus the existing engine suite confirm the
  resolve hook doesn't disturb normal resolution.

### Changed

- MANUAL §4's `--watch` section gains a "Module-graph invalidation"
  paragraph. `OUT-OF-SCOPE.md`'s Moderate bucket collapsed to a
  one-line "everything shipped" note + the deferred PDF417 item.

## [0.5.29] — 2026-05-26

Hardens the entry-script ESM→CJS rewriter against **interleaved
comments and irregular whitespace** in multi-line imports. Library-
internal; no new dependency, no API change.

### Added

- `stripComments` removes `// line` and `/* block */` comments from
  an import statement before the regex match, and is string-literal
  aware (a `//` inside a quoted module path isn't mistaken for a
  comment). Applied in both `importStatementComplete` (so a
  commented import still terminates on its closing quote) and
  `convertImport`.
- `convertImport` now collapses whitespace runs (`strings.Fields`)
  so ragged indentation / alignment in a multi-line import doesn't
  defeat the token regexes.
- `TestRun_AwkwardImports`: a multi-line named import with a
  trailing line comment, an inner block comment, and uneven
  whitespace now rewrites and runs correctly.

### Changed

- `OUT-OF-SCOPE.md`'s "Robust import parsing" entry resolved.

## [0.5.28] — 2026-05-26

Adds a custom **`Options.ModuleLoader`** hook to `pkg/scriptengine`
— a library-side API (no script binding) that lets embedders serve
modules from somewhere other than disk: an in-memory FS, a network
source, an embedded bundle.

### Added

- `Options.ModuleLoader func(candidatePath string) (source string,
  found bool, err error)`. Consulted for every require/import
  candidate before the filesystem. The engine probes the bare path
  plus the usual extension fallbacks (`.ts` / `.tsx` / `.js` /
  `.mjs` / `.cjs` / `.json`) so a loader can match on a plain
  suffix; a `.ts` / `.tsx` source is transpiled like a disk read.
  `found: false` falls through to disk; a returned error aborts
  resolution.
- 2 tests: serve-from-memory (in-memory module map matched by
  suffix, imported and called) and error-aborts.
- MANUAL §3 (library API) documents the hook with an example. The
  `Verbose` Options field is now shown in the struct listing too
  (it existed but was undocumented in the §3 snippet).

### Changed

- `OUT-OF-SCOPE.md`'s "Custom `PathResolver`" entry resolved.

## [0.5.27] — 2026-05-26

Adds **`api.ai`** — run one-shot prompts through a coding-assistant
CLI (claude / codex / copilot / gemini). `os/exec` (stdlib), no new
dependency.

### Added

- `api.ai.providers()` lists detected CLIs on PATH (preference
  order); `api.ai.send(opts)` runs a prompt through one. `opts`:
  `{ prompt (required), provider?, system?, context?, timeout? }`.
  `system` / `context` are prepended (portable across CLIs with
  different flags). Returns `{ provider, output, exitCode }`;
  non-zero exit is data, missing provider + deadline throw.
- Chose the options-object shape over the rhai-style builder chain
  — the idiomatic JS equivalent, and it avoids threading a mutable
  builder handle through goja.
- `buildAIArgv` is a pure function (unit-testable without the CLIs).
  6 tests: argv builder per provider, prompt composition,
  send via a fake on-PATH provider script, no-provider throws,
  missing-prompt throws, provider detection.
- `examples/scripts/ai.ts` gracefully degrades without a provider
  (in the CI offline subset); MANUAL §5 ts block + prose.

### Changed

- `OUT-OF-SCOPE.md`'s `ai::request` entry resolved. **The Moderate
  bucket is now empty** — every binding-side and protocol item has
  shipped; only the library-internal items (PathResolver, robust
  import parsing, watch module-graph) remain.

## [0.5.26] — 2026-05-26

Adds **`api.dict`** — RFC 2229 DICT protocol client. No maintained
pure-Go DICT library exists, so the protocol is hand-rolled over
`net/textproto`. No new dependency.

### Added

- `api.dict.define(host, word, opts?)` → `{ word, found,
  definitions: [{ db, dbName, text }] }`; `found: false` on no
  match (data, not an error).
- `api.dict.match(host, word, opts?)` → `{ word, matches: [{ db,
  word }] }`. `opts.strategy` (default `prefix`), `opts.database`
  (default `*`), `opts.port` (default 2628).
- 5 tests against an in-process fake DICT server: define, define-
  not-found, match, validation, and a `dictFields` quote-aware
  tokeniser unit test. The textproto `StartResponse`/`EndResponse`
  pairing is required around the read sequence (without it the
  pipeline deadlocks — caught during development).
- `examples/scripts/dict.ts` gracefully degrades; hits dict.org
  (`make demo` only, not CI). MANUAL §5 ts block + prose.

### Changed

- `OUT-OF-SCOPE.md`'s `dict` entry resolved.

## [0.5.25] — 2026-05-26

Adds **`api.ldap`** — anonymous LDAP query over `go-ldap/v3`.
Stateful-handle binding (read / inspection surface, no modify).
New dependency: `github.com/go-ldap/ldap/v3`.

### Added

- `api.ldap.open(url, opts?)` dials `ldap://` / `ldaps://`, binds
  anonymously (or with `opts.bindDN` / `opts.password`), resolves
  to `{ rootDSE, search, close }`.
- `rootDSE()` reads the server metadata entry; `search(baseDN,
  filter, attrs?)` runs a subtree search returning `{ dn,
  <attr>: [values] }` per entry (attributes stay arrays).
- 3 tests (validation / error paths — LDAP round-trip needs a
  live server): empty-URL throws, bad-URL throws,
  connection-refused throws.
- `examples/scripts/ldap.ts` gracefully degrades; hits a public
  test LDAP (`make demo` only, not CI). MANUAL §5 ts block + prose.

### Changed

- `OUT-OF-SCOPE.md`'s `ldap` entry resolved.

## [0.5.24] — 2026-05-26

Adds **`api.memcached`** — text-protocol client over
`bradfitz/gomemcache`. Stateful-handle binding. New dependency:
`github.com/bradfitz/gomemcache`.

### Added

- `api.memcached.open(addr)` (host:port) → `{ get, set, delete }`.
  `get` returns the string or `null` on a cache miss; `set(key,
  value, expirySeconds?)` stores bytes (0 = never expire);
  `delete` returns `true` if the key existed.
- 3 tests against an in-process fake memcached (minimal text-protocol
  stand-in — no maintained pure-Go in-process server exists):
  set/get/delete round-trip, delete-miss-returns-false,
  empty-addr-throws.
- `examples/scripts/memcached.ts` gracefully degrades without a
  server (in the CI offline subset); MANUAL §5 ts block + prose.

### Changed

- `OUT-OF-SCOPE.md`'s `memcached` entry resolved.

## [0.5.23] — 2026-05-26

Adds **`api.redis`** — RESP client over `redis/go-redis/v9`. A
stateful-handle binding. New dependency: `github.com/redis/go-redis/v9`
(official client; `alicebob/miniredis` as a test-only dep).

### Added

- `api.redis.open(url)` parses a `redis://[:password@]host:port/db`
  URL, PINGs to surface a bad address, and resolves to
  `{ do, ping, close }`.
- `do(command, ...args)` runs an arbitrary RESP command — the
  binding stays small by not mirroring hundreds of methods.
  Replies map the obvious way; a missing key (nil reply) becomes
  JS `null` rather than throwing. Redis-level errors throw.
- 4 tests via in-process miniredis: SET/GET/DEL/PING, list + hash
  commands, bad-URL throws, ping-fails-on-dead-server.
- `examples/scripts/redis.ts` gracefully degrades without a server
  (in the CI offline subset for that reason); MANUAL §5 ts block +
  prose.

### Changed

- `OUT-OF-SCOPE.md`'s `redis` entry resolved.

## [0.5.22] — 2026-05-26

Adds **`api.browser`** — a stateful HTTP session with an automatic
cookie jar and replayed default headers. Second stateful-handle
binding (same shape as `api.sqlite`). Pure stdlib: `net/http` +
`net/http/cookiejar` + `golang.org/x/net/publicsuffix`.

### Added

- `api.browser.open()` → handle `{ setUserAgent, setHeader, get,
  post, cookies }`. The cookie jar persists across requests (a
  login POST followed by a GET replays the session cookie
  automatically); default headers set via `setUserAgent` /
  `setHeader` are replayed on every request. `cookies(url)`
  inspects the jar.
- `get` / `post` return the `{ status, ok, headers, body, url }`
  shape from `api.http.request`. No explicit close — the session
  is GC'd with the handle.
- 4 tests against an httptest cookie server: cookie-jar persists,
  headers replayed, cookies-inspection, empty-URL throws.
- `examples/scripts/browser.ts` (hits httpbin.org, network demo
  only); MANUAL §5 ts block + prose.

### Changed

- `OUT-OF-SCOPE.md`'s `browser()` entry resolved.

## [0.5.21] — 2026-05-26

Adds **`api.netstatus.check(host, opts?)`** — an aggregate
connectivity probe that runs DNS / TCP / TLS / HTTP against one
host concurrently and folds them into a single status object. No
new dependency (composes `net` / `crypto/tls` / `net/http`).

### Added

- Fan-out via a WaitGroup; each sub-probe carries `ok` + an
  `error` string on failure. `reachable` is `dns.ok && tcp.ok`;
  TLS / HTTP are reported but don't gate it. Individual failures
  are data, not throws — the result is always a complete snapshot.
  Only a missing host argument throws.
- Result `{ host, port, elapsedMs, reachable, dns, tcp, tls,
  http }`.
- 3 tests: unreachable-is-data (closed port → reachable false, no
  throw), shape-complete (all four sub-probes always present),
  missing-host throws.
- `examples/scripts/net-probe.ts` grows a netstatus section;
  MANUAL §5 ts block + prose.

### Changed

- `OUT-OF-SCOPE.md`'s `netstatus::check` entry resolved.

## [0.5.20] — 2026-05-26

Adds **`api.net.wss(url, opts?)`** — a WebSocket handshake probe.
New dependency: `github.com/coder/websocket` (the maintained
successor to nhooyr.io/websocket; pure Go).

### Added

- Opens a `ws://` / `wss://` connection, optionally measures a
  ping/pong RTT (`opts.ping`, default true), closes. Returns
  `{ url, connected, subprotocol, status, handshakeMs, pingMs }`.
  `pingMs` is -1 when the ping is skipped or the server doesn't
  pong. Failed handshake (non-101, refused, bad URL) throws.
- The ping path uses `CloseRead` to pump control frames in the
  background so the pong is processed (coder/websocket's `Ping`
  requires the read loop to be running).
- 4 tests against an in-process httptest echo server: handshake +
  ping, no-ping (pingMs -1), bad-URL throws, empty-URL throws.
- MANUAL §5 `api.net` ts + prose bullet. (No live demo — public WS
  echo servers are too flaky for `make demo`; the local-server
  tests cover it.)

### Changed

- `OUT-OF-SCOPE.md`'s `wss` entry resolved.

## [0.5.19] — 2026-05-26

Adds **`api.net.smtp(host, opts?)`** — an SMTP capability probe
(EHLO + parse advertised extensions). No mail is sent. Hand-rolled
over `net/textproto`; no new dependency.

### Added

- Opens the connection, reads the banner, sends EHLO, parses the
  250 response into `{ host, port, banner, ehloDomain, extensions,
  starttls, authMechanisms, sizeLimit }`, then QUITs.
- `starttls` is a boolean; `authMechanisms` is the parsed AUTH
  mechanism list (e.g. `["PLAIN", "LOGIN"]`); `sizeLimit` is the
  SIZE extension's value. A server advertising none reports
  `false` / `[]` / `0` — a finding, not an error. Connection /
  protocol failures throw.
- 3 tests against an in-process fake SMTP listener: full
  capability parse, no-STARTTLS server, connection-refused throws.
- `examples/scripts/net-probe.ts` grows a guarded smtp section
  (smtp.gmail.com); MANUAL §5 `api.net` ts + prose bullet.

### Changed

- `OUT-OF-SCOPE.md`'s `smtp` entry resolved.

## [0.5.18] — 2026-05-26

Adds **`api.net.ping(host, opts?)`** — reachability probe in TCP
(default, no privileges) or ICMP modes. New dependency:
`github.com/prometheus-community/pro-bing` (for the ICMP path).

### Added

- TCP mode dials `host:port` `count` times and measures connect
  RTT — works in containers / CI, no raw-socket privileges. ICMP
  mode does real echo via pro-bing (needs root / CAP_NET_RAW;
  opt-in).
- Returns `{ host, ip, mode, sent, received, lossPercent, minMs,
  avgMs, maxMs }`. An unreachable host resolves with `received: 0`
  / `lossPercent: 100` — "down" is a normal outcome, not a throw.
  DNS-resolution failure and bad arguments throw.
- 4 tests: TCP reachable (localhost listener, 0% loss), TCP
  unreachable (closed port, 100% loss, no throw), bad-host throws,
  validation (unknown mode / empty host).
- `examples/scripts/net-probe.ts` grows a ping section; MANUAL §5
  `api.net` ts block + prose bullet.

### Changed

- `OUT-OF-SCOPE.md`'s `ping` entry resolved.

## [0.5.17] — 2026-05-26

Adds **`api.http.request(method, url, opts?)`** — the full HTTP
client beyond the two-line `get` / `post`. Pure `net/http`, no new
dependency.

### Added

- Per-call `headers`, `body`, `timeout` (default 30s), `retry`
  (re-attempts on transport errors + 5xx, never 4xx; linear
  backoff capped at 1s), `follow` (redirect control; default true),
  and basic-auth `username` / `password`.
- Result `{ status, ok, headers, body, url }` — `ok` is status in
  [200,400), `headers` lower-cased name→value, `url` the final URL
  after redirects. 4xx/5xx surface via `status`/`ok` (no throw);
  transport errors and context deadline throw.
- 7 tests via httptest: get status/headers/body, post + headers +
  basic auth, 4xx-doesn't-throw, retry-on-5xx (counts attempts),
  no-follow sees the 3xx, transport-error throws, input validation.
- `examples/scripts/http-request.ts` (hits httpbin.org, network
  demo only); `--examples` step 35; MANUAL §5 ts block + prose.

### Changed

- `OUT-OF-SCOPE.md`'s `http(url, opts?)` entry resolved.

## [0.5.16] — 2026-05-26

Adds the **PGP backend** to `api.encrypt`, completing the
two-backend design `detectBackend` (v0.5.8) anticipated. `encrypt`
/ `decrypt` now auto-dispatch between age and PGP based on the
key/ciphertext format — one API, two backends. New dependency:
`github.com/ProtonMail/go-crypto/openpgp` (the maintained pure-Go
PGP fork; x/crypto/openpgp is frozen).

### Added

- `api.encrypt.keygenPgp(opts?)` → armored `{ publicKey, privateKey }`
  PGP key blocks (RSA 2048). `opts.name` / `opts.email` populate the
  primary user ID.
- `api.encrypt.encrypt` routes to PGP when the first recipient is a
  PGP public-key block; output is always ASCII-armored
  (`-----BEGIN PGP MESSAGE-----`). Multi-recipient works the same as
  age (any recipient's private key decrypts).
- `api.encrypt.decrypt` routes to PGP when an identity is a PGP
  private-key block or the ciphertext is an armored PGP message.
  Accepts armored or binary PGP ciphertext.
- age and PGP recipient sets can't be mixed in one call (the
  formats are incompatible) — the backend is picked from the first
  recipient / the identity / the ciphertext.
- A deliberately small PGP subset: keygen + encrypt + decrypt. No
  signing, subkey management, or web-of-trust.
- `TestEncrypt_PGPRoundTrip`, `TestEncrypt_PGPDetectBackend`,
  `TestEncrypt_PGPWrongKeyThrows`, `TestEncrypt_AgeAndPGPDontCross`
  (regression guard that the two backends stay independent).
- `examples/scripts/encrypt.ts` grows a PGP section; MANUAL §5
  rewritten to describe the two-backend dispatch + `keygenPgp`.

### Changed

- `OUT-OF-SCOPE.md`'s PGP-backend entry resolved.

## [0.5.15] — 2026-05-26

Adds **JWK (JSON Web Key) support** to `api.jwt`. The `secret`
parameter now accepts a third form alongside raw bytes and PEM: a
JWK JSON object. Useful when keys arrive from a JWKS endpoint or a
config file in JWK form. New dependency:
`github.com/lestrrat-go/jwx/v2/jwk` (pure Go).

### Added

- `jwt.sign` / `jwt.validate` detect a JWK by the leading `{` plus
  a `"kty"` member, parse it via `jwk.ParseKey`, and extract the
  underlying crypto key (`*rsa.PrivateKey` / `*ecdsa.PrivateKey` /
  ed25519 key / `[]byte` for `oct`). The `kty` picks the key type,
  so a JWK works with whatever matching `opts.algorithm` is passed.
- JWK input bypasses the PEM/bytes cross-checks — the `kty` is
  authoritative. A malformed JWK throws with the parse error.
- `TestJwt_JWKRoundTrip` (EdDSA / ES256 / RS256), `TestJwt_JWKOctHMAC`
  (oct → HMAC), `TestJwt_JWKMalformedThrows`. `jwkPair` test helper
  generates in-memory JWK pairs.
- `examples/scripts/jwt.ts` grows a JWK section; MANUAL §5 key-shape
  prose updated to describe all three input forms.

### Changed

- `OUT-OF-SCOPE.md`'s JWK entry resolved.

## [0.5.14] — 2026-05-26

Adds **`opts.quietZone`** to `api.barcode.encode`. Pads the
rendered bars with a white margin — the spec-required clear zone
that EAN / UPC (and real-world scanners, gozxing included) need.
With it, an `encode("ean13", …) → decode(…)` round-trip works
without caller-side padding. No new dependencies — pure
`image/draw` over the existing boombuler output.

### Added

- `opts.quietZone` on `api.barcode.encode`: `true` adds a default
  margin (10% of the width, floored at 10px); a number sets an
  explicit pixel margin on each side; absent / `false` / `0` /
  negative → no padding (unchanged from before).
- `quietZonePixels` + `withQuietZone` helpers (the latter is a
  `draw.Draw` paste onto a white canvas).
- `TestBarcodeEncode_QuietZoneEAN13RoundTrip` (the headline:
  EAN-13 round-trips through encode→decode) and
  `TestQuietZonePixels` (9 cases pinning the bool / number /
  absent / negative / wrong-type branches).
- `examples/scripts/barcode.ts` grows a quiet-zone section showing
  the raw-EAN-fails / padded-EAN-decodes contrast; MANUAL §5 and
  `--examples` step 31 updated.

### Changed

- `OUT-OF-SCOPE.md`'s quiet-zone entry resolved.

## [0.5.13] — 2026-05-26

Adds **`api.preg2`** — the PCRE-flavoured regex sibling of
`api.preg`. Runs on `github.com/dlclark/regexp2` (a .NET regex
port), which supports lookahead, lookbehind, backreferences, and
possessive quantifiers — the features RE2 (and therefore
`api.preg`) can't do. New dependency:
`github.com/dlclark/regexp2 v1.12.0` (pure Go).

### Added

- `api.preg2.match` / `matchAll` / `replace` — identical API to
  `api.preg` (same `/pattern/flags` syntax, same
  `{ match, groups, index }` shape), so switching engines is a
  one-word change.
- Flag set is `i` / `m` / `s` / `x` — the `x`
  (ignore-pattern-whitespace) flag is the one RE2 couldn't honour.
  `u` / `U` still error (regexp2 is Unicode-aware by default; no
  global-ungreedy switch).
- `replace` uses .NET / regexp2 `$1` / `${1}` substitution syntax.
- 14 sub-tests: lookahead, lookbehind, backreference, matchAll,
  replace, all four flags, null-on-no-match, error cases,
  optional-group-empty (same stable-shape policy as preg).
- `examples/scripts/preg2.ts` demonstrates the RE2-can't features;
  `--examples` step 34; MANUAL §5 ts block + a prose paragraph
  spelling out the backtracking trade-off.

### Changed

- `OUT-OF-SCOPE.md`'s "PCRE-compatible regex engine" entry
  resolved.

## [0.5.12] — 2026-05-26

Completes the `api.sqlite` namespace with **prepared statements**.
`handle.prepare(sql)` compiles a statement once and resolves to a
handle whose `exec` / `query` / `queryValue` run it repeatedly with
fresh bind params (no SQL string on those calls — just the `?`
params). Cuts the per-call parse + plan cost for batch loops. No
new dependencies.

### Added

- `handle.prepare(sql)` → `Promise<stmt>` where `stmt` is
  `{ exec, query, queryValue, close }`. The exec / query /
  queryValue methods take only bind params; the SQL was fixed at
  prepare() time.
- Invalid SQL throws at `prepare()`, not the first `exec()`.
- Statements must be `close()`d; a leaked statement pins driver
  resources. Bound to the database handle, not a transaction
  (in-transaction prepared statements are out of scope).
- `sqliteArgsFrom(call, start)` generalises the old
  `sqlitePositionalArgs` so the handle / transaction methods read
  params from index 1 (after the SQL string) while prepared-
  statement methods read from index 0.
- 4 new tests: prepared exec loop, prepared query/queryValue,
  use-after-close throws, invalid-SQL-at-prepare throws. The
  sqlite suite is now 20.
- `examples/scripts/sqlite.ts` grows a prepared-statements section;
  `--examples` step 33 and MANUAL §5 updated.

### Changed

- `OUT-OF-SCOPE.md`'s SQLite entry removed — the namespace is now
  feature-complete (open / exec / query / queryValue / begin /
  prepare / close).

## [0.5.11] — 2026-05-26

Twelfth Moderate cut. Adds **transactions** to `api.sqlite` —
`handle.begin()` resolves to a nested transaction handle. No new
dependencies; extends v0.5.10's binding. Exercises the
nested-handle pattern: a handle method that itself returns a
handle.

### Added

- `handle.begin()` → `Promise<tx>` where `tx` is
  `{ exec, query, queryValue, commit, rollback }`. The exec /
  query / queryValue methods have the same shape and semantics as
  the top-level handle's, scoped to the transaction.
- `tx.commit()` / `tx.rollback()` finalize the transaction.
  Once finalized it's spent — further exec/query/commit/rollback
  calls throw `sql: transaction has already been committed or
  rolled back` via the `sqlite.tx.*:` prefix.
- A constraint violation inside a transaction throws (prefixed
  `sqlite.tx.exec:`) but doesn't auto-roll-back — the script
  decides whether to retry or roll back. Rolling back after a
  caught error preserves the pre-transaction state.
- 6 new tests: commit-visible, rollback-discards, tx query,
  constraint-then-rollback (table untouched), use-after-commit
  throws, double-commit throws. The sqlite suite is now 16 tests.
- `examples/scripts/sqlite.ts` grows a transactions section
  (commit, rollback, caught-constraint-then-rollback).
- `--examples` step 33 extended with a begin/commit snippet;
  MANUAL §5 gains the `begin()` ts block + a prose bullet on the
  transaction lifetime contract.

### Changed

- **Internal refactor**: `sqliteExec` / `sqliteQuery` /
  `sqliteQueryValue` now take an `sqlExecutor` interface
  (`ExecContext` + `QueryContext`, satisfied by both `*sql.DB`
  and `*sql.Tx`) plus an error-prefix label, instead of a
  concrete `*sql.DB`. The transaction handle reuses the exact
  same code paths as the top-level handle — only the executor
  and the label differ. Row scanning is extracted into a shared
  `scanRows` helper. No behaviour change for the existing
  top-level methods.
- `OUT-OF-SCOPE.md`'s SQLite entry narrowed from "transactions +
  prepared statements" to prepared statements only (transactions
  shipped here).

## [0.5.10] — 2026-05-26

Eleventh Moderate cut. Adds **`api.sqlite`** — sercon's first
stateful-handle binding. `open()` returns a handle object whose
methods (`exec` / `query` / `queryValue` / `close`) are bound to
the underlying connection. New dependency:
`modernc.org/sqlite v1.50.1` — the pure-Go SQLite translation (no
cgo, so the project's cgo-free rule holds; `mattn/go-sqlite3` is
ruled out for needing cgo).

### Added

- `api.sqlite.open(path)` → `Promise<handle>`. `":memory:"` for an
  in-RAM database, any other string for a file path (created if
  absent). The connection is `Ping`-ed before `open` resolves so a
  bad path surfaces at open() rather than the first query.
- `handle.exec(sql, ...params)` → `{ rowsAffected, lastInsertId }`.
  For DDL / INSERT / UPDATE / DELETE. Both counters are
  driver-reported; `CREATE TABLE` reports zero for both.
- `handle.query(sql, ...params)` → array of row objects keyed by
  column name. Type mapping: INTEGER / REAL → number, TEXT →
  string, NULL → null, BLOB → Uint8Array (with a UTF-8 promotion
  to string, since SQLite stores TEXT and BLOB identically and the
  heuristic recovers the common TEXT case while keeping genuine
  binary BLOBs as bytes).
- `handle.queryValue(sql, ...params)` → first column of the first
  row, or null when no rows match. For `SELECT count(*)`, PRAGMAs,
  single-column lookups.
- `handle.close()` → release the connection. No finalizer — the
  documented pattern is open / use / close.
- Parameters bind positionally as `?` placeholders. goja's native
  type exports (int64 / float64 / string / nil / []byte) pass
  straight to the driver, so no script-side coercion is needed.
- All handle methods plus `open` are async (Promise-returning).
- **Implementation note**: the handle methods are
  `PromisifyAsync(...).Func` — the raw `func(goja.FunctionCall)
  goja.Value` — not the `AsyncBinding` carrier. The engine only
  unwraps `AsyncBinding` to a goja-callable at *registration*
  time; the handle map is built at script-run time (inside the
  `open()` resolution), past that unwrap point, so taking `.Func`
  directly is required. Without it the methods export as plain
  objects and `db.exec(...)` throws "Not a function". This is the
  reusable wrinkle for every future stateful-handle binding.
- `TestSQLite_*` (10 tests): in-memory round-trip, exec counters
  for DDL/UPDATE/DELETE, ordered query, queryValue + null-on-no-row,
  every parameter type, BLOB round-trip staying binary, file-backed
  persistence across handles, invalid-SQL throw with prefix,
  empty-path throw with :memory: hint, use-after-close error.
- `examples/scripts/sqlite.ts` walks the full lifecycle; pure-Go so
  it's in the CI offline subset.
- `--examples` step 33 covers the binding; MANUAL §5 gains the
  `api.sqlite` ts block plus a paragraph per method and a note on
  the stateful-handle pattern.
- `cmd/sercon/api_docs.go` gains a `sqlite.open` entry (the handle
  methods aren't registered bindings so they don't appear in the
  d.ts namespace tree).

### Changed

- `OUT-OF-SCOPE.md`'s SQLite entry resolved; replaced with a
  forward-looking "transactions + prepared statements" entry
  (`handle.begin()` / `handle.prepare()` for batch workloads).

## [0.5.9] — 2026-05-26

Tenth Moderate cut. Adds **`sercon --watch`** — re-run on file
change for iterative development. Single-process loop (no
subprocess respawn) that reuses the existing `Engine` across runs,
since each `Run` already gets a fresh `*goja.Runtime`. New
dependency: `github.com/fsnotify/fsnotify v1.10.1` (pure Go, de
facto standard).

### Added

- `--watch` CLI flag. After the initial run, blocks and re-runs
  the entry scripts each time a watched file changes under the
  script root. SIGINT / SIGTERM exit cleanly. Per-script failures
  inside the loop log as `FAIL` but don't terminate the session —
  a syntax error you're trying to fix shouldn't kick you out.
- Watcher walks the script root recursively at startup; new
  directories appearing during the session are picked up
  automatically. Symlinks aren't followed (avoids classic
  watch-loop pitfalls).
- File filter: `.ts` / `.tsx` / `.js` / `.jsx` / `.json` trigger
  re-runs; `.d.ts` is matched via suffix so regenerated
  declaration files also trigger. Editor swap files (`.swp`,
  `~`-suffixed backups) are ignored.
- Directory filter: anything starting with `.` (`.git`,
  `.vscode`, …) plus `node_modules` are excluded from both the
  initial recursive walk and dynamic add. Both directory groups
  generate floods of irrelevant events on save.
- Debounce: events are collected for **150 ms** after the first
  one before re-running. Each new event during the window resets
  the timer, so a rapid save burst (editors typically fire write
  → rename → chmod) becomes one re-run after it settles.
- Run separator: each run is delimited by
  `--- sercon initial run @ HH:MM:SS ---` or
  `--- sercon re-run @ HH:MM:SS ---` so the output is visually
  distinct from the previous run.
- 28 unit tests pin the filter logic without spawning a real
  watcher: `TestIsWatchableFile` (15 file-name cases — every
  extension we support plus the rejection-list cases),
  `TestShouldWatchDir` (8 directory cases),
  `TestAddRecursive_FiltersHiddenAndNodeModules` (real
  filesystem walk verifying the dir count matches expectations
  with `.git` / `node_modules` correctly skipped),
  `TestAddRecursive_MissingRootErrors`.
- MANUAL §4 gains a `--watch` subsection covering what's watched,
  what's filtered, the debounce window, the run separator
  format, and the engine-reuse design note.
- `--help` lists the flag with its purpose.

### Changed

- `OUT-OF-SCOPE.md`'s "Watch mode" entry resolved; replaced with
  a forward-looking "module-graph invalidation" entry — v0.5.9
  re-runs every entry script on every change, a smarter loop
  would re-run only the entries whose import graph includes the
  touched file.

## [0.5.8] — 2026-05-26

Ninth Moderate cut. Adds **`api.encrypt.detectBackend`** — a pure
classifier that routes recipient / identity strings to the right
encryption backend by prefix matching. No new dependencies; no
parsing or I/O. The actual PGP encrypt/decrypt backend (which
`detectBackend` enables scripts to dispatch on) is left as a
forward-looking entry — the classifier is useful standalone for
hybrid age/PGP workflows where one branch uses sercon and the
other shells out to `gpg`.

### Added

- `api.encrypt.detectBackend(input)` → `{ backend: "age" | "pgp" |
  "unknown", kind?: "public" | "private" }`. Recognises three age
  forms (`age1...` X25519 recipients, `AGE-SECRET-KEY-1...`
  identities, and SSH public keys — `ssh-rsa` / `ssh-ed25519` —
  which age accepts natively) plus PGP armored block markers
  (`-----BEGIN PGP PUBLIC KEY BLOCK-----` /
  `-----BEGIN PGP PRIVATE KEY BLOCK-----`). The `kind` field is
  only present on identified backends; unknown returns
  `{ backend: "unknown" }` with no kind.
- Whitespace tolerance: leading and trailing whitespace are
  stripped before matching, so input read from a config file with
  surrounding newlines still classifies correctly.
- `TestEncryptDetectBackend` runs 16 input cases through the
  classifier (every age + PGP branch, three whitespace cases, six
  unknown cases including PEM keys that share the `-----BEGIN`
  prefix to confirm no false positives).
  `TestEncryptDetectBackend_AgreesWithRoundTrip` proves that an
  "age public" classification means encrypt() accepts that input
  and an "age private" classification means decrypt() accepts it
  — the classifier and the encrypt/decrypt paths agree.
- `toJSStringLit` test helper for safely embedding arbitrary
  strings (including newlines and quote chars) into JS source as
  string literals.
- `examples/scripts/encrypt.ts` grows a `detectBackend` section
  iterating over six sample inputs and printing the classification.
- `--examples` step 32 extended to mention the classifier with
  the dispatch idiom; MANUAL §5 prose gains a paragraph describing
  what the classifier covers and the false-negative-on-unfamiliar
  policy.
- `cmd/sercon/api_docs.go` entry added.

### Changed

- `OUT-OF-SCOPE.md`'s `detectBackend` entry rewritten as a
  forward-looking "PGP backend for `api.encrypt.*`" — the
  classifier shipped, but actually using PGP recipients in
  `encrypt` / `decrypt` is the substantial next slice.

## [0.5.7] — 2026-05-26

Eighth Moderate cut. Adds **`api.encrypt.rekey`** — re-encrypt
a ciphertext for a fresh recipient set without ever exposing the
plaintext to the caller. The internal decrypt+encrypt loop keeps
the plaintext on the Go stack between the two stages; nothing
reaches JS-land. No new dependencies — the existing
`filippo.io/age` covers it.

### Added

- `api.encrypt.rekey(ciphertext, oldIdentities, newRecipients, opts?)`
  → `Uint8Array`. Decrypts with `oldIdentities` and re-encrypts
  for `newRecipients`. Identity / recipient inputs use the same
  string-or-string[] shape as `encrypt` / `decrypt`; same
  cross-checks (private-as-recipient, public-as-identity) apply.
- **Format preservation by default.** When `opts.armored` is unset,
  the output matches the input — armored in / armored out, binary
  in / binary out. That's what you want when key-rotating a
  payload that lives in a fixed location (file, vault row, JSON
  field). Explicit `opts.armored: true | false` overrides.
- Input auto-detect on the read side reuses the existing
  `looksArmored` sniffer — same logic as `decrypt`.
- 10 new sub-tests in `api_encrypt_test.go` cover: binary
  round-trip, old-identity-locked-out (proves the recipient set
  actually changed), armored format preservation, binary format
  preservation, both opts.armored overrides, wrong-old-identity
  error path with `encrypt.rekey:` prefix, multi-new-recipient,
  public-as-old-identity cross-check, and three input-validation
  cases (empty ciphertext / empty oldIdentities / empty
  newRecipients). The encrypt suite is now 35 sub-tests.
- `examples/scripts/encrypt.ts` grows a rekey section
  demonstrating the rotation plus the "alice is locked out
  afterwards" proof.
- `--examples` step 32 extended to include the rekey form;
  MANUAL §5 prose gains a paragraph describing the
  internal-only-plaintext guarantee and the format-preservation
  default.

### Changed

- `OUT-OF-SCOPE.md`'s `rekey` entry resolved. The
  `detectBackend` + PGP entry remains as the final piece of the
  original five-function `encrypt::*` design.

## [0.5.6] — 2026-05-26

Seventh Moderate cut. Extends `api.encrypt.*` with **ASCII-armoured
output**. No new top-level binding: `opts.armored: true` on the
existing `encrypt` is the smaller surface (vs a separate
`encryptArmored` function) and `decrypt` auto-detects the format
from the leading bytes. One new import — `filippo.io/age/armor` —
no new go.mod entry; same `filippo.io/age` module.

### Added

- `api.encrypt.encrypt(data, recipients, opts?)` grows an
  `opts.armored: boolean` field. When true, the output is age's
  ASCII armor: the `-----BEGIN AGE ENCRYPTED FILE-----` banner
  followed by a base64-encoded body and a matching END line.
  Default stays `false` (binary) so existing v0.5.5 callers are
  unaffected.
- `api.encrypt.decrypt` auto-detects armored vs binary input.
  Leading whitespace and BOM bytes (common artefacts of pasting
  from JSON / YAML / email containers) are tolerated before the
  banner check, so a stray `\n` ahead of the payload doesn't
  defeat detection. The matched banner is the literal
  `armor.Header` constant exported by `filippo.io/age/armor`,
  not just the `-----BEGIN` prefix that PEM keys share — so
  there's no false-positive risk.
- Armored ciphertext as a JS string (the natural shape after
  pasting from JSON) also round-trips, since `jsArgToBytes`
  produces UTF-8 bytes that the armor reader can parse.
- `TestEncrypt_ArmoredOutputAndRoundTrip`,
  `TestEncrypt_ArmoredStringRoundTrip`,
  `TestEncrypt_DefaultStaysBinary`,
  `TestEncrypt_ArmoredWrongIdentityErrors`, and
  `TestLooksArmored` (7 sub-tests: exact banner, leading
  whitespace, BOM prefix, PEM private key, binary age header,
  empty, plain text). 12 new sub-tests total; the encrypt suite
  is now 25.
- `examples/scripts/encrypt.ts` grows a section demonstrating
  armor output, the banner peek, and string-form round-trip.
- `--examples` step 32 extended with the `opts.armored` case;
  MANUAL §5 prose updated with the armor wrapping rules and the
  auto-detect description on decrypt.

### Changed

- `OUT-OF-SCOPE.md`'s `encryptArmored` entry resolved (shipped via
  the `opts.armored` option per the design note in v0.5.5's
  changelog). `rekey` and `detectBackend` (+ PGP) remain.

## [0.5.5] — 2026-05-26

Sixth Moderate cut. **`api.encrypt.*`** — age X25519 encryption.
First slice of the five-function OUT-OF-SCOPE entry: the core
round-trip (keygen / encrypt / decrypt). Armoured ASCII output,
rekeying, and the recipient-format dispatcher are left as
follow-up entries. One new dep: `filippo.io/age v1.3.1` (pure-Go
reference implementation).

### Added

- `api.encrypt.keygen()` — Generate a fresh X25519 identity.
  Returns `{ publicKey, privateKey }` as the bech32 strings age
  writes to disk: `publicKey` is the `age1...` recipient,
  `privateKey` is the `AGE-SECRET-KEY-1...` identity.
- `api.encrypt.encrypt(data, recipients)` — Seal data to one or
  more recipients. Input accepts string / Uint8Array / ArrayBuffer.
  `recipients` accepts a single string or an array. Output is the
  binary age format as Uint8Array. Multi-recipient encryption uses
  age's native header — any one of the listed identities can
  decrypt without re-encryption per reader.
- `api.encrypt.decrypt(ciphertext, identities)` — Open an age
  payload with one of the supplied identities. Returns Uint8Array
  plaintext; scripts decode via `api.text.decode(bytes, "utf-8")`
  for text payloads (goja doesn't ship TextDecoder).
- All three members are synchronous. Encryption is pure CPU work
  with a small API surface; matching the call shape of the other
  crypto bindings (`api.jwt`, `api.hash`) keeps things uniform.
- **Cross-checks at the binding boundary** with named-key hints:
  - Private key passed as a recipient throws "looks like a
    private key (AGE-SECRET-KEY-...); recipients are public keys
    (age1...)". Catches the most common JS-side mix-up before
    age's bech32 parser produces a cryptic error.
  - Public key passed as an identity throws the inverse hint.
- `TestEncrypt*` (13 sub-tests covering: keygen shape, two-calls-
  differ entropy sanity, single-recipient round-trip,
  multi-recipient round-trip, wrong-identity error, both
  cross-check paths, four input-validation cases, non-age
  ciphertext path).
- `examples/scripts/encrypt.ts` walks through keygen, single +
  multi-recipient round-trips, both cross-check throws, and a
  binary payload round-trip. Pure stdlib + age (pure-Go) so it's
  in the CI offline subset.
- `--examples` step 32 covers the binding; MANUAL §5 gains the
  `api.encrypt` ts block plus a paragraph per op and a
  cross-check / wrong-identity rationale section.
- `cmd/sercon/api_docs.go` grows three entries so the emitted
  `api.d.ts` carries hover docs.

### Changed

- `OUT-OF-SCOPE.md`'s five-function `encrypt::*` entry collapsed
  to three forward-looking entries: `encryptArmored` (armoured
  ASCII format via `filippo.io/age/armor`), `rekey` (two-step
  decrypt-then-encrypt API design), and `detectBackend` (age vs
  PGP dispatcher — brings PGP support as a side effect).

## [0.5.4] — 2026-05-26

Fifth Moderate cut. **`api.barcode.decode`** — symmetric counterpart
to the existing `api.barcode.encode`. Reads PNG / JPEG / WebP image
bytes, finds the barcode, and returns `{ format, text }`. Two new
deps, both pure-Go: `github.com/makiuchi-d/gozxing` (decoder library;
covers 12 symbologies) and `golang.org/x/image` (WebP decoder
registered with stdlib's `image.Decode`).

### Added

- `api.barcode.decode(data, format?)` → `Promise<{ format, text }>`.
  Auto-detect by default (walks readers in 2D-first / constrained-1D
  / permissive-1D priority order); pass a `format` hint to lock the
  reader and surface mismatched-format errors clearly.
- `api.barcode.decodableFormats()` returns the supported decode set
  separately from `barcode.formats()` (encode) — the two are close
  but not identical because gozxing covers `upce` / `itf` / `code93`
  that the encoder doesn't, and lacks `pdf417` that the encoder
  does.
- PNG / JPEG support via stdlib `image/png` + `image/jpeg`; WebP
  via a blank-import of `golang.org/x/image/webp` that registers
  the decoder with `image.Decode` (so format detection is
  magic-byte sniffing rather than caller-declared).
- `sniffImageFormat` helper exposes PNG / JPEG / WebP magic-byte
  recognition for tests; the binding itself goes through
  `image.Decode` directly so format support stays consistent with
  whatever decoders are registered.
- `TestBarcodeDecode_*` (20 sub-tests): 2D round-trip × 3,
  1D round-trip × 3 (formats that don't need a quiet zone),
  EAN-13 round-trip with manually-padded quiet zone, auto-detect,
  non-image bytes throws, blank image throws, unknown format hint
  throws, sniffImageFormat × 5, JPEG-input round-trip.

### Changed

- `api.barcode.encode`'s `opts.width` / `opts.height` are no longer
  silently dropped. The binding was reading opts via the 2-arg
  `optsAsMap` helper but the call signature is
  `encode(format, data, opts)` so opts is at position 2, not 1.
  Same shape as the `diff.compare` / `archive.extract` fixes from
  earlier cuts. Width/height now propagate from JS through to
  boombuler's scaler.
- `OUT-OF-SCOPE.md`'s "Encoding / decoding / barcodes" entry
  resolved; replaced with two forward-looking items: a pure-Go
  PDF417 decoder (no obvious library — defer until needed) and an
  `opts.quietZone` flag on `encode` so EAN/UPC PNGs round-trip
  without caller-side padding.
- MANUAL §5 prose extends the barcode paragraph with the decode
  surface, the priority-order auto-detect, and four documented
  quirks (PDF417 encode-only, EAN/UPC quiet-zone requirement,
  Code 39 checksum char, codabar wrapper stripping).

## [0.5.3] — 2026-05-26

Fourth Moderate cut. Extends `api.jwt.*` with the **full asymmetric
algorithm matrix**: RSA (RS256/RS384/RS512), RSA-PSS (PS256/PS384/
PS512), ECDSA (ES256/ES384/ES512), and Ed25519 (EdDSA). v0.5.2's
HMAC support is unchanged. No new dependencies — the existing
`github.com/golang-jwt/jwt/v5` already covers them; the work was
the JS-side key-shape design and cross-check guards.

### Added

- All asymmetric algorithm names accepted in `opts.algorithm`:
  RS256 / RS384 / RS512 / PS256 / PS384 / PS512 / ES256 / ES384 /
  ES512 / EdDSA. `jwtSupportedAlgoList` reports them in error
  messages alongside the HMAC trio.
- **PEM key shape.** The `secret` parameter is overloaded by the
  algorithm: HMAC algorithms use the byte string directly;
  asymmetric algorithms expect a PEM-encoded key (private for
  `sign`, public or certificate for `validate`). PEM detection is
  the literal `-----BEGIN` prefix. Supports PKCS#1 + PKCS#8 + SEC1
  formats via jwt-go's `Parse*FromPEM` helpers.
- **Bidirectional cross-check** at the binding boundary, throwing
  with named hints rather than letting silent footguns through:
  - PEM secret + HMAC algorithm → "looks like a PEM-encoded key —
    set opts.algorithm to RS256 / ES256 / EdDSA / etc."
  - Plain-bytes secret + asymmetric algorithm → "needs a
    PEM-encoded private/public key but secret is plain bytes"
  - These fire before jwt-go is called so they're throws rather
    than `valid: false` resolutions. The validate side
    pre-decodes the token header (we already have the segments
    split) to derive the algorithm when `opts.algorithm` isn't
    set, so the check fires uniformly.
- **Algorithm-confusion guard.** When `opts.algorithm` is set on
  validate, jwt-go's `WithValidMethods` whitelist restricts the
  parser to that single algorithm. Without this guard, an attacker
  with access to the public key bytes could forge an HS256 token
  using those bytes as the HMAC secret (the classic JWT exploit).
  Sign-side cross-check blocks the construction; validate-side
  whitelist blocks acceptance of externally-forged tokens.
- `TestJwt_AsymmetricRoundTrip` (8 sub-tests covering RS256 /
  RS384 / RS512 / PS256 / PS384 / PS512 / ES256 / ES384 / EdDSA);
  `TestJwt_AsymmetricRoundTripES512` is isolated since slow CI
  runners occasionally trip on P-521 deterministic-curve overhead.
  `TestJwt_AsymmetricWrongPublicKeyResolvesFalse`,
  `TestJwt_AsymmetricCrossCheckErrors`, and
  `TestJwt_AlgorithmConfusionGuard` (jwt-go-forged HS256 token
  presented to a validator expecting EdDSA — rejected).
- `pemKeyPair(t, alg)` test helper generates fresh in-memory key
  pairs per test so no key material is committed to the repo.
- `examples/scripts/jwt.ts` grows an Ed25519 round-trip plus the
  PEM-cross-check demonstration. The demo keys are clearly
  labelled test fixtures.

### Changed

- `TestJwt_AsymmetricAlgoRejected` is renamed
  `TestJwt_UnsupportedAlgoRejected` and trimmed: the old test
  asserted that RS256 / ES256 / EdDSA were rejected, which is no
  longer true. The remaining cases (`none`, `HS999`, `RSA-OAEP`)
  still throw with "unsupported algorithm" wording. The
  empty-algorithm-string case is deliberately dropped because the
  default (HS256) is the right behaviour for scripts passing opts
  for audience / issuer without an algorithm preference.
- `optAlgorithm` and the validate-side algorithm normaliser
  preserve EdDSA's mixed-case identifier — jwt-go's canonical
  name is `EdDSA` (not `EDDSA`), so `ToUpper` is wrong for that
  one specifically.
- MANUAL §5 prose extended with the PEM key shape, the
  bidirectional cross-check rules, and the algorithm-confusion
  guard. `--examples` step 30 rewritten to show an asymmetric
  flow alongside the HMAC flow.
- `OUT-OF-SCOPE.md`'s "Asymmetric JWT algorithms" entry resolved
  (shipped); replaced with a forward-looking JWK-key-shape entry
  (jwx/v2/jwk or hand-rolled).

## [0.5.2] — 2026-05-26

Third Moderate cut. **`api.jwt.*`** — sign / view / validate over
HMAC-signed JWTs. New dependency:
`github.com/golang-jwt/jwt/v5 v5.3.1` (pure Go, de facto standard).
Asymmetric algorithms (RSA / ECDSA / EdDSA) are deliberately split
off into a follow-up cut so the JS-side key-shape design can be
done in isolation.

### Added

- `api.jwt.sign(claims, secret, opts?)` produces a
  compact-serialisation signed JWT. `opts.algorithm` defaults to
  `"HS256"`; `"HS384"` and `"HS512"` are also supported. Claims
  pass straight through to jwt-go's `MapClaims`, so RFC 7519
  reserved claims (`exp`, `nbf`, `iat`, `aud`, `iss`, `sub`,
  `jti`) work alongside arbitrary application claims. Missing
  reserved claims aren't synthesised.
- `api.jwt.view(token)` decodes header + payload **without
  verifying the signature** and returns `{ header, payload,
  signature }`. Useful for debugging auth flows or surfacing
  `aud` / `iss` to the user before deciding to trust the token.
  Both `RawURLEncoding` (no padding) and the padded `URLEncoding`
  form are accepted on input; the returned `signature` is the
  raw base64url segment as it appeared in the token.
- `api.jwt.validate(token, secret, opts?)` verifies the signature
  + standard claims and resolves with
  `{ valid: true, claims }` or `{ valid: false, reason }`. The
  resolve-don't-throw contract is the key design point: scripts
  branch on `valid` for bad signature / expired / nbf / aud
  mismatch / iss mismatch. Only structural input errors (wrong
  segment count, invalid base64, invalid JSON, empty secret /
  empty token) throw — those aren't validation failures and a
  script pattern-matching on `valid: false` shouldn't
  accidentally accept a garbage string.
- `opts.audience` / `opts.issuer` propagate into jwt-go's
  `WithAudience` / `WithIssuer` parser options when set; absent,
  jwt-go skips those checks.
- Unsupported algorithm names — including `RS256`, `ES256`,
  `EdDSA`, and the special `"none"` value — throw at `sign` /
  `validate` time with a named-algorithm error. Silent fallback
  to a weaker algorithm than requested would be a security
  footgun.
- `TestJwt*` (19 sub-tests covering: round-trip per algorithm
  HS256/HS384/HS512, view-without-secret, bad-signature
  resolve-false, expired-token resolve-false, audience and issuer
  mismatch resolve-false, four asymmetric-algo rejections, four
  malformed-input throws, empty-secret throws on both sign and
  validate).
- `examples/scripts/jwt.ts` walks through every form; pure stdlib
  so it runs in the CI offline subset.
- `--examples` step 30 covers the binding; MANUAL section 5 gains
  the `api.jwt` block plus a paragraph per op.
- `cmd/sercon/api_docs.go` grows three entries so the emitted
  `api.d.ts` carries hover docs.

### Changed

- `OUT-OF-SCOPE.md`'s JWT entry rewritten: HMAC support is shipped;
  the asymmetric matrix (RSA / ECDSA / EdDSA) is now an explicit
  follow-up item with notes on the JS-side key-shape design.

## [0.5.1] — 2026-05-26

Second Moderate cut. **`api.preg.*`** — PHP-style `/pattern/flags`
regex syntax on top of Go's stdlib `regexp` (RE2). The "preg"
naming is a deliberate homage; the semantics are RE2's. No new
dependencies.

### Added

- `api.preg.match(pattern, subject)` — first hit as
  `{ match, groups, index }`, or `null` when nothing matches.
  Optional groups that didn't match surface as empty strings (not
  `undefined`) so the result shape stays stable across calls.
- `api.preg.matchAll(pattern, subject)` — same shape, drained
  into an array.
- `api.preg.replace(pattern, replacement, subject)` — substitutes
  via Go's `$1` / `${1}` backref syntax. PHP's `\1` form is **not**
  translated — `\\` escapes are already legitimate in the
  replacement and aliasing them would surprise users coming from
  Go's `regexp.ReplaceAllString`.
- Supported flags: `i` / `m` / `s`, merged into Go's `(?ims)`
  inline-flag prefix. Unsupported PHP flags (`u`, `U`, `x`) and
  unknown flags throw with a clear, named-flag error rather than
  silently dropping. The `u` error explicitly notes that RE2 is
  UTF-8 by default so the flag is unnecessary.
- `TestPreg*` (15 sub-tests covering null-on-no-match, first-match
  groups + index, matchAll length, replace backrefs, all three
  supported flags, four unsupported / unknown flag forms, three
  malformed-pattern forms, optional-group empty-string).
- `examples/scripts/preg.ts` walks through every form above plus
  the unsupported-flag error path. Included in `make demo` and the
  CI offline subset (the binding is pure stdlib so it always
  works).
- `--examples` step 29 covers the binding; MANUAL section 5 gains
  the `api.preg` block plus a paragraph on the RE2-vs-PCRE
  semantics difference and the flag rules.
- `cmd/sercon/api_docs.go` grows three entries so the emitted
  `api.d.ts` carries hover docs for `match` / `matchAll` /
  `replace`.

### Changed

- `OUT-OF-SCOPE.md`'s entry for `preg_match` / `preg_replace` is
  rewritten as a forward-looking note about a possible
  `regexp2`-backed PCRE binding — RE2 covers most uses but
  scripts that genuinely need lookahead / lookbehind / pattern
  backrefs would justify a sibling `api.preg2.*` namespace.

## [0.5.0] — 2026-05-26

First Moderate cut: **JSDoc support in the `.d.ts` emitter.** Editor
hover for `api.*` bindings is now populated, and library embedders
have a first-class way to attach docs without writing struct tags or
sibling registration data.

Bumped to 0.5.0 by convention (Moderate bucket → 0.5.x); the
release-please manifest follows along. No new runtime dependencies.

### Added

- `Engine.SetDocs(path, doc)` and `Engine.SetMemberDocs(namespace, docs)`
  attach JSDoc strings to registered bindings. `path` is the dotted
  lookup key — a bare name for top-level bindings (`"log"`,
  `"http"`), or `"namespace.member"` for namespace members
  (`"http.get"`, `"exec.shell"`). The doc map lives on the engine
  rather than the registration, so the same `SetDocs` call works
  regardless of which of the five `Register…` variants was used.
- Multi-line docs (split on `\n`) expand to a standard
  `* `-prefixed JSDoc block, preserving blank lines as bare `*`
  lines. Single-line docs collapse to `/** … */`. Bindings without
  a doc entry emit no JSDoc block at all (no empty `/** */`
  placeholders).
- Calling `SetDocs` with an empty string removes any previously
  set doc for that path; that's also the documented way to undo a
  doc entry.
- `TestWriteTypes_Docs*` (5 sub-tests in `engine_test.go`):
  single-line top-level, multi-line expansion with blank-line
  handling, namespace member docs, absent-doc-no-block, empty-string
  removes.
- `cmd/sercon/api_docs.go` ships a curated doc map for every
  binding under `api.*`. The CLI calls `Engine.SetDocs("api", …)`
  and `Engine.SetMemberDocs("api", apiDocs())` so the emitted
  `examples/scripts/api.d.ts` now grows readable editor hover for
  the example surface.
- MANUAL.md gains a `SetDocs` / `SetMemberDocs` subsection in §3
  (library API) and a JSDoc-on-declarations subsection in §13
  (type generation).

### Changed

- `writeDTS` now accepts an `(regs, docs)` pair; `Engine.WriteTypes`
  passes the engine's doc map through.
- `writeMemberObject` carries a path prefix so nested sub-namespaces
  look up docs under the right dotted key.
- The lockstep checklist in CLAUDE.md grows a seventh artifact:
  `cmd/sercon/api_docs.go` must be updated when adding or changing
  a binding.
- `make version-check` and `make release-prep` now include
  `.release-please-manifest.json` as a fourth version marker so
  it stays in sync with the Go const and the two MANUAL.md
  strings.
- `OUT-OF-SCOPE.md`'s Moderate / `.d.ts` generator subsection is
  removed; this was its only entry.

## [0.4.21] — 2026-05-26

Repo / tooling cut — no new bindings, no script-API surface change.
Wires `release-please` as the primary release driver, retiring the
manual "edit CHANGELOG, `git tag`, push" flow. Closes the Easy
bucket of `OUT-OF-SCOPE.md` (every entry shipped across v0.3.0 –
v0.4.21).

### Added

- `.github/workflows/release-please.yml` runs `googleapis/release-please-action@v4`
  on every master push. The action maintains a "chore: release
  X.Y.Z" PR built from Conventional-Commits subjects since the last
  tag. Merging the PR bumps version markers, updates
  `CHANGELOG-AUTO.md`, and pushes a `vX.Y.Z` tag. `skip-github-release: true`
  is set so the existing tag-triggered release workflow keeps
  ownership of binary publishing.
- `release-please-config.json` — `release-type: simple`,
  `changelog-path: CHANGELOG-AUTO.md` (so the hand-curated
  CHANGELOG.md is never overwritten), and `extra-files` pointing at
  `pkg/scriptengine/version.go` and `MANUAL.md` so all three
  version markers are rewritten as one unit. `changelog-sections`
  surfaces `feat` / `fix` / `perf` / `refactor` / `docs` / `build` /
  `ci` and hides `chore`.
- `.release-please-manifest.json` pinned at `0.4.21` so
  release-please knows where to start counting from.
- `x-release-please-version` end-of-line marker comments on each
  versioned line: the `const Version` in `pkg/scriptengine/version.go`
  and the two MANUAL.md version strings (cover block + footer).
  The generic file updater finds these markers and rewrites just the
  version digits on the line, leaving everything else intact.

### Changed

- `.github/workflows/release.yml` is now documented as fired from
  either path — release-please merge or manual `git tag` — and the
  goreleaser job description acknowledges both entry points.
- `make release-prep VERSION=x.y.z` is reframed in the Makefile
  comment as a manual fallback for ad-hoc local bumps. The sed
  patterns were tightened with capture groups so they preserve the
  trailing marker comments. The `version-check` target's sed regex
  was anchored to end-of-line so the captured value isn't polluted
  by the marker comment.
- `CLAUDE.md`'s Versioning, CI, and release-flow sections now
  describe release-please as the primary driver.
- `OUT-OF-SCOPE.md`'s Easy bucket no longer has a Repo / tooling
  subsection — the only remaining entry shipped here.

## [0.4.20] — 2026-05-26

Final slice of **Easy / External tool integrations**: a wrapper
around the GitHub `gh` CLI. With this cut, every entry in the Easy
bucket has shipped. Still no new dependencies — `os/exec` and
`encoding/json`.

### Added

- `api.gh.authStatus()` → `Promise<{ authenticated, user, raw }>` —
  status probe. Runs `gh api user --jq .login` (machine-friendly)
  rather than `gh auth status` (multi-line human report). Missing
  gh and unauthenticated sessions resolve as data (`authenticated:
  false`), not throws — a status probe scripts can branch on.
  Context cancellation still throws.
- `api.gh.prList(opts?)` → `Promise<Array<{ number, title, state,
  author, headRefName, baseRefName, url, createdAt, updatedAt }>>`.
  Lists pull requests on the repo identified by `opts.cwd` (or the
  engine's working directory). Defaults: open state, limit 30.
  Filters: `state`, `limit`, `author`. `gh`'s `author: { login, …}`
  wrapper is flattened to a bare login string.
- `api.gh.repoView(repo?, opts?)` → `Promise<{ name, owner,
  description, url, defaultBranch, visibility }>`. With no arg,
  uses the cwd's repo; pass `"owner/name"` for any repo `gh` has
  access to. Convenience flattenings: `owner` is a login string
  (not `{login, …}`), `defaultBranch` is the branch name (not
  `defaultBranchRef.name`). Empty repos resolve with
  `defaultBranch: ""` rather than `undefined`.
- `parsePRListJSON` and `parseRepoViewJSON` are pulled out as
  testable helpers so the JSON-flattening logic can be exercised
  without spawning `gh`.
- `TestParsePRListJSON_*` / `TestParseRepoViewJSON_*` (5 sub-tests):
  author-wrapper flattening, null-author preserved, owner +
  defaultBranchRef flattening, null defaultBranchRef yields
  `defaultBranch: ""`, malformed-JSON error.
- `TestGhAuthStatus_NoGhResolvesFalse` pins the no-throw-on-missing
  contract via `t.Setenv("PATH", "/nonexistent")`.
- `TestGhAuthStatus_AgreesWithGhAuthStatus` is a skip-when-missing
  integration probe that cross-checks our boolean against `gh auth
  status`'s real exit code on the host.
- `examples/scripts/gh.ts` gracefully degrades when `gh` is missing
  or unauthenticated so it can be part of `make demo` everywhere.
  Excluded from CI (needs network + a logged-in account).
- `--examples` step 28 covers the binding; MANUAL section 5 gains
  the `api.gh` block plus three bullets of prose.
- `examples/scripts/api.d.ts` regenerated.

### Changed

- `OUT-OF-SCOPE.md`'s Easy bucket no longer has an External tool
  integrations subsection — every entry shipped across v0.4.17 –
  v0.4.20.

## [0.4.19] — 2026-05-26

Third slice of **Easy / External tool integrations**: a wrapper
around the host `git` CLI. `api.gh` is split off into the next cut
(v0.4.20) — given the size of the git surface, packing both
together would have made one over-large release. No new
dependencies — `os/exec` and a single `regexp` for diffStat are all
this needs.

### Added

- `api.git.branch(opts?)` → `Promise<{ current, detached, all }>` —
  current branch (empty on detached HEAD), detached flag derived
  from git's own exit code on `symbolic-ref --short HEAD`, and the
  list of local branches.
- `api.git.isClean(opts?)` → `Promise<boolean>` — convenience
  boolean over `git status --porcelain` emptiness.
- `api.git.revParse(rev, opts?)` → `Promise<string>` — full 40-char
  SHA. Invalid refs throw with git's stderr message in the chain.
- `api.git.status(opts?)` → `Promise<Array<{ path, indexStatus,
  workingStatus }>>` — parsed porcelain v1 output. The status
  fields are the raw single-char codes (`M`, `A`, `D`, `R`, `?`, …)
  so scripts can dispatch without re-parsing.
- `api.git.add(paths, opts?)` — stages one path or a list. `--` is
  inserted between `add` and the paths so leading-dash values work.
- `api.git.commit(message, opts?)` → `Promise<{ sha }>` — empty
  message throws before spawning. `allowEmpty:true` toggles
  `--allow-empty`. Returns the post-commit HEAD SHA.
- `api.git.log(opts?)` → `Promise<Array<{ sha, shortSha, author,
  email, timestamp, subject }>>` — `limit` (default 50) most-recent
  commits in `revRange` (default `HEAD`). Tab-separated format
  string keeps parsing one-line-per-commit.
- `api.git.diffStat(opts?)` → `Promise<{ files, insertions,
  deletions }>` — aggregates `git diff --shortstat`. Default range
  is `HEAD~1..HEAD`. Pure-add or pure-delete diffs return zero on
  the missing side instead of throwing.
- `api.git.runText(args, opts?)` → `Promise<{ stdout, stderr,
  exitCode }>` — escape hatch for invocations the typed bindings
  don't cover. Non-zero exits surface as data (not a throw); spawn
  failures and context cancellation still throw.
- All bindings accept `opts.cwd` so a single engine can work across
  multiple checkouts.
- `TestGit*` (10 sub-tests): fresh repo, dirty-tree status,
  revParse + invalid ref, add + commit round-trip, empty-message
  guard, log ordering and fields, diffStat counters, runText
  non-zero exit, runText input validation, detached-HEAD reporting.
- `examples/scripts/git.ts` builds a throwaway temp repo, exercises
  the read-only ops, demonstrates runText, then cleans up after
  itself. Included in `make demo` and the CI offline subset (git is
  on PATH everywhere we care about).
- `--examples` step 27 covers the binding; MANUAL section 5 gains
  the `api.git` block plus a paragraph per op.
- `examples/scripts/api.d.ts` regenerated.

### Changed

- `optInt` (previously private to barcode) now accepts plain Go
  `int` and `nil` opts maps in addition to `int64` / `float64`.
  Mirrors the `optMillis` change from v0.4.18 and lets test
  harnesses hand in maps without `int64()` casts.
- `OUT-OF-SCOPE.md`'s Easy / External tool integrations list drops
  the `git(repo_path)` entry. `gh()` remains as the v0.4.20 cut.

## [0.4.18] — 2026-05-26

Second slice of **Easy / External tool integrations**: HTTP via
`recon` (preferred) with `curl` as a fallback. Still no new
dependencies — `os/exec` and the system binaries do the work.

### Added

- `api.exec.http(method, url, opts?)` → `Promise<{ status, headers,
  body, durationMs, backend }>` — curl-compatible HTTP client.
- `opts.headers` is a `Record<string, string>` of request headers;
  `opts.body` is piped to the backend as `--data-binary @<tempfile>`
  so CR / LF survive verbatim; `opts.timeout` defaults to 30 000 ms;
  `opts.follow` toggles `-L`; `opts.insecure` toggles `-k`.
- `opts.backend` picks the binary explicitly: `"auto"` (default —
  recon first, curl as a fallback), `"recon"`, or `"curl"`. The
  result tells you which backend ran via `backend`.
- Response headers are dumped to a temp file via `-D <path>` and
  parsed back, rather than relying on `-i`'s body stream. Recon's
  `-i` is verbose-debug style (`< Header: value`), so a unified
  parser across the two backends needed an out-of-band channel.
- Redirect chains (with `follow: true`) skip past intermediate 3xx
  blocks and report the final response.
- HTTP 4xx / 5xx do not throw — they're a normal HTTP outcome and
  callers branch on `status`. Process-start failures, transport
  errors, and context deadline / cancel throw.
- `TestExecHTTP_*` (8 sub-tests): baseline GET, POST with body +
  headers, 4xx-doesn't-throw, transport error, timeout throws,
  forced curl backend, forced recon backend, input validation.
- `TestParseHeaderFile_RedirectChain` /
  `TestParseStatusCode_ReconQuirkyStatusLine` /
  `TestParseStatusCode_NoCode` pin the parser against the quirks
  observed in the real backends (recon prints
  `HTTP/HTTP/2.0 200 OK`, curl prints `HTTP/2 200`).
- `examples/scripts/exec-http.ts` walking through every form above;
  hits httpbin.org so it's in `make demo` but not in CI (same
  policy as `net-probe.ts` and `email-auth.ts`).
- `--examples` step 26 covers the binding; MANUAL section 5 gains
  the `api.exec.http` block plus a paragraph on the throw-vs-resolve
  contract and the backend-selection rules.
- `examples/scripts/api.d.ts` regenerated.

### Changed

- `optMillis` now accepts a plain Go `int` in addition to `int64`
  and `float64`. JS-side integers go through goja as `int64`, but
  Go-level test harnesses that build option maps directly sometimes
  hand in `int`. Tolerating both removes a silent fall-back-to-
  default footgun.
- `OUT-OF-SCOPE.md`'s Easy / External tool integrations list drops
  the recon-with-curl-fallback HTTP entry (now shipped); `git` and
  `gh` wrappers remain as the v0.4.19 cut.

## [0.4.17] — 2026-05-26

First slice of **Easy / External tool integrations**: a generic
subprocess runner, the foundation for the next two cuts (HTTP via
recon-with-curl-fallback in 0.4.18, git/gh wrappers in 0.4.19). No
new dependencies — `os/exec` and `context` from the standard library.

### Added

- `api.exec.shell(cmd, opts?)` → `Promise<{ stdout, stderr,
  exitCode, success, durationMs }>` — generic subprocess runner.
- String `cmd` is passed verbatim to `/bin/sh -c` (or `cmd /C` on
  Windows) so pipes, redirects, and globs work as a user would
  type them. Array `cmd` is treated as `argv` directly with no
  shell involvement.
- `opts.cwd` sets the working directory; `opts.env` is merged on
  top of the parent process environment (not a replacement);
  `opts.timeout` defaults to 30 000 ms; `opts.stdin` is piped to
  the child.
- Non-zero exits resolve with `success: false` and the real
  `exitCode` — they do not throw, since "ran the linter, expected
  to be told it failed" is a routine outcome.
- Process-start failures (binary not on `PATH`, permission denied)
  and context deadline / cancellation throw — those aren't normal
  subprocess outcomes.
- `TestExecShell_*` (8 sub-tests): string cmd via shell, argv mode,
  non-zero exit not thrown, stdin pipe, env override, cwd
  respected, timeout throws, input validation.
- `examples/scripts/exec-shell.ts` walking through every form
  above; included in `make demo` and the offline CI step.
- `--examples` step 25 covers the binding; MANUAL section 5 gains
  the `api.exec.shell` block plus a paragraph on the
  throw-vs-resolve contract.
- `examples/scripts/api.d.ts` regenerated.

### Changed

- `OUT-OF-SCOPE.md`'s Easy / External tool integrations list drops
  the `shell(cmd, opts?)` entry (now shipped); the `recon-with-curl`
  HTTP entry and the `git` / `gh` wrappers remain as the next two
  cuts.

### Fixed

- `api.archive.extract(path, dest, opts)` now respects
  `opts.overwrite`. The previous implementation read opts via the
  2-arg `optsAsMap` helper but the binding takes three positional
  args, so the helper saw `destDir` (a string) as the opts arg and
  silently fell back to `overwrite: false`. Re-extracting into a
  populated destination tripped `O_EXCL` even when callers had
  explicitly opted in. Pinned with a new
  `TestArchiveExtract_OverwriteOptThroughBinding` that drives the
  binding through its real `FunctionCall` shape (the existing
  `TestArchive_OverwriteFlag` exercised the internal `extractZip`
  helper directly and so missed the marshalling layer).

## [0.4.16] — 2026-05-26

Sole cut on **Easy / JSON / querying**. One new pure-Go dep
(`itchyny/gojq`) plus a small normalisation pass to bridge goja's
integer types onto gojq's runtime expectations.

### Added

- `api.jq.query(data, filter)` → `Promise<unknown>` — runs the
  filter and returns the first emitted value (or `null` when the
  filter emits nothing).
- `api.jq.queryAll(data, filter)` → `Promise<unknown[]>` — drains
  the iterator and returns every emitted value.
- Filter syntax is full jq via
  [`github.com/itchyny/gojq`](https://github.com/itchyny/gojq) —
  field access, `.[]` explode, `select`, `map`, `add`, `group_by`,
  optional access via `?`, anything jq itself supports.
- Data round-trips through goja's `.Export()` as `map[string]any` /
  `[]any` trees, which gojq accepts directly.
- `TestJq_RunJqQuery` (5 sub-tests): scalar field, first element,
  exploded names, `select`-filtered admins, computed sum via
  `[.users[].age] | add`. Pins both the first-result and
  full-iterator counts.
- `TestJq_ParseError`: syntax errors surface a clear Go error
  mentioning "parse".
- `TestJq_RuntimeError`: in-iterator type errors (gojq emits them
  as in-band values) get type-asserted out and surfaced as Go
  errors / JS throws.
- `TestJq_OptionalMissing`: `.does.not.exist?` returns one
  result equal to `nil`, which goja converts to JS `null`.
- `examples/scripts/jq.ts` walks scalar / explode / select / sum /
  group_by / optional access patterns, plus a try/catch around a
  parse-error filter.
- `--examples` step 23 added; existing email step shifts to 24.
  `exampleCount` is now 24.

### Changed

- `MANUAL.md` § Built-in `api` declares the `jq` shape; new prose
  block covers the data-shape expectation, the int64 normalisation
  detail, and the error-handling contract.

### Fixed

- Without explicit normalisation gojq panics with
  `invalid type: int64` on any arithmetic against
  goja-exported integers (every JS-side integer becomes an
  `int64` after `Export`). `normaliseForJq` walks the input tree
  and coerces all sized integers to plain `int` and `float32` to
  `float64` before handing it to gojq. The reproducer would be as
  simple as `.users[].age` on a JSON-ish input — fixed inline.

### Dependencies

- New direct (pure Go):
  `github.com/itchyny/gojq v0.12.19`,
  transitive `github.com/itchyny/timefmt-go v0.1.8` (used by
  gojq's `strftime` / `strptime` filters).

## [0.4.15] — 2026-05-26

Sole cut on **Easy / Data comparison** — unified-diff helper plus a
forced bump of the minimum Go version. One new pure-Go dep.

### Added

- `api.diff.compare(a, b, opts?)`:
  - Inputs are strings (UTF-8) or any byte sequence (`ArrayBuffer`,
    `Uint8Array`).
  - Returns `{ identical, binary, added, removed, diff, format }`.
  - `identical` short-circuits with an empty `diff`. `binary`
    (NUL byte in the first 8 KB) likewise — a unified diff of
    binary content isn't useful. Otherwise `diff` is the unified
    diff text and `added` / `removed` are body-only `+` / `-` line
    counts (file headers excluded, mirroring `git diff --shortstat`).
  - `opts`: `context` (default 3), `fromFile` (default "a"),
    `toFile` (default "b").
- Backed by `github.com/pmezard/go-difflib/difflib` for the unified
  diff (preferred over `sergi/go-diff` because that's char-level;
  pmezard produces unified diffs directly).
- `TestDiff_CountAddedRemoved` — pins the `+++ b` / `--- a` header-
  exclusion behaviour that lets `git diff --shortstat`-style counts
  come out right.
- `TestDiff_LooksBinary` — 5 fixtures including a NUL inside the
  8 KB sample window, a NUL past the sample window (should still
  be treated as text), UTF-8 prose, and empty input.
- `examples/scripts/diff.ts` walks through a non-trivial line-edit
  diff (with custom `fromFile` / `toFile`) plus the identical /
  binary short-circuits.
- `--examples` step 22 added; existing email step shifts to 23.
  `exampleCount` is now 23.

### Changed

- **`go.mod` `go` directive bumped from 1.22 to 1.25**, and the
  README badge / CI matrix updated to match. Several deps
  (`golang.org/x/text` v0.37, `golang.org/x/crypto` v0.52,
  `klauspost/compress` v1.18.6, `beevik/ntp` v1.5.0,
  `likexian/whois` v1.15.7) now require Go ≥ 1.24, and x/text
  specifically requires 1.25. The alternative was pinning all of
  those down — sacrificing security and bug fixes — so the floor
  moves instead.
- `MANUAL.md` § Built-in `api` declares the `diff` shape and
  documents the identical / binary short-circuits, the
  `+`/`-`-counting rule, and the `opts.{context,fromFile,toFile}`
  defaults.

### Fixed

- The shared `optsAsMap` helper reads the JS arg at position 1
  (typical `func(target, opts)` shape). `diff.compare(a, b, opts)`
  has its opts at position 2, so the helper would have returned
  nil. `diffCompare` reads the position-2 opts inline — the
  helper stays in place for the rest of the namespaces that use
  the position-1 convention.

### Dependencies

- New direct (pure Go): `github.com/pmezard/go-difflib v1.0.0`.

## [0.4.14] — 2026-05-26

Sole cut on **Easy / Archives & document handling**. Stdlib-only:
`archive/zip` + `archive/tar` + `compress/gzip`. No new go.mod deps.

### Added

- `api.archive.create(destPath, sources)`:
  - Format inferred from `destPath` extension: `.zip`, `.tar`,
    `.tar.gz` (also `.tgz`).
  - Sources are either bare strings (basename becomes the
    in-archive name) or `{path, name?}` objects.
  - Directories are walked recursively; the directory's basename
    becomes the archive subdir and descendants land relative to
    it. All in-archive paths use forward slashes so output is
    cross-platform.
  - Returns `{ path, format, entries, bytes }`.
- `api.archive.extract(archivePath, destDir, opts?)`:
  - Format inferred from `archivePath`'s extension.
  - `opts.overwrite` defaults to `false`; collisions on existing
    files cause an `os.ErrExist`-shaped failure unless explicitly
    overwritten.
  - Returns `{ path, format, dest, entries }`.
- **zip-slip / tar-slip protection** via `safeJoin`. Refuses any
  archive entry whose name is absolute (`/…` or `\…`), contains a
  `..` segment, or whose resolved target falls outside destDir.
  No silent sanitisation — malicious archives surface with a
  clear error rather than getting rewritten into legal-looking
  entries.
- `TestArchive_RoundTrip` (3 sub-tests): create + extract round-
  trip for zip / tar / tar.gz against a small fixture tree.
- `TestArchive_SafeJoinRejectsEscape`: classic `../`, embedded
  `sub/../../`, and absolute-path entries all rejected; legitimate
  nested paths still accepted.
- `TestArchive_ZipSlipRejected`: builds a hand-crafted tar with a
  `../escape.txt` entry; extractTar refuses it.
- `TestArchive_OverwriteFlag`: pre-existing destination file is
  preserved when `overwrite:false`, clobbered with `overwrite:true`.
- `examples/scripts/archive.ts` round-trips `README.md`,
  `CHANGELOG.md`, `LICENSE` through all three formats and extracts
  into sibling dirs. Verified live: zip 19 KB / tar 49 KB /
  tar.gz 18.6 KB; 3 entries extracted from each.
- `--examples` step 21 added; existing email step shifts to 22.
  `exampleCount` is now 22.

### Changed

- `MANUAL.md` § Built-in `api` declares the `archive` shape; new
  prose block covers the source-format conventions
  (string vs `{path, name}` entries), the directory-recursion
  rule, the `overwrite` opt's collision semantics, and the
  zip-slip / tar-slip rejection policy.

## [0.4.13] — 2026-05-26

Third and final cut on **Easy / Encoding / decoding / barcodes** —
the check-digit helpers. Pure stdlib math, no new deps. After this
release Easy / Encoding is empty (the scanner mentioned in v0.4.11's
plan lives in Moderate, not Easy).

### Added

- `api.checkdigit.*`:
  - `algos()` → `string[]` (`luhn`, `isbn10`, `isbn13`, `ean13`,
    `ean8`, `upca`).
  - `validate(algo, input)` → `boolean`. Non-digit characters,
    wrong-length input, and unknown algorithm names all return
    `false` — scripts can probe candidates without wrapping in
    try/catch.
  - `compute(algo, partial)` → `string`. Takes the input *without*
    its check digit and returns just that digit (or `"X"` for
    ISBN-10 position 10).
  - `inspect(algo, input)` → `{ algo, input, valid, given,
    computed }`. Union of validate + compute, useful for
    "tell me everything" diagnostics in interactive tools.
- Synchronous (no Promise) because the algorithms are local math
  with no I/O — sub-microsecond per call.
- Six algorithms implemented inline:
  - **Luhn** (right-to-left doubling, mod-10).
  - **ISBN-10** (weighted sum mod-11; position 10 encoded as `X`).
  - **ISBN-13** / **EAN-13** (weights `1,3,1,3,…`, mod-10). Aliased
    because the math is identical.
  - **EAN-8** / **UPC-A** (weights `3,1,3,1,…`, mod-10).
- `TestCheckdigit_KnownVectors` — every algorithm validates a
  well-known good vector, rejects a single-digit-flipped variant,
  and reconstructs the original check digit from the partial.
  Includes both numeric and `X`-suffix ISBN-10 cases.
- `TestCheckdigit_UnknownAlgorithm` — clean error paths.
- `TestCheckdigit_BadInput` (18 sub-cases) — empty, non-digit,
  and wrong-length inputs all surface false rather than panicking.
- `TestCheckdigit_InspectShape` — pins the inspect-return key set.
- `examples/scripts/checkdigit.ts` — validates + computes + inspects
  across all six algorithms. Verified live: every known vector
  validates and every partial reconstructs to the original check
  digit (including ISBN-10 `X`).
- `--examples` step 20 added; existing email step shifts to 21.
  `exampleCount` is now 21.

### Changed

- `MANUAL.md` § Built-in `api` declares the `checkdigit` shape and
  the per-algorithm input-length / weighting table. Notes the
  validate→`boolean`, compute→`string`, inspect→`object`
  signatures and the sync-vs-Promise distinction.

## [0.4.12] — 2026-05-26

Second of three cuts on **Easy / Encoding / decoding / barcodes** —
the charset side. v0.4.11 covered barcode encoders; v0.4.13 will
add check-digit helpers and a scanner. Two new pure-Go deps.

### Added

- `api.text.detect(data)`: charset detection via
  [`saintfish/chardet`](https://github.com/saintfish/chardet).
  Returns `{ charset, confidence, language?, candidates: [...] }`
  where `confidence` is the 0–100 scale chardet publishes.
- `api.text.encode(text, charset)`: UTF-8 string → bytes in target
  charset. Lossy conversion fails rather than silently dropping
  characters with no representation; callers wanting lossy
  behaviour pre-process the input.
- `api.text.decode(data, charset)`: bytes-in-charset → UTF-8 string.
  Mirror of `encode`; combined they round-trip every charset
  htmlindex knows.
- Charset names follow the WHATWG Encoding Living Standard aliases
  via `golang.org/x/text/encoding/htmlindex.Get` — UTF-8,
  ISO-8859-1, Windows-1252, Shift_JIS, GBK, GB18030, Big5,
  EUC-JP, EUC-KR, etc., plus every documented alias. Unknown
  names throw a clear error.
- `TestText_RoundTrip` (5 sub-tests): encode → decode round-trip
  across UTF-8 / Latin-1 / Windows-1252 / Shift_JIS / GBK.
- `TestText_UnknownCharset`: clean error from `htmlindex.Get` for
  bogus names.
- `TestText_DetectLatin1NotUTF8`: chardet must NOT classify
  Latin-1 bytes containing `0xE9` (è / é) as UTF-8.
- `examples/scripts/charset.ts` — round-trips five representative
  charsets and runs detect against a long Latin-1 sample.
  Verified live: chardet picks Windows-1252 at 78% confidence
  with French as the language hint.
- `--examples` step 19 added; the email step shifts to 20.
  `exampleCount` is now 20.

### Changed

- `MANUAL.md` § Built-in `api` declares the `text` shape; new
  prose block calls out the WHATWG-alias charset names, the
  encoder's strict (non-lossy) behaviour, and the
  candidate-list shape that `detect` returns.

### Dependencies

- New direct (pure Go):
  `github.com/saintfish/chardet v0.0.0-20230101081208-5e3ef4b5456d`,
  `golang.org/x/text v0.37.0` (promoted from indirect — the
  encoding family is needed at top level).

## [0.4.11] — 2026-05-26

First of three cuts on **Easy / Encoding / decoding / barcodes** — the
encoder side, covering ten barcode symbologies under one entry point.
Charset detection / round-tripping lands in v0.4.12; check-digit
helpers and a scanner (decode-from-image) in v0.4.13.

### Added

- `api.barcode.*`:
  - `encode(format, data, opts?)` → `Promise<Uint8Array>` (PNG bytes).
    Ten supported formats: `qr`, `datamatrix`, `aztec`, `pdf417` (2D);
    `code128`, `code39`, `codabar` (linear text); `ean13`, `ean8`,
    `upca` (retail).
  - `formats()` → string list so scripts can iterate.
  - Default size: 256×256 for 2D codes, 400×120 for linear / retail.
    Override via `opts.width` / `opts.height`.
- Format-appropriate encoder defaults: QR uses medium error
  correction + auto mode; Code 39 includes a Mod-43 checksum;
  PDF417 uses security level 5; Aztec runs at 33% ECC with
  auto-selected layers.
- `TestBarcode_EncodeAllFormats` — every supported format produces
  a valid PNG (signature + header + size check) against an
  appropriate payload. Sized assertions confirm the default-sizing
  logic picks 256×256 vs 400×120 correctly.
- `TestBarcode_UnknownFormat` — unknown format names surface a
  clean error mentioning the supported list.
- `examples/scripts/barcode.ts` iterates `formats()`, encodes a
  format-appropriate sample for each, and verifies the PNG
  signature. Also demos a custom-sized QR.
- `--examples` step 18 ("Barcodes (api.barcode.*)"); the existing
  email step shifts to 19. `exampleCount` is 19.

### Changed

- `MANUAL.md` § Built-in `api` declares the `barcode` shape with
  the full format union; new prose block covers the format
  categories (2D / linear / retail) and the encoder-specific
  defaults.
- `MANUAL.md` correction: the compression section claimed the
  return type was `ArrayBuffer`, but goja actually surfaces a Go
  `[]byte` return as `Uint8Array` (you read `.length`, not
  `.byteLength`). Both the type declaration and the prose now say
  `Uint8Array`.
- `examples/scripts/compression.ts` already used `new Uint8Array(...)`
  so the script was correct; only the doc was off.
- `OUT-OF-SCOPE / Easy / Encoding / decoding / barcodes` lost the
  three encoder bullets; charset detect / decode / encode and
  check-digit remain.

### Dependencies

- New direct (pure Go):
  `github.com/boombuler/barcode v1.1.0`.

## [0.4.10] — 2026-05-26

Sole cut on **Easy / Hashing & compression** — the compression piece.
Hashing already shipped in v0.3.1; this rounds out the sub-section.

### Added

- `api.compression.*`: a uniform compress / decompress surface over
  **nine pure-Go algorithms**:
  - **stdlib**: `gzip`, `deflate`, `zlib`, `bzip2` (read only — see
    below).
  - **third-party (pure Go)**: `bzip2` write via `dsnet/compress/bzip2`,
    `zstd` via `klauspost/compress`, `brotli` via `andybalholm/brotli`,
    `lz4` via `pierrec/lz4/v4`, `xz` via `ulikunitz/xz`, `snappy`
    via `golang/snappy`.
  - `api.compression.compress(algo, data)` / `decompress(algo, data)`
    accept `string` (UTF-8) or `ArrayBuffer` / `Uint8Array` input
    and return `ArrayBuffer`. `api.compression.algos()` returns the
    supported list so scripts can iterate without hard-coding.
- `compressBytes` / `decompressBytes` route by algorithm in a flat
  switch — each branch handles writer creation, write, and Close in
  the order each compressor's framing demands.
- `TestCompression_RoundTrip` (9 sub-tests) — every algorithm
  round-trips a ~960 B payload byte-for-byte.
- `TestCompression_UnknownAlgorithm` — unknown algo names surface a
  clean error rather than silently returning empty bytes.
- `examples/scripts/compression.ts` iterates `algos()` and reports
  compressed size + ratio for each. Verified live:
  brotli=5.4%, deflate=6.4%, zlib=7.1%, zstd=7.8%, gzip=8.4%,
  snappy/lz4=10.0%, xz=13.3%, bzip2=17.7% on the demo corpus.
- `--examples` step 17 added; existing email step shifts to 18.
  `exampleCount` is now 18.

### Changed

- `MANUAL.md` § Built-in `api` declares the `compression` shape with
  the full algo-set typed in the union, plus a prose section
  cross-referencing the upstream lib for each algorithm.
- `Makefile`'s `DEMO_SCRIPTS` and `examples/README.md` table absorb
  `compression.ts`. CI workflow's offline subset includes it too —
  compression doesn't touch the network.
- `OUT-OF-SCOPE` / Easy / Hashing & compression section is empty
  (hashing landed in v0.3.1; compression closes the slot now).

### Dependencies

- New direct (all pure Go):
  `github.com/klauspost/compress v1.18.6`,
  `github.com/andybalholm/brotli v1.2.1`,
  `github.com/pierrec/lz4/v4 v4.1.26`,
  `github.com/ulikunitz/xz v0.5.15`,
  `github.com/golang/snappy v1.0.0`,
  `github.com/dsnet/compress v0.0.1`.

## [0.4.9] — 2026-05-26

Second cut on **Easy / Email authentication**, completing the
sub-section. Adds MTA-STS, TLS-RPT, BIMI, and a parallel
`email.all` aggregator on top of v0.4.8's SPF + DMARC. Still
stdlib-only.

### Added

- `api.email.mtaSts(domain)` — looks up `TXT(_mta-sts.<domain>)`,
  parses the `v=STSv1; id=…` marker, then fetches and parses the
  RFC 8461 policy file at
  `https://mta-sts.<domain>/.well-known/mta-sts.txt`. Returns
  `{ present, record, txt: { v, id }, policy?: { version, mode,
  mx[], maxAge }, policyError? }`. Policy-fetch failures (TLS error,
  4xx, timeout) don't fail the binding — the TXT view is still
  returned and the fetch error surfaces as a string under
  `policyError`.
- `api.email.tlsRpt(domain)` — `TXT(_smtp._tls.<domain>)` lookup,
  parses `v=TLSRPTv1; rua=…` into a flat tag map and surfaces
  `rua` separately.
- `api.email.bimi(domain, opts?)` — looks up
  `<selector>._bimi.<domain>`, selector defaulting to `default`
  (override via `opts.selector`). Surfaces the logo URL `l` and
  VMC URL `a`.
- `api.email.all(domain)` — runs every probe in parallel via
  goroutines and returns an aggregate object keyed by probe name.
  Per-probe failures surface under `<probe>.error` so a partial
  result is still useful (e.g. SPF + DMARC found, MTA-STS policy
  fetch timed out).
- `parseMTASTSPolicy` — RFC 8461 line-based `key: value` parser.
  Repeated `mx:` lines aggregate into a slice; `max_age` coerces
  to an int when numeric.
- `TestParseMTASTSPolicy` — three table cases covering the
  canonical form, CRLF + comments + leading whitespace tolerance,
  and the non-numeric `max_age` fallback.
- `TestEmailNamespace_HandlesMissing` extended to cover all five
  individual probes plus the `email.all` aggregator.

### Changed

- Renamed `parseDMARCTags` to `parseTagMap` (the format is shared
  with BIMI / TLS-RPT / the MTA-STS TXT marker). Kept the old name
  as a thin alias so the existing test continues to compile.
- `examples/scripts/email-auth.ts` now demos all five probes and
  the aggregator against `google.com`.
- `--examples` step 17 absorbs the new bindings.
- `MANUAL.md` § Built-in `api` declares the three new shapes + the
  aggregator return type; a new prose block covers each probe's
  semantics and the MTA-STS policy-error fallback behaviour.
- `OUT-OF-SCOPE` / Easy / Email authentication section is now
  empty.

## [0.4.8] — 2026-05-26

First of two cuts on **Easy / Email authentication**. This one covers
SPF + DMARC — the two records with mature semantics and small, easy
parsing. MTA-STS / TLS-RPT / BIMI + the aggregate `email.all` land
in v0.4.9. All stdlib; no new `go.mod` deps.

### Added

- `api.email.spf(domain)`: looks up `TXT(domain)`, returns the first
  record starting with `v=spf1`. Mechanisms are tokenised, and the
  trailing `all` qualifier is summarised under `allPolicy` as one of
  `"pass" | "fail" | "softfail" | "neutral" | ""`. Missing records
  surface as `{ present: false }` rather than throwing; DNS
  operational errors throw as usual.
- `api.email.dmarc(domain)`: looks up `TXT(_dmarc.<domain>)`, parses
  the `v=DMARC1; key=val; …` form into a flat tag map (keys
  case-folded; values keep internal whitespace + commas so `rua`
  lists survive). Common tags are surfaced separately on the result:
  `policy`, `subdomain`, `percent`, `rua`, `ruf`. Same
  presence-false convention as SPF.
- `examples/scripts/email-auth.ts` against `google.com`. Network-
  dependent, so it joins `net-probe.ts` outside the CI offline
  subset.
- `--examples` step 17 ("Email authentication"); `exampleCount` is
  now 17.
- Tests:
  - `TestParseDMARCTags` (5 table cases) pins the tag-format
    behaviour offline: case-folded keys, whitespace tolerance,
    `rua` comma preservation, empty / malformed parts dropped.
  - `TestEmailNamespace_HandlesMissing` end-to-end: a bogus
    `.invalid` domain resolves to `{ present: false }` on both
    bindings, exercising the NXDOMAIN → presence-false path.

### Changed

- `MANUAL.md` § Built-in `api` declares the `email` shape; a new
  prose block calls out the presence-check convention, the SPF
  `allPolicy` summary, and the DMARC parser's case-folding and
  whitespace rules. A trailing sentence promises the remaining three
  protocols + aggregator for v0.4.9.
- `OUT-OF-SCOPE` Easy / Email authentication trimmed: SPF + DMARC
  drop out, leaving the (renamed) "MTA-STS / TLS-RPT / BIMI +
  aggregator" entry for the next cut.

## [0.4.7] — 2026-05-26

Second cut on **Easy / Protocol probes & connectivity** — the two
third-party-lib items that didn't fit v0.4.6's stdlib-only slice.
After this release, Easy / Protocol probes is empty.

### Added

- `api.net.ntp(host, opts?)`: NTPv4 clock query via
  [`github.com/beevik/ntp`](https://github.com/beevik/ntp). Returns
  `{ serverTime, offsetMs, rttMs, stratum, referenceTime,
  rootDelayMs, rootDispersionMs }`. Optional `{ timeout?, port? }`
  with defaults of 5 s and UDP 123.
- `api.net.whois(domain, opts?)`: two-hop WHOIS lookup (IANA →
  registrar's whois server) via
  [`github.com/likexian/whois`](https://github.com/likexian/whois)
  with parsing through
  [`github.com/likexian/whois-parser`](https://github.com/likexian/whois-parser).
  Returns `{ raw, domain?, registrar? }`; `raw` is always populated,
  parsed views are best-effort. `opts.timeout` defaults to 10 s.
- `TestNetNTP_UnreachableHostSurfacesError` — confirms the binding
  surfaces a host-side error (rather than crashing) when pointed at
  a closed UDP port. Offline; runs under `-short`.
- `TestNetWHOIS_InvalidDomainDoesNotPanic` — smoke test for the
  whois wrapper. Skipped under `testing.Short()` because there's no
  reasonable way to stand up a fake whois server locally; the test
  is mostly useful for confirming the dependency wires through.
- `examples/scripts/net-probe.ts` extended with `ntp` against
  `pool.ntp.org` and `whois` against `example.com`.
- `--examples` step 16 absorbs the two new probes; the note line
  now also documents the NTP default port.

### Changed

- `MANUAL.md` § Built-in `api` declares the two new probe return
  shapes, with prose calling out beevik/ntp's per-probe options and
  the likexian/whois library's context-less timeout plumbing.

### Dependencies

- New direct: `github.com/beevik/ntp v1.5.0`,
  `github.com/likexian/whois v1.15.7`,
  `github.com/likexian/whois-parser v1.24.21`. All pure Go.
- Transitive: `github.com/likexian/gokit v0.25.16`.

## [0.4.6] — 2026-05-26

First of two cuts on **Easy / Protocol probes & connectivity**. This
one covers the stdlib-only items — `tcp`, `dns`, `tls` — so no new
`go.mod` entries. `ntp` and `whois` (which need third-party libs)
land in v0.4.7.

### Added

- `api.net.tcp(target, opts?)`: TCP connect probe. Resolves the host,
  dials, and reports `{ host, port, ip, latencyMs }`. `target` is
  `"host:port"` or `"host"` (with `opts.port` overriding the default
  `"80"`). Optional `{ timeout: ms }`, default 5s. Backed by
  `net.Dialer.DialContext`.
- `api.net.dns(host, opts?)`: DNS lookups via `net.Resolver`. Returns
  `{ a, aaaa, mx, txt, cname, ns }` with empty record sets omitted so
  scripts can probe membership. `opts.types` scopes the query (case-
  insensitive subset of `"a" | "aaaa" | "mx" | "txt" | "cname" | "ns"`).
- `api.net.tls(target, opts?)`: leaf-certificate inspection. Returns
  `{ cn, issuer, notBefore, notAfter, daysRemaining, dnsNames,
  serialNumber, fingerprintSha256 }`. Dials with `InsecureSkipVerify`
  so even expired / hostname-mismatched certs come back inspectable.
  Default port is `443`. Hosts wanting validity verification should
  re-run `crypto/x509.Verify` themselves.
- `cmd/sercon/api_net_test.go`: four local-fixture tests covering
  TCP against a localhost listener, DNS against `localhost`, DNS
  types-filter behaviour, and TLS against a self-signed ECDSA server.
  No external network access — all four pass offline.
- `examples/scripts/net-probe.ts` demonstrates the three probes
  against `example.com`. Excluded from the CI offline subset; runs
  via local `make demo`.
- `--examples` walkthrough gains step 16 ("Protocol probes
  (api.net.*)"); `exampleCount` is now 16.

### Changed

- `MANUAL.md` § Built-in `api` declares the new `net` shape, plus a
  prose block calling out the timeout default, `target` parsing
  rules, the `InsecureSkipVerify` policy on `tls`, and the
  empty-set-omission behaviour on `dns`.
- `examples/README.md` table and `Makefile`'s `DEMO_SCRIPTS` gain
  `net-probe.ts`. CI workflow's "smoke examples" step explicitly
  enumerates the offline subset and notes the network-dependent ones
  in a comment.

## [0.4.5] — 2026-05-26

Fifth slice of the **Easy** bucket — the **Repo / tooling**
sub-section. CI workflow, cross-platform release pipeline, and a
local make target that handles the boring parts of cutting a
release. No code changes; everything lands as new YAML and
Makefile targets.

### Added

- **`.github/workflows/ci.yml`** — runs on every push to `master` and
  every PR. Matrix: Go 1.22 + latest stable, on `ubuntu-latest` and
  `macos-latest`. Each job: `go build` (slim flags), `go vet`,
  `go test ./...`, plus the offline subset of `examples/scripts/*`
  (excludes `async.ts`, which hits example.com). A separate `lint`
  job runs `golangci-lint@v2.12.2` (pinned to match `make lint`'s
  fallback).
- **`.github/workflows/release.yml`** — triggers on `v*.*.*` tag
  push. Runs `goreleaser release --clean` with the repo's
  `.goreleaser.yml`. Uses `GITHUB_TOKEN` only — no PAT needed.
- **`.goreleaser.yml`** — cross-compiles darwin-{amd64,arm64} /
  linux-{amd64,arm64} / windows-amd64. Mirrors `make release`'s
  `-trimpath` + `-s -w` flags. Each archive bundles LICENSE, README,
  CHANGELOG, MANUAL.md, and MANUAL.pdf. Changelog block on the
  release page is sourced from Conventional Commits with the
  noisy types (`docs:`, `chore:`, `test:`, `style:`) filtered out.
- **`make release-prep VERSION=x.y.z`** — bumps the three version
  markers (`pkg/scriptengine/version.go`, MANUAL cover, MANUAL
  footer), runs `version-check`, and prints the next-step checklist.
  The CHANGELOG move stays manual because that's the part that
  needs editorial judgement.
- **`make version-check`** — standalone sanity check that the three
  version markers agree. Run by `release-prep`; useful by itself
  after editing one of them by hand.

### Changed

- `CLAUDE.md`'s common-commands block lists the two new make targets.
- `CLAUDE.md` gains a "CI and release flow" section describing what
  each workflow does and which tags will or won't trigger
  `goreleaser` (tags pre-`v0.4.5` won't, because the YAML wasn't in
  those commits — by design).

### Notes

- Tags `v0.2.4`–`v0.4.4` are still local-only and will keep their
  source-only releases when eventually pushed. From `v0.4.5` onward,
  pushing the tag triggers `goreleaser` and the release gains
  prebuilt darwin / linux / windows archives plus `checksums.txt`.

## [0.4.4] — 2026-05-26

Fourth slice of the **Easy** bucket — the **CLI** sub-section. Stdin
script support, a distinct exit code per failure type, and a `-v` flag
that actually reveals something useful. No new external dependencies.

### Added

- **Stdin script support.** A `-` positional argument reads an entry
  script from `os.Stdin`. PASS / FAIL lines and trace output use
  `<stdin>` as the label. Arguments are still processed in order, so
  `sercon prelude.ts -` runs the prelude and then stdin.
- **`Options.Verbose io.Writer`** on `pkg/scriptengine`. When non-nil,
  receives engine traces prefixed `[sercon] `: the rewritten
  entry-script JS (the form goja actually runs, post-ESM-to-CJS
  rewrite + async IIFE) and each module-resolution event. The CLI's
  `-v` flag plugs in `os.Stderr`.
- **`scriptengine.ErrTranspile`** sentinel. Both entry-script and
  required-module transpile failures now wrap it via `fmt.Errorf("%w:
  ...", ErrTranspile, ...)`, so hosts can use `errors.Is` to
  distinguish "the script never ran" from a runtime throw.
- **Distinct CLI exit codes**, the documented matrix is:
  - `0` — all scripts passed.
  - `1` — CLI usage error.
  - `2` — at least one transpile error (`errors.Is(err, ErrTranspile)`).
  - `3` — at least one timeout / context cancel
    (`errors.Is(err, ErrScriptTimeout | context.Canceled | …)`).
  - `4` — at least one JS throw (anything else).
  - Highest applicable code wins across a multi-script invocation.
- `TestRun_TranspileErrorSentinel` — pins the `errors.Is` contract.
- `TestRun_VerboseWriterEmitsTraces` — sanity-checks that the engine
  emits both transpile-entry and require-resolved lines.

### Changed

- `cmd/sercon/main.go` is now structured around an exit-code return
  rather than the old `error`-and-`os.Exit(1)` shape; `main()` is a
  one-liner `os.Exit(run(os.Args[1:]))`.
- `--help` gains an `ARGUMENTS` block (positional / stdin semantics)
  and an expanded `EXIT STATUS` block listing all five codes plus the
  "highest wins" rule. Synopsis grows a `sercon [flags] -` line; an
  extra example shows the shell-pipeline form.
- `MANUAL.md` § 4 (CLI) absorbs the new flag semantics, the stdin
  usage block, the exit-code table, and a paragraph explaining what
  `-v` reveals.

## [0.4.3] — 2026-05-26

Third slice of the **Easy** bucket — the **`.d.ts` generator**
sub-section. Two real behavioural fixes plus a new tracked artifact
(`examples/scripts/api.d.ts`).

### Added

- **`AsyncBinding` carrier type** in `pkg/scriptengine`. The struct
  pairs the raw goja-callback `func(goja.FunctionCall) goja.Value`
  with a `TSReturnType` string computed from the generic parameter
  of `PromisifyAsync[T]`. The d.ts emitter reads this to emit
  `Promise<T>` for async bindings; the engine unwraps the struct to
  its `.Func` at registration time so goja's host-callback
  special-case still fires.
- **`make types`** target — regenerates `examples/scripts/api.d.ts`
  from the current CLI binding surface. The output is now tracked in
  git so editor autocomplete and reviewers see the public api shape
  without running the binary.
- **`examples/scripts/api.d.ts`** — checked-in artifact, regenerable
  via `make types`. Lockstep rule in `CLAUDE.md` extends to seven
  artifacts and now points at this file.
- `TestWriteTypes_StructMethodReceiver` and
  `TestWriteTypes_AsyncBindingPromise` in `engine_test.go` covering
  both fixes end-to-end.

### Changed

- **`PromisifyAsync[T]` return type** is now `AsyncBinding` rather
  than the bare `func(goja.FunctionCall) goja.Value`. Existing CLI
  bindings (`api.http.*`, `api.time.sleep`) work unchanged — the
  engine recursively unwraps any `AsyncBinding` it finds in
  `map[string]any` namespace bodies before handing values to
  `vm.Set`.
- **`structShape` reflects methods from the original (possibly
  pointer) type** instead of unwrapping to `Elem` first. Go exposes
  pointer-receiver methods on `*T`'s method set, not on `T`'s, so
  the old code dropped them silently for `Register("counter", &c)`
  style registrations. Field iteration still uses the underlying
  struct.
- **`funcSig` now takes an `isMethod` flag**, used by callers that
  pass `reflect.Method.Type` (whose `In(0)` is the receiver). The
  flag is `true` only at the two callers reflecting methods —
  `structShape` and `writeConstructorDecl`. Everywhere else it
  stays `false`. Receivers no longer leak into the printed
  parameter list.
- **`writeValueDecl` resolves `RegisterFactory` factories at d.ts
  time** by invoking them with `(nil, nil)`, matching what
  `writeNamespaceDecl` already did. A panic falls back to a TODO
  comment + `unknown`. Lets factory-built `AsyncBinding`
  registrations surface as `Promise<T>` instead of the previous
  TODO.

### Removed

- The unused `Promised[T any] func(...)` marker type. Its job is
  done by `AsyncBinding` via a struct field, which dodges goja's
  exact-type check for `func(goja.FunctionCall) goja.Value`
  callbacks (named func types fall through that check and end up
  on the reflect path, which can't pack JS args into a
  `FunctionCall`).

## [0.4.2] — 2026-05-26

Second slice of the **Easy** bucket — the **Require / module loading**
sub-section. Two real behavioural additions to the resolver plus a
JSON-import regression test. No new external dependencies (the rewrite
uses `encoding/json` from stdlib).

### Added

- **`.js` → `.ts` swap** in `Engine.resolveRequirePath`. When a request
  ends in `.js` / `.cjs` / `.mjs` and the literal file is absent, the
  resolver now tries the same path with `.ts` / `.tsx`. Handles
  `package.json` `main` fields that point at compiled output where
  only the TypeScript source is on disk.
- **`package.json` `source` preference** via a new
  `Engine.maybeRewritePackageJSON`. When the source loader is asked
  for a `package.json` and that file has a `source` field pointing at
  an existing `.ts`/`.tsx` file, the JSON is rewritten on the fly so
  `main` points at the TS source. Matches the convention used by
  parcel/microbundle/etc.
- Three new tests:
  - `TestRun_PackageJsonMainTSFallback` — `main: "lib/index.js"`
    plus an on-disk `lib/index.ts` resolves correctly.
  - `TestRun_PackageJsonSourcePreferred` — `source` wins over `main`
    even when `main`'s target exists, proving the rewrite isn't
    just a fallback.
  - `TestRun_JSONImport` — `import data from "./data.json"` and
    `require("./data.json")` both yield the parsed JSON object.
- Two new runnable examples + supporting fixtures:
  - `examples/scripts/json-import.ts` (+ `helpers/data.json`).
  - `examples/scripts/pkg-resolution.ts` (+ `helpers/pkg/` tree
    containing `package.json`, `src/lib.ts`, and a decoy
    `dist/index.js`).

### Fixed

- `Engine.resolveRequirePath` now returns
  `require.ModuleFileDoesNotExistError` instead of a private
  "module not found" wrapper. Without this, goja_nodejs's `loadAsFile`
  short-circuited the fallback chain on the very first miss — meaning
  `loadAsDirectory` (and therefore `package.json` resolution) never
  ran. This was a latent bug; the new package-json tests surfaced it.

### Changed

- `Makefile`'s `DEMO_SCRIPTS` list gains the two new examples.
- `examples/README.md` script table extended accordingly.
- `MANUAL.md` § 10 (Module resolution) documents the new
  `.js` → `.ts` swap and `package.json` `source` preference, plus an
  explicit JSON-import snippet.

## [0.4.1] — 2026-05-26

Example coverage for everything that landed in 0.3.x – 0.4.0. Pure
docs/examples patch; no behaviour change.

### Added

- `examples/scripts/hash.ts` — every `api.hash.*` algorithm + a known
  SHA-256 vector check.
- `examples/scripts/strings.ts` — `api.str.*` tour (trim with mask,
  pad, reverse, stripHtml, base64, url, html-entity, sprintf,
  normalizeNewlines).
- `examples/scripts/path-and-time.ts` — `api.path.*` and
  `api.time.format` with both an IANA zone and a local-zone variant.
- `examples/scripts/default-export.ts` (+ `helpers/answer.ts`) — proves
  the entry rewriter's `__esModule ? .default : module` unwrap.
- `examples/scripts/tsx-demo.ts` (+ `helpers/el.tsx`) — TSX module
  loading with an `@jsx h` pragma so JSX rewrites to a local factory.
- `make demo` Makefile target — runs every success-path example as a
  single command. Excludes `hang.ts` (intentional timeout demo).
  `DEMO_SCRIPTS` variable enumerates the list so future additions only
  need a one-line update.

### Changed

- `examples/README.md` gains a table of bundled scripts and what each
  demonstrates.
- `MANUAL.md` § Quickstart points at `make demo` and lists the demo
  inventory.
- `CLAUDE.md` "Keeping docs in lockstep" gains a sixth artifact:
  `examples/scripts/`. Any change to a user-visible binding, flag, or
  script-facing behaviour now must add or update the relevant example
  there and pass `make demo`. The rule explicitly carves out
  library-only changes (`WithScriptRoot`, `Engine.Reset()`) which
  don't need example growth.

## [0.4.0] — 2026-05-26

Opening cut of the **Easy** backlog bucket — the **Transpile / entry
rewriter** sub-section. No code changes to the engine; this is
test-and-doc coverage for paths that already worked but were never
proven end-to-end. The MINOR bump reflects the bucket transition, not
new behaviour.

### Added

- `TestRun_ESMDefaultExport` — proves `import answer from "./mod"`
  resolves to the value of `export default …` via the entry-script
  rewriter's `__esModule ? .default : module` unwrap.
- `TestRun_ESMDefaultAndNamed` — proves a mixed
  `import def, { named } from "./mod"` statement assigns both names
  correctly (uses `reImportDefAndN`).
- `TestRun_TSXModuleEndToEnd` — proves a `.tsx` helper module is
  resolved by extension fallback, transpiled by esbuild's `LoaderTSX`,
  and executes through goja. Uses an `@jsx h` pragma so JSX rewrites
  to a local factory rather than `React.createElement`.

### Changed

- `MANUAL.md` § 8 (TypeScript support) replaces the "wired but not yet
  exercised" caveat about `.tsx` / `.jsx` with a concrete usage block
  showing the `@jsx h` pragma pattern and the `export default`
  interop.

Final slice of the **Trivial** backlog bucket — the **Repo / tooling**
sub-section. Adds the linter config and shakes out the findings the
codebase had collected while we focused on features. After this cut
the Trivial bucket is empty; the next minor (v0.4.0) starts on the
**Easy** bucket.

### Added

- `.golangci.yml` — golangci-lint v2 config. Uses the `standard`
  default linter set (errcheck, govet, ineffassign, staticcheck,
  unused) with two small adjustments: `errcheck.check-type-assertions`
  is on, and the `govet.inline` analyzer is disabled because its
  suggestions to inline calls into x/crypto wrappers aren't actionable
  without bumping our Go minimum.
- `make lint` target. Uses the system `golangci-lint` if installed,
  otherwise falls back to a one-shot `go run` of the pinned version
  (`GOLANGCI_VERSION=v2.12.2`) so contributors don't need to install
  anything globally.

### Fixed

A clean lint pass surfaced 19 issues across the codebase. Highlights:

- `cmd/sercon/api_hash.go`: BLAKE3 now uses `blake3.Sum256` (one-shot)
  rather than instantiating a `Hasher` and discarding `Write`'s
  return value.
- `pkg/scriptengine/bindings.go`: explicit `_ =` discard on
  `resolve` / `reject` returns from `vm.NewPromise()`. The errors are
  only ever non-nil if the promise has already settled, which is
  impossible in our usage — making the discard explicit documents
  that.
- `pkg/scriptengine/dts.go`: `reflect.Ptr` → `reflect.Pointer` (the
  canonical name; `Ptr` is the deprecated alias).
- `cmd/sercon/help.go`: removed an unused `(*styler).section` helper
  and a dead-store assignment in the TS highlighter.
- `pkg/scriptengine/timeout.go`: deleted the unused `withInterrupt`
  helper. The file now holds only the `ErrScriptTimeout` sentinel,
  matching the actual architecture (the live cancellation watcher is
  inline in `engine.go`). `CLAUDE.md`'s description of the file is
  updated to match.

### Changed

- `CLAUDE.md` lists `make lint` in the common-commands block and the
  description of `pkg/scriptengine/timeout.go` no longer mentions the
  deleted helper.

Third slice of the **Trivial** backlog bucket — the
**String utilities & formatting** sub-section. All stdlib, no new
dependencies.

### Added

- `api.str.*` (eighteen members): `trim` / `ltrim` / `rtrim` (PHP-style
  custom-mask trimming), `reverse` (rune-aware), `stripHtml`, `nl2br`
  (with optional XHTML mode), `br2nl`, `base64Encode` / `base64Decode`,
  `urlEncode` / `urlDecode` (form-style), `htmlEntityDecode`, `pad` /
  `lpad` / `rpad` (recon-shaped, defaults to a single-space pad on the
  right), `sprintf` / `printf` (Go fmt verbs; printf writes to stdout),
  `normalizeNewlines` ("lf" / "crlf" / "cr").
- `api.path.*`: `dirname`, `basename(p, suffix?)` — POSIX semantics
  (forward slashes).
- `api.time.format(unixMs, fmt, tz?)`: small strftime renderer over a
  curated token set (`%Y %y %m %d %H %M %S %F %T %j %A %a %B %b %z %Z
  %%`). Day/month names are English; unknown `%X` tokens are emitted
  verbatim. Takes Unix milliseconds for symmetry with `api.time.nowMs`.
- Three new test files driving each surface through real Engine + Run:
  `api_str_test.go`, plus a Go-level table test for `strftime`.
- `--examples` walkthrough adds steps 14 ("String utilities") and 15
  ("Paths and time formatting"). `exampleCount` is now 15.

### Changed

- `MANUAL.md` declares the new `str` / `path` shapes and extends the
  `time` shape with `format`. A new prose block calls out the
  recon-style vs JS-native semantic differences (trim mask, urlEncode
  form-style, sprintf verbs, pad sides, strftime token table).

## [0.3.1] — 2026-05-26

Second slice of the **Trivial** backlog bucket — the
**Hashing & compression** sub-section. (Compression is rated Easy
rather than Trivial in OUT-OF-SCOPE.md, so this cut covers hashing
only.)

### Added

- `api.hash.*` script binding: nine algorithms returning lowercase hex
  digests of the UTF-8 input.
  - `md5`, `sha1`, `sha256`, `sha384`, `sha512` (stdlib `crypto/*`)
  - `sha3_256`, `sha3_512` (`golang.org/x/crypto/sha3`)
  - `blake3` — 32-byte output (`lukechampine.com/blake3`,
    pure-Go BLAKE3)
  - `crc32` — IEEE polynomial, zero-padded to 8 hex chars (stdlib
    `hash/crc32`)
- `cmd/sercon/api_hash_test.go`: per-algorithm vectors for the empty
  string and the canonical "abc" input.
- `--examples` walkthrough gains a "Hashing (api.hash.*)" section
  (`exampleCount` is now 13).

### Changed

- `MANUAL.md` declares the new `api.hash` shape in its built-in `api`
  declaration block and explains the UTF-8 / lowercase-hex / crc32-IEEE
  semantics.

### Dependencies

- New direct: `lukechampine.com/blake3 v1.4.1`,
  `golang.org/x/crypto v0.52.0` (pulls in `klauspost/cpuid/v2` for
  blake3's CPU feature detection — still pure Go).

## [0.3.0] — 2026-05-26

First minor focusing on the **Trivial** backlog bucket, starting with
the **Engine** sub-section. This release is two pure-API-surface
additions; no library dependencies were added.

### Added

- `RunOption` + `WithScriptRoot(dir)`: pass per-Run overrides to
  `Engine.Run` / `Engine.RunFile` without rebuilding the engine. The
  override applies only to the current call and rewrites the entry
  script's effective base directory for `require` / `import`
  resolution.
- `Engine.Reset()`: clear every registered binding. Lets a long-lived
  Engine be reused across unrelated script batches that each want a
  clean global namespace. Not safe to call concurrently with Run.

### Changed

- `Engine.Run` / `Engine.RunFile` are now variadic in their option
  list; existing two-/three-arg callers compile unchanged.

## [0.2.4] — 2026-05-26

### Fixed

- `make manual` no longer passes `--toc`. recon's auto-injected TOC
  was being placed above the cover-page `<div>`, pushing the cover
  to page 2. `MANUAL.md` ships its own curated `## Table of contents`
  section, which stays in flow and renders in the correct order.

### Changed

- `OUT-OF-SCOPE.md` gains a top-level **Deferred** bucket alongside
  Trivial / Easy / Moderate / Hard. Items land there with a stated
  reason rather than an effort estimate, ready to re-promote when the
  reason resolves. First occupant: `pdf_export_page` (no trustworthy
  pure-Go PDF renderer).

## [0.2.3] — 2026-05-25

### Added

- `MANUAL.md` now opens with an HTML `<div class="cover">` cover page
  (title, subtitle, version, date, repo & licence) styled by recon's
  bundled CSS, matching the
  [`codedeviate/recon`](https://github.com/codedeviate/recon) manual.

### Changed

- `make manual` now passes `--unsafe-html` (so the cover-page `<div>`
  renders) and `--page-break-on-h1` (so every top-level section starts
  on a fresh PDF page). Updated `--doc-*` metadata to "sercon User
  Manual" with keywords.

## [0.2.2] — 2026-05-25

### Added

- `LICENSE` — MIT, copyright 2026 Thomas Björk.
- Badge row at the top of `README.md`: GitHub repo, license, Go version,
  latest release, pkg.go.dev. Style mirrors the
  [`codedeviate/webrunner`](https://github.com/codedeviate/webrunner)
  badges, with the `crates.io` slot replaced by `pkg.go.dev`. A
  Homebrew tap badge is staged in an HTML comment and will be enabled
  once `sercon` lands in `codedeviate/homebrew-cli`.

### Changed

- `go.mod`: lowered the `go` directive from `1.26.3` to `1.22` to
  match the README's "Go 1.22+" claim and the project's original
  compatibility target. `go mod tidy` also split direct/indirect
  requires into separate blocks.

## [0.2.1] — 2026-05-25

### Added

- `Makefile` with `build`, `release`, `manual`, `test`, `vet`, `clean`
  targets. `make release` builds with `-trimpath -ldflags='-s -w'` for
  a roughly 30% smaller binary (~23 MB → ~16 MB on darwin/arm64).
- `MANUAL.pdf` — PDF rendering of `MANUAL.md` produced by
  `make manual` (uses `recon --md-to-pdf`). Regenerated whenever
  `MANUAL.md` changes.

### Changed

- `CLAUDE.md` lockstep section now lists `MANUAL.pdf` and points at
  `make manual`; the common-commands section uses `make` targets.

## [0.2.0] — 2026-05-25

### Added

- `--help` / `-h`: in-depth, colourised help screen (sections, flag table,
  examples, exit codes, see-also). Colour is auto-disabled when stdout is
  not a TTY and on `NO_COLOR=1`; force on with `FORCE_COLOR=1`.
- `--examples`: colourised walkthrough of every script feature
  (logging, assertions, http, time, env, imports, require, promises,
  error handling, timeouts, goja built-ins, eventloop additions).
- `--version`: prints the engine version (`scriptengine.Version`) plus
  goja / goja_nodejs / esbuild versions read from `runtime/debug`
  build info.
- `MANUAL.md`: long-form reference covering the library API, CLI, the
  built-in `api` surface, goja built-ins, eventloop additions, top-level
  await mechanics, module resolution, error semantics, and limitations.
- `pkg/scriptengine.Version` constant, bumped in lockstep with the git
  tag.

### Changed

- `CLAUDE.md` now documents the lockstep update rule for `MANUAL.md`,
  `--help`, `--examples`, and `CHANGELOG.md`.

## [0.1.0] — 2026-05-25

Initial cut of the embeddable TypeScript script engine. The project went
through an intra-day rename from `tsrun` to `sercon` before being
tagged; the final names below are the ones that ship in 0.1.0.

### Added

- `pkg/scriptengine` library:
  - `Engine`, `Options` (with `DisableConsole`, `Timeout`, `ScriptRoot`), `New`.
  - `Register`, `RegisterNamespace`, `RegisterConstructor` for static
    bindings; `RegisterFactory` / `RegisterNamespaceFactory` for bindings
    that need the per-Run `*goja.Runtime` and `*eventloop.EventLoop`.
  - `Run` / `RunFile` with a fresh runtime per call and a shared
    `require.Registry` compile cache.
  - `PromisifyAsync[T]` helper that schedules resolution back onto the
    event loop and parks a `SetTimeout` sentinel so `loop.Run` waits for
    the async tail.
  - `Promised[T]` marker for d.ts return-type annotation.
  - `WriteTypes` — `.d.ts` emitter with cycle protection
    (visited-set + depth cap) and special cases for `goja.FunctionCall`
    and `goja.Value`.
  - Two-mode esbuild transpile: CJS for required modules, ESM-then-rewrite
    for the entry script to support top-level `await`.
  - TS-aware custom `require` source loader with Node-style extension
    fallback (`.ts`, `.tsx`, `.js`, `.cjs`, `.mjs`, `.json` and
    `index.*` for directories).
  - Interrupt-based timeout (`ErrScriptTimeout`) and `context.Context`
    cancellation, both routed through one watcher goroutine that calls
    `vm.Interrupt` plus `loop.Terminate`.
- `cmd/sercon` CLI with `-timeout`, `-root`, `-emit-dts`, `-v` flags and
  the example `api` namespace (`api.log`, `api.assert.*`, `api.http.*`,
  `api.time.*`, `api.env.get`).
- Example scripts: `smoke.ts`, `async.ts`, `helpers/assert.ts`,
  `helpers/fixtures.ts`, `hang.ts` (timeout demo).
- Test suite (11 cases) covering the 10 spec-required scenarios plus a
  bare-specifier require-resolution sanity check. Golden `.d.ts` fixture
  refreshable via `go test -update`.
- READMEs at the repo root and under `examples/`.
- `CLAUDE.md` describing non-obvious architecture and the project's
  semver + Conventional Commits conventions.

[Unreleased]: about:blank
[0.1.0]: about:blank
