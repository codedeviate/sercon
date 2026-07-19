# sercon — Thematic Capability History

This file is the thematic companion to `CHANGELOG.md`. Where the changelog
gives per-version detail, this document tells the story by subsystem: when
each capability arrived, how it grew, and what shape it has today. The span
covered is v0.1.0 (2026-05-25) through v0.99.1 (2026-07-19).

`OUT-OF-SCOPE.md` tracks open/parked backlog and is not duplicated here.

---

## 1. Engine & language core

### Initial architecture (v0.1.0)

The engine shipped in v0.1.0 as `pkg/scriptengine`. Its core design was set
from the start: `Engine.Run` creates a fresh `goja.Runtime` (via
`goja_nodejs/eventloop`) for every call, while a single `*require.Registry`
is shared across runs as a compiled-bytecode cache. Registrations are
re-applied inside each loop callback so no globals leak between runs.

Two esbuild transpile modes were established at v0.1.0:
- **Required modules** use `FormatCommonJS` — straightforward.
- **Entry scripts** use `FormatESModule`, then `rewriteEntryESMToCJS`
  line-scans the output, converts `import` to `require()`, and wraps the
  body in an `async IIFE`. This lets scripts use top-level `await`, which
  esbuild rejects under CJS.

A TS-aware `require` source loader added `.ts`, `.tsx`, `.js`, `.cjs`,
`.mjs`, `.json` extension fallbacks and `index.*` directory resolution on
top of what `goja_nodejs/require` provides natively.

Interrupt-based timeout (`ErrScriptTimeout`) and `context.Context`
cancellation were both wired through a single watcher goroutine that calls
`vm.Interrupt` + `loop.Terminate`.

### Event-loop lifetime management (v0.1.0, v0.10.0)

`PromisifyAsync[T]` was designed to solve a subtle problem: `loop.Run`
exits when `jobCount` drops to zero, which only counts timers — not
`RunOnLoop` callbacks. Without a sentinel, an async binding loses the race
against the loop. `PromisifyAsync` parks a 24-hour `SetTimeout` while work
is in flight and clears it on resolution.

`Engine.HoldRun(reason)` was added in v0.10.0 as a refcounted variant of
the same pattern for long-lived bindings (servers, listeners) that don't
have a single Promise to hold the loop open. The returned `release` func is
idempotent; the engine drains any leaked holds at Run end.

v0.98.1 closed a subtle gap in both sentinels: the Go-side `loop.SetTimeout`
they use defers its `jobCount` increment into an aux job, while the eventloop
decrements `jobCount` *before* running a timer callback. So in the sequence
`await <setTimeout-backed promise>; await <host binding>` the loop could exit
the instant the awaited timer dropped `jobCount` to zero — before the sentinel
was armed — silently dropping the host call's continuation (the script
"succeeded" with exit 0). `PromisifyAsync` and `HoldRun` now call
`bumpLoopSync`, which schedules a no-op `setImmediate` (the JS-facing timers
increment `jobCount` synchronously, unlike the Go APIs) to bridge that
transient-zero window. Surfaced by the DailyScripts port.

`LoopCallable` (also v0.10.0) wraps a captured `goja.Callable` so it can
be invoked from any goroutine via `.Call(buildArgs)`. On-loop callers use
`.CallOnLoop(vm, args...)` to avoid deadlocking `RunOnLoop`.

### Polyfill and `for await` support (v0.10.0)

esbuild's `__forAwait` lowering and user code that does
`obj[Symbol.asyncIterator] = …` need to agree on the same key. A per-Run
polyfill installs `Symbol.asyncIterator = Symbol.for("@@asyncIterator")`.
The same v0.10.0 cut enabled esbuild's `Supported` flag to lower
`for await (…)` and async generators into forms goja can parse.

### Source-mapped error positions (v0.29.0)

Runtime errors in `.ts` scripts previously reported transpiled-JS
line/column positions. v0.29.0 attached inline source maps so both
synchronous throws and async rejections reference TypeScript line numbers.
The entry-script ESM→CJS rewriter became line-shift-aware. Error strings
now include the full stack (previously async rejections were message-only).

### `.d.ts` generation (v0.1.0 → v0.5.0 → v0.26.0)

`WriteTypes` shipped in v0.1.0 with cycle protection (visited-set + depth
cap `maxTypeDepth = 4`) and special cases for `goja.FunctionCall` (→
`(...args: unknown[])`) and `goja.Value` (→ `unknown`) to prevent runtime
stack overflows on goja's self-referential types.

v0.4.3 added the `AsyncBinding` carrier so async bindings emit `Promise<T>`
instead of `(...args)`. The `PromisifyAsync[T]` return type changed from a
bare callback to `AsyncBinding`. Factory-built registrations were also
taught to surface `Promise<T>` by invoking factories with `(nil, nil)` at
d.ts time.

v0.5.0 added `Engine.SetDocs` / `Engine.SetMemberDocs` for JSDoc strings in
emitted declarations, with a curated doc map in `cmd/sercon/docs.go`.

v0.26.0 replaced the flat doc map with a structured `MemberDoc{Summary,
Params, ReturnType, Returns, Errors, Example}` model. The emitter now emits
real signatures with `@param`/`@returns`. A companion `--emit-reference` /
`Engine.WriteReference` generates a markdown reference spliced into MANUAL.md
§16 (via `make reference`, which `make manual` runs). All eleven namespaces
were fully documented in that single cut.

v0.47.0 made the d.ts emitter honour `MemberDoc.ReturnType` for
`PromisifyAsync` (async) bindings — previously it always wrapped the marker
type (usually `Promise<unknown>`), discarding the documented shape. v0.48.0
extended the same fix to the §16 reference generator, so async bindings with a
documented return type (e.g. `services.webdriver.connect`,
`net.probe.traceroute`, `runtime.secrets.get`) render their rich `Promise<…>`
type in both the d.ts and MANUAL.md.

v0.87.2 made the emitted declaration file itself valid TypeScript. Doc
signatures had accumulated references to six named types (`Request`,
`Response`, `Pane`, `Envelope`, `Message`, `Image`) that were never declared,
`text.printf`/`text.sprintf` emitted a non-array rest parameter, and
`server.icmp.listen` ordered a required parameter after an optional one — 32
`tsc --noEmit` errors in all. The emitter now prepends a type prelude (shapes
lifted from MANUAL §5.6/5.10/5.11) and corrects those signatures, and a CI
`dts` job emits and `tsc`-checks the file on every push so it stays clean.

### Module resolution improvements (v0.4.2, v0.5.28, v0.5.29, v0.5.30)

v0.4.2 added `.js` → `.ts` swap (for `package.json` `main` fields pointing
at compiled output) and `package.json` `source` field preference.

v0.5.28 added `Options.ModuleLoader` — a hook consulted before the
filesystem, letting embedders serve modules from memory, a network source,
or an embedded bundle.

v0.5.29 hardened the entry-script ESM→CJS rewriter against interleaved
comments and irregular whitespace in multi-line imports.

v0.5.30 added `Engine.SetResolveHook` and `Engine.ResetModuleCache` to
support `--watch` module-graph invalidation.

### Registration kinds

Five `Register*` variants were present from v0.1.0: `Register`,
`RegisterNamespace`, `RegisterConstructor`, `RegisterFactory`,
`RegisterNamespaceFactory`. `RegisterConstructor` gained runtime semantics
(running the Go constructor, coercing JS args, exposing methods) in v0.30.0.
`export default <expr>` from the entry script resolves as the value
`Engine.Run` returns (also v0.30.0).

### Per-Run options and reset (v0.3.0)

`Engine.Run` / `Engine.RunFile` became variadic in their options argument,
taking zero or more `RunOption` values. `WithScriptRoot(dir)` overrides the
module-resolution root for a single Run without mutating the engine. A
companion `Engine.Reset()` clears every registered binding, letting a
long-lived engine be reconfigured and reused across unrelated workloads. These
are embedder-facing (library) APIs, not reachable from a `.ts` script.

### `Engine.AbortRun` (v0.35.0)

Cancels the in-flight Run via the engine's interrupt path. Added to wire
the TUI's Ctrl-C handler.

### Resettable run deadline (v0.55.0)

The per-Run timeout watcher (which arms a single timer from `Options.Timeout`
and interrupts the VM on fire) became **resettable**: it now loops over a
per-Run reset channel, re-arming or disarming the timer mid-run. Two exported
methods drive it (same per-Run-handle pattern as `AbortRun`):
`Engine.SetRunTimeout(d)` (d > 0 re-arms the kill deadline to fire d from now;
d ≤ 0 disables it) and `Engine.RunTimeoutRemaining() (time.Duration, bool)`.
The initial `-timeout` deadline is unchanged when a script never adjusts it.
Surfaced to scripts as `runtime.setDeadline`/`clearDeadline`/`getDeadline`
(see §4 `runtime`).

### Stable JSON key order (v0.16.0, v0.20.0, v0.11.2)

`JSON.stringify` of binding results previously shuffled keys run-to-run
because goja derives JS object property order from Go map iteration (which
Go randomises). v0.16.0 converted several bindings to json-tagged structs.
v0.20.0 introduced `scriptengine.Ordered` for bindings whose keys are
dynamic or conditional. v0.11.2 fixed `text.preg` / `text.preg2` results.

---

## 2. CLI

### Flags and initial shell (v0.1.0 → v0.2.0 → v0.4.4)

v0.1.0 shipped the `sercon` CLI with `-timeout`, `-root`, `-emit-dts`, `-v`
flags. v0.2.0 added `--help` / `-h` (colourised, auto-disabled without TTY),
`--examples` (feature walkthrough), and `--version` (engine + dependency
versions from `runtime/debug`).

v0.4.4 added stdin script support (`-` positional), `Options.Verbose`, the
`ErrTranspile` sentinel, and a distinct exit-code matrix: `0` (all pass),
`1` (usage), `2` (transpile error), `3` (timeout/cancel), `4` (JS throw).

v0.48.0 added `--secrets-prefix` (and the `SERCON_SECRETS_PREFIX` env) to set
the `runtime.secrets` keystore namespace prefix (see §4 `runtime`).

v0.56.0 added `--env-file PATH` (repeatable): load `KEY=VALUE` pairs from a
`.env` file into the process environment before the script runs, so
`runtime.env.get` and any spawned subprocess see them. Parses `KEY=VALUE`, `#`
comments, blank lines, an optional leading `export`, and optional surrounding
quotes — no shell expansion. A variable already in the real environment always
wins; among multiple files a later file overrides an earlier one. Replaces the
`set -a; source .env; set +a` ritual and makes shebang scripts self-sufficient.

v0.58.0 added `--doctor`: print a category-grouped report of every optional
external tool sercon can use (git, gh, AI providers, agent-browser,
chromedriver/geckodriver, typst, recon/curl, clipboard/image tools) — installed?,
version, purpose — and validate the **chromedriver↔Chrome major-version match**.
Exits 0 normally (a missing tool is fine — everything is optional enrichment) and
5 on a detected compatibility conflict. Same diagnostics are scriptable via
`services.doctor` (see §4 `services`).

### Help paging (v0.15.0)

`--help` and `--examples` pipe through `$PAGER` (falling back to `less
-FRX`) when stdout is a terminal, like git. `--no-pager` disables it.

### `sercon run` and shebang (v0.13.0)

`sercon run <script> [args...]` runs exactly one script and passes every
token after the script path as `runtime.argv[2:]`. Scripts can begin with
a `#!` shebang line (stripped before transpile, line numbers preserved),
making `#!/usr/bin/env -S sercon run` practical.

v0.99.1 completed the picture for the intended deployment — a `bin/` launcher
**symlinked onto `PATH`**. Previously the entry script's relative
`import`/`require` specifiers resolved against the symlink's directory (its
module id came from `filepath.Abs`, which doesn't follow symlinks), so
`../lib/…` broke unless the launcher was a wrapper passing the real path.
`Engine.RunFile` now `filepath.EvalSymlinks`-resolves the entry path (falling
back to the unresolved path on a broken link), and the CLI's default `-root`
does the same via a shared helper — so a symlinked launcher runs identically to
invoking the real file, and `runtime.argv[1]` is the real path. Surfaced by the
DailyScripts port.

### `--watch` live re-run (v0.5.9)

`--watch` re-runs the entry script on file changes after the initial run. The
watcher walks the script root recursively at startup (picking up newly created
dirs), filters on source extensions (`.ts`/`.tsx`/`.js`/`.jsx`/`.json`),
ignores dot-prefixed paths (`.git`, …), and debounces events over 150 ms before
re-running. It pairs with the engine-side `Engine.SetResolveHook` /
`Engine.ResetModuleCache` (v0.5.30) that invalidate the module graph between
runs (see §1).

### `sercon serve` (v0.10.0, v0.11.0)

`sercon serve script.ts` was introduced alongside the `server` namespace in
v0.10.0. It adds: structured access log to stderr (`ts remote method path
status dur_µs`), `--shutdown-timeout` (default 30s), `--port-override`, and
a `READY listening on tcp/…` line on stdout per listener. SIGTERM exits 0.
SMTP access logging (per-stage AUTH/MAIL/RCPT/DATA/QUIT) was added in
v0.11.0.

### `sercon init` (v0.17.0)

Drops `sercon.d.ts` plus a `jsconfig.json` into a directory so any
TypeScript-language-server editor gives completion and hover docs for the
reserved globals with no plugin. Existing files are left untouched unless
`--force`.

### `--emit-dts` / `--emit-reference` (v0.1.0 / v0.26.0)

`-emit-dts <path>` emits the declaration file for the CLI's reserved-global
surface. `--emit-reference` (v0.26.0) generates the structured markdown
reference spliced into MANUAL.md §16 via `make reference`.

### `console` global (v0.14.0)

A browser/Node-compatible `console` shim was added: `log`/`info`/`debug` →
stdout, `warn`/`error` → stderr. This replaced goja's default console
(which routed everything through Go's logger — timestamped, all on stderr).
`runtime.log` remains the native stdout logger. `console.table` (Node/Bun/Deno
-parity bordered table with a leading `(index)` column, optional `Values`
column, and a `columns` restrict/order argument; hand-rolled Unicode-box
renderer, no new dependency) was added in v0.98.0, surfaced by the DailyScripts
port (~24 call sites).

---

## 3. Surface re-architecture (v0.8.0 → v0.9.0)

v0.8.0 grouped the original flat `api.*` bindings into nine category
buckets: `runtime`, `crypto`, `text`, `format`, `fs`, `net`, `db`, `tools`,
`ui`. Two leaf renames: `api.net.*` probes → `api.net.probe.*`;
`api.text.*` charset → `api.text.charset.*`.

v0.9.0 (BREAKING) removed the `api.` wrapper entirely and promoted the
nine buckets to top-level globals. Three additional renames:
`tools` → `services`, `format` → `codec`, `ui.tui` → `tui`. The `Sercon`
global was removed; `Sercon.argv` became `runtime.argv`.

v0.13.1 cleaned up remaining `api_` file-name prefixes and renamed the
bundled declaration file from `api.d.ts` to `sercon.d.ts`.

---

## 4. Reserved script globals

### `runtime`

Present from v0.1.0 as `api.log`, `api.assert.*`, `api.time.*`,
`api.env.get`. Promoted to `runtime.*` in v0.9.0.

String utilities (`str.*`, 18 members: trim/pad/base64/html/sprintf/…),
`path.dirname`/`basename`, and `time.format` arrived as the Trivial bucket
was worked in the v0.3.x range. `runtime.assert.equal` was fixed to perform
deep structural comparison (previously reference equality) in v0.19.0.
`console.*` rendering was fixed to JSON-format objects in v0.19.0.
`runtime.argv` follows the Node/Bun layout `[programName, scriptPath,
...userArgs]`, available from v0.6.0.

`runtime.secrets` (v0.48.0) reads/writes string credentials in the OS keystore
(macOS Keychain, Linux Secret Service / libsecret, Windows Credential Manager),
pure-Go via `zalando/go-keyring` (no cgo): `available` (advisory bool),
`get(name, account)`, `set(name, account, secret)`, `delete(name, account)`.
All operations are confined to a prefix namespace (keystore service =
`PREFIX + name`), where `PREFIX` comes from `--secrets-prefix` >
`SERCON_SECRETS_PREFIX` > the default `sercon/`. v0.48.1 added argument
validation so a missing/empty `name`, `account`, or `secret` rejects with a
clear error instead of silently keying `"<prefix>undefined"`.

`runtime.clipboard` (v0.51.0) reads/writes the host OS system clipboard text:
`available` (advisory bool), `read() → Promise<string>` (`""` when empty),
`write(text) → Promise<void>`. It is the first sanctioned *host* clipboard
binding and an **external-CLI fallback** — no pure-Go, no-cgo library covers
every platform — feature-detected on PATH per the `services.git`/`gh`/`ai`
precedent: macOS `pbcopy`/`pbpaste`, Linux `wl-clipboard` (Wayland) or
`xclip`/`xsel` (X11), Windows `clip` + PowerShell `Get-Clipboard`. The backend
is resolved by a pure `clipboardBackend(goos, wayland, look)` function (so it is
unit-tested without touching the environment); writes feed text via stdin and
every subprocess is bounded by a ~5s timeout. When no backend is installed the
ops throw a clean error and `available` is `false`; the static binary stays
fully functional.

PNG **image** support followed in v0.52.0: `imageAvailable` (advisory),
`readImage() → Promise<Uint8Array | null>` (`null` when the clipboard holds no
image), `writeImage(png) → Promise<void>` (PNG-validated on write). Backends:
macOS reads via `pngpaste` (not built-in) and writes via `osascript`+a temp
file; Linux uses `wl-copy`/`wl-paste` or `xclip -t image/png` (not `xsel`, which
has no image support); Windows uses PowerShell. v0.52.1 fixed the example's
degenerate 1×1 sample (macOS CoreGraphics can't finalize a PNG from it). v0.52.2
fixed a Linux write hang: `xclip`/`wl-copy` fork a daemon to own the selection,
and the inherited captured-stderr pipe blocked `cmd.Wait()` forever — clipboard
writes now route the child's std streams to `os.DevNull` (an `*os.File`, so
`os/exec` spawns no copier goroutine). Verified end-to-end across macOS, Linux
X11 (Xvfb), and Linux Wayland (headless sway) for both text and PNG.

`runtime.setDeadline`/`clearDeadline`/`getDeadline` (v0.55.0) let a script
control its own wall-clock kill timeout — the same deadline the `-timeout` flag
sets — at runtime: `setDeadline(ms)` moves the deadline to now + ms (extend /
keep-alive; `ms <= 0` disables), `clearDeadline()` removes it, `getDeadline()`
returns the ms remaining or `null`. So a script can lengthen a long task, or
even add a deadline to a `-timeout 0` run, or disable its timeout entirely from
within. Named `setDeadline` (not `setTimeout`) to avoid conflation with the JS
global event-loop timer. Backed by the resettable run watcher + `SetRunTimeout`
in §1.

### `crypto`

**Hash family** (`crypto.hash.*`, nine algorithms: md5/sha1/sha256/sha384/
sha512/sha3_256/sha3_512/blake3/crc32): shipped in v0.3.1 as `api.hash.*`.

**JWT** (`crypto.jwt.*`): shipped across v0.5.2–v0.5.3. v0.5.2 added
`sign`/`view`/`validate` for HMAC algorithms (HS256/384/512); v0.5.3 added
the full asymmetric matrix (RSA RS256/384/512, RSA-PSS PS256/384/512,
ECDSA ES256/384/512, Ed25519/EdDSA) with bidirectional PEM cross-checks and
an algorithm-confusion guard (`WithValidMethods`). v0.5.15 added JWK
(`{"kty":…}`) as a third key-shape alongside raw bytes and PEM.

**Encrypt** (`crypto.encrypt.*`): shipped across v0.5.5–v0.5.8 + v0.5.16.
v0.5.5 added age X25519 `keygen`/`encrypt`/`decrypt`. v0.5.6 added
`opts.armored` and auto-detect for armored input. v0.5.7 added `rekey`
(re-encrypt without exposing plaintext to JS). v0.5.8 added
`detectBackend` (age vs PGP classifier). v0.5.16 added the PGP backend
(`keygenPgp`, PGP-routed `encrypt`/`decrypt` via ProtonMail go-crypto)
completing the two-backend auto-dispatch design.

### `text`

**`text.str.*`** (18 string utility functions): landed in the Trivial bucket
(v0.3.x range, as `api.str.*`).

**`text.charset.*`** (detect/encode/decode): arrived as v0.4.12 (`api.text.*`),
backed by `saintfish/chardet` and `golang.org/x/text/encoding/htmlindex`.

**`text.diff`** (`diff.compare`): v0.4.15, backed by `pmezard/go-difflib`.
Returns `{ identical, binary, added, removed, diff, format }` with a
unified-diff body.

**`text.jq`** (`jq.query`/`queryAll`): v0.4.16, backed by `itchyny/gojq`
(full jq filter syntax).

**`text.preg`** (`preg.match`/`matchAll`/`replace`): v0.5.1, PHP-style
`/pattern/flags` syntax over Go's RE2 engine.

**`text.preg2`** (`preg2.match`/`matchAll`/`replace`): v0.5.13, PCRE-flavoured
sibling backed by `dlclark/regexp2`, supporting lookahead, lookbehind, and
backreferences that RE2 cannot do.

**`text.markdown.toHtml`** (Markdown → HTML): v0.98.0, backed by pure-Go
`yuin/goldmark` (CommonMark; GFM extensions — tables/strikethrough/task
lists/autolinks — on by default, plus a `hardBreaks` option). String in,
string out — complements the existing Markdown → **PDF** path
(`recon --md-to-pdf`). Surfaced by the DailyScripts port (its `md`/`md2html`
helper); the new dependency was maintainer-authorized.

### `codec`

**`codec.compression.*`** (nine algorithms: gzip/deflate/zlib/bzip2/zstd/
brotli/lz4/xz/snappy): v0.4.10, all pure-Go.

**`codec.barcode.*`** (encode ten symbologies → PNG): v0.4.11 (`api.barcode.encode`).
`barcode.decode` (auto-detect from PNG/JPEG/WebP via `makiuchi-d/gozxing`):
v0.5.4. `opts.quietZone` on encode (spec-required clear zone for EAN/UPC
round-trips): v0.5.14.

**`codec.checkdigit.*`** (algos/validate/compute/inspect; Luhn, ISBN-10,
ISBN-13, EAN-13, EAN-8, UPC-A): v0.4.13.

**`codec.php.*`** / **`codec.perl.*`** (serialize/unserialize, varExport/Dump
round-trips): v0.12.0, shared dump IR with stable key order and
cycle detection.

**`codec.xml.*`** (`encode`/`decode` via `@`-prefix attributes and `#text`
convention): v0.32.0.

**`codec.yaml.*`** (`parse`/`stringify`): v0.98.0, the `JSON`-shaped pair
matching `codec.toml`, backed by the pure-Go `yaml.v3` (promoted from an
indirect to a direct dependency). First document of a `---` stream only;
non-string mapping keys are coerced to their string form so goja can expose
the mapping as an object. Surfaced by the DailyScripts port (native config
loading in place of a JSON shim).

### `fs`

**`fs.path.*`** (`dirname`, `basename`): shipped in the Trivial bucket (v0.3.x
range).

**`fs.archive.*`** (`create`/`extract` for zip/tar/tar.gz with zip-slip
protection): v0.4.14 (`api.archive.*`), stdlib-only.

- **File read/write (v0.62.0)** — `fs` gained general file primitives:
  `writeText`/`writeBytes`/`readText`/`readBytes`/`mkdir`/`exists`/`remove`/`stat`
  (all async). Previously `fs` only did path math + archives, so scripts could
  capture screenshots but not write the report document that stitches them
  together; this closes that gap. Writes are Node-like (fail on a missing parent;
  `mkdir` is `mkdir -p`); no path sandboxing, matching the existing
  `image.save`/`screenshot` writers. Shipped with `fs-report.ts` — an illustrated
  per-step screenshot report.

- **Fast search — `fs.find` + `fs.grep` family (v0.99.0)** — native, pure-Go
  file search (fd-like) and content search (rg-like), so scripts stop shelling
  out to `rg`/`fd`. `fs.find(opts?)` walks with parallel traversal
  (`charlievieth/fastwalk`) and **`.gitignore`-aware directory pruning**
  (`monochromegane/go-gitignore`) — the property that makes `rg`/`fd` fast is
  not descending `node_modules`/`.git` at all — plus `**` globs
  (`bmatcuk/doublestar`), regex/type/extension filters, smart-case, and a
  `stat: true` shape. `fs.grep(opts)` matches file contents with a literal
  `bytes.Index` fast-path or RE2, with context lines, word/multiline
  (whole-file), invert, per-file/total caps, and binary skipping; `fs.grepFiles`
  (rg `-l`) and `fs.grepCount` (rg `-c`) are the count/list shortcuts. Every
  binding returns a `Promise<T[]>` by default or an **async iterator** with
  `stream: true` (mirrors the `server_ws` iterator: `HoldRun` + a cancellable
  producer goroutine), and roots on the Run context for prompt cancel/timeout.
  Built subagent-driven; the review loop caught a `fastwalk` concurrency race
  (fn is mutex-serialized), an ignoreCase-literal byte-offset bug, and a
  `type:`-preset filter that returned zero results. v1 reads `.gitignore` +
  `.ignore` only (no cross-file negation). Three new authorized pure-Go deps;
  stays cgo-free. Examples: `fs-find.ts`, `fs-grep.ts`.

### `net`

**`net.http.*`** (`get`/`post`): present from v0.1.0 (as `api.http.*`).
`net.http.request(method, url, opts?)` with per-call headers/body/timeout/
retry/redirect control: v0.5.17. v0.56.0 added `bodyBytes` (a `Uint8Array` of
the raw, undecoded response bytes) alongside the UTF-8 `body` on all three —
so a script can recover non-UTF-8 content via `text.charset.decode(r.bodyBytes,
"ISO-8859-1")` instead of losing bytes to U+FFFD.

**`net.probe.*`** (TCP connect, DNS, TLS cert inspection): v0.4.6. NTP
(beevik/ntp) and WHOIS (likexian/whois): v0.4.7. Ping (TCP mode default,
ICMP opt-in via prometheus-community/pro-bing): v0.5.18. SMTP capability
probe (EHLO + advertised extensions): v0.5.19. WebSocket handshake probe
(`wss`): v0.5.20. Aggregate `net.netstatus.check` (fan-out of DNS/TCP/TLS/
HTTP in parallel): v0.5.21. `net.probe.traceroute` (TTL-stepped ICMP/UDP/TCP
path trace): v0.33.0. `net.probe.ping` UDP mode (ICMP port-unreachable proof
of reachability): v0.33.0.

**`net.browser`** (stateful HTTP session with cookie jar and default headers):
v0.5.22.

**`net.email.*`** (SPF, DMARC): v0.4.8; (MTA-STS, TLS-RPT, BIMI, `email.all`
aggregator): v0.4.9. Outbound `net.email.send` with MIME composition,
STARTTLS/TLS/none modes, and per-recipient outcome: v0.11.0.

**`net.tcp`** / **`net.udp`** / **`net.icmp`** client sockets: v0.22.0. Long-lived,
bidirectional, push/callback read model. `net.icmp.send` gained raw
non-Echo body mode in v0.27.0.

**`net.capture`** (packet capture via pure-Go gopacket; live on Linux/macOS,
pcap file read/write, no libpcap): v0.24.0. Tcpdump-syntax filter option:
v0.25.0. Filter grammar extended in v0.42.0 with `net X/Y` (IPv4/IPv6 CIDR,
optional `src`/`dst`) and `portrange A-B` (inclusive, optional `src`/`dst`),
composing with `and`/`or`/`not` + parens across `capture.open`,
`capture.openFile`, and `net.raw.open` — still a post-decode userspace
predicate, not kernel BPF. `net.capture.routes()` (v0.45.0) is a synchronous,
unprivileged snapshot of the host IP routing table (`{ destination, gateway,
interface, family, metric }`) beside `net.capture.interfaces()`: Linux parses
`/proc/net/route` + `/proc/net/ipv6_route`, macOS/BSD read the routing socket
via `x/net/route`, Windows is stubbed. Deeper decode (v0.50.0): the decoded packet object gained `arp`
(operation + sender/target MAC & IP), `vlan` (802.1Q id/priority/drop/inner
type), and `dns` (id/qr/opcode/rcode + `questions[]`/`answers[]` with
type-aware `data`) layers, plus `tcp.window`/`tcp.checksum` and a parsed
`tcp.options` object (mss/windowScale/sackPermitted/timestamps). All additive
— keys appear only when that layer decodes — and shared by the `net.raw`
handlers.

**`net.raw.*`** (`net.raw.open` full raw IPv4 packet engine with tcpdump
filter; `net.raw.tcp` one-shot raw TCP probe): v0.34.0. Root/CAP_NET_RAW
required; Linux + macOS only.

**`net.load.http`** (v0.52.0): an authorized HTTP load / resilience self-test
harness. A worker pool over a shared `*http.Client` drives a target at a given
`concurrency` for a `requests` count or `duration` (optional `rps` cap),
returning a report — `sent`/`completed`/`failed`, achieved `rps`, `errorRate`,
latency `min/mean/p50/p90/p95/p99/max` (pure nearest-rank `percentiles`),
`statusCounts`, `errors`. It is **defensive-only**: a dual-use guardrail refuses
public targets unless `confirm:true` (loopback/private always allowed, via a
pure `classifyTarget`), concurrency is capped at 1000, and there are no
amplification/spoofing/evasion helpers — just a plain HTTP client loop. TCP/UDP
flood generators and ramp/soak/burst stage profiles are deliberately out of
scope (see `OUT-OF-SCOPE.md`).

### `db`

**`db.sqlite`** (pure-Go `modernc.org/sqlite`): v0.5.10. Transactions
(`handle.begin()`): v0.5.11. Prepared statements (`handle.prepare()`):
v0.5.12.

**`db.redis`** (redis/go-redis, `do` for arbitrary RESP commands): v0.5.23.

**`db.valkey`** (v0.49.0): a client for Valkey, the RESP-compatible Redis fork.
Same `open(url) → { do, ping, close }` surface as `db.redis` (reuses the same
pure-Go go-redis client) and additionally accepts the `valkey://` / `valkeys://`
URL schemes (normalised to `redis://` / `rediss://`).

**`db.memcached`** (bradfitz/gomemcache, get/set/delete): v0.5.24.

**`db.ldap`** (go-ldap/v3, rootDSE + subtree search): v0.5.25.

**`db.postgres`** / **`db.mysql`** / **`db.mssql`** (pure-Go drivers, shared
`database/sql` handle with exec/query/queryValue/begin/prepare/close):
v0.18.0.

**`db.clickhouse`** (clickhouse-go v2) and **`db.oracle`** (go-ora, no cgo):
v0.21.0.

**`db.dict`** (RFC 2229 DICT protocol, hand-rolled over `net/textproto`):
v0.5.26 (as `api.dict`).

**Integration tests** (dbplayground): opt-in, env-gated Go tests
(`SERCON_TEST_*`) exercise the networked engines against the
`github.com/codedeviate/dbplayground` Docker fleet; `make test-integration`
manages the fleet lifecycle. A dedicated CI workflow
(`.github/workflows/integration.yml`) runs `make test-integration` on push / PR
to master (and on demand via `workflow_dispatch`), separate from the `ci.yml`
build/test matrix so the Docker fleet cycle doesn't slow normal runs.
Unversioned dev tooling — skips without the fleet, so default `go test ./...`
stays green.

All multi-engine SQL handles share the same interface: `exec`/`query`/
`queryValue`/`begin`/`prepare`/`close`; placeholder syntax is driver-native.

### `services`

**`services.exec.shell`** (subprocess runner, `/bin/sh -c` or argv mode,
non-zero exits resolve as data): v0.4.17. Group-kill on timeout/cancel:
v0.6.0. `services.exec.stream` (line-by-line streaming with `onLine`
callback): v0.28.0. `services.exec.shell` gained `{ pty: true }` for
pseudo-terminal (color/progress) support on Unix: v0.35.0.
`services.exec.interactive` (child wired to sercon's own terminal for
`docker -it` / `ssh` / interactive REPLs — Unix pty + raw mode + `SIGWINCH`,
restored on every exit path; non-TTY and Windows inherit the raw handles):
v0.98.0, surfaced by the DailyScripts port (its `inherit`-stdio `sysCall`).

**`services.exec.http`** (recon-with-curl-fallback HTTP client): v0.4.18.

**`services.git.*`** (branch/status/add/commit/log/diffStat/runText, all
accepting `opts.cwd`): v0.4.19.

**`services.gh.*`** (authStatus/prList/repoView): v0.4.20.

**`services.ai`** (`providers`/`send` via claude/codex/copilot/gemini CLIs
on PATH): v0.5.27.

**`services.agentBrowser`**: see §6 below.

**`services.webdriver`**: see §6 below.

**`services.typst`** (v0.54.0): an external-CLI binding to the Typst
typesetting compiler — the `typst-as-lib` Rust embedding path can't be linked
under no-cgo, so this shells out to the official `typst` CLI (feature-detected
via `available`, like git/gh/ai). `version()`, `fonts()`, `compile(opts)` —
inline `source` or a `.typ` `input` → PDF bytes, or write PDF/PNG/SVG to an
`output` path (`root`/`inputs`/`ppi`/`fontPaths`) — and `query(opts)`
(selector → JSON). PDF-bytes return is the no-output default; PNG/SVG require an
output path. Verified end-to-end against typst 0.15.0.

**`services.doctor(requires?)`** (v0.58.0): external-requirements diagnostics —
a shared check registry (used by both the binding and the `--doctor` flag, §2)
probes every optional external tool concurrently and returns
`{ ok, satisfied, unmet, tools }`. Each `tools[]` entry carries `installed`,
`version`, `purpose`, `ok`, and an optional `detail`; the only compatibility
check is **chromedriver↔Chrome major** (a mismatch sets `ok:false` and flips the
report `ok`). A missing tool is *not* a failure (everything is optional).
`requires` is an array of **feature names** (mirroring the `*.available` gates —
`webdriver`/`typst`/`ai`/`git`/`clipboard`/…) **or** specific binaries; a feature
is met when ≥1 backing binary is installed and conflict-free, `unmet` lists the
rest, and an unknown requirement throws. Turns doctor from a readout into an
assertable prerequisite check a script can self-skip on.

### `tui`

Multi-pane terminal UI backed by tview/tcell. Shipped in v0.7.0 as
`api.tui.*`, promoted to a top-level global in v0.9.0.

Scripts declare a recursive split-tree layout (`tui.layout`), obtain pane
handles with `write`/`writeln`/`clear`/`title`, and route subprocess output
via `exec.shell(cmd, { pane })`. ANSI colors pass through; non-TTY falls
back to prefixed plain-text lines.

v0.11.1 fixed a double-init bug that left the terminal in raw mode after
the second TUI run.

v0.35.0 added: `{ pty: true }` shell exec rendering into panes; per-pane
`wrap` (`"char"` | `"word"` | `"off"`) and `color` (bool) options; pane
autoscroll with opt-out; `tui.layout({ mouse: true })` for mouse-wheel
scrolling; `tui.waitKey()` / `tui.onKey(handler)`. Ctrl-C now aborts on a
single press. Terminal detection was fixed to use `term.IsTerminal` (an
`os.ModeCharDevice` check misclassified `/dev/null` as interactive).

v0.35.1 added left-click pane focus and fixed scroll-under-cursor
(previously changed focus while scrolling).

### `image` (v0.53.0)

The eleventh top-level global — image I/O and manipulation, all pure-Go (no
cgo). Decode PNG/JPEG/GIF/TIFF/BMP/WebP (stdlib `image/*` + `golang.org/x/image`)
and rasterize an SVG subset (`srwiley/oksvg`+`rasterx`); a chainable,
**synchronous**, immutable `Image` handle (each transform returns a fresh
handle) backed by `disintegration/imaging`: `resize` (a `0` dimension preserves
aspect), `fit`/`thumbnail`/`crop`/`rotate`(+90/180/270)/`flipH`/`flipV`,
`brightness`/`contrast`/`gamma`/`saturation`/`sharpen`/`blur`/`grayscale`/
`invert`, `overlay`/`paste`; encode via `bytes(format)` / `save(path)` to
PNG/JPEG/GIF/TIFF/BMP/WebP (WebP encode via `HugoSmits86/nativewebp`, lossless).
GIF decode is first-frame; SVG is rasterize-in only. v0.53.0 also pinned
`make manual` to `--pdf-engine chrome` (recon 0.101.0 made typst its default
engine, which rejects the `--unsafe-html` the manual needs). v2 (parked):
animated GIF/APNG, EXIF/orientation, custom convolution/filters, SVG output.

### `web` (v0.66.0)

The twelfth top-level global — fetch & parse web documents, all pure-Go (no
cgo). It closes the HTML-scraping gap that `codec.xml` left open: `codec.xml`
handles strict, well-formed XML, but real-world pages are tag soup and feeds
arrive in three rival formats. `web` answers with three families, each pairing
a synchronous `parse(string)` with an async `load(url, opts?)` that GETs first:
**`web.feed`** parses RSS/Atom/JSON feeds (`mmcdole/gofeed`) into a single
normalized model — the format quirks (`pubDate`/`updated`, `description`/
`summary`) unified, with a per-item `raw` escape hatch carrying enclosures and
namespaced (`media:*`/`dc:*`) extras; **`web.sitemap`** parses urlset /
sitemapindex documents with transparent `.xml.gz` decompression and a bounded
`{expand:true}` index recursion that merges child URLs (per-child failures
collected, not thrown); **`web.html`** parses leniently (`golang.org/x/net/html`)
into a chainable `Node` queryable by CSS (`andybalholm/cascadia` — `find`/
`findAll`) or XPath (`antchfx/htmlquery` — `xpath`/`xpathAll`), with accessors
`text`/`html`/`innerHTML`/`tag`/`attr`/`attrs`. Every `load` reuses the
`net.http` option surface, sends a default `sercon-web/<version>` User-Agent,
and throws on non-2xx.

---

## 5. Servers (`server.*`)

### Foundation (v0.10.0)

The `server` namespace and the `sercon serve` subcommand landed together in
v0.10.0. The engine additions that make long-lived servers possible —
`LoopCallable` (off-loop callable dispatch) and `Engine.HoldRun` (refcounted
sentinel to keep the event loop alive) — are covered in §1.

### HTTP / HTTPS (v0.10.0)

`server.http` / `server.https`: stdlib `http.ServeMux` Go 1.22+ pattern
routing, onion-style middleware, static-file mount helper (`server.http.static`
wraps `http.FileServer` + `http.StripPrefix`), and WebSocket upgrade
(`res.upgradeWebSocket`) via async iterator backed by coder/websocket. The
per-connection goroutine pumps frames via a buffered channel; the JS-side
async iterator resolves frames on the loop via `LoopCallable`.

v0.43.0 added `cert: "self-signed"` to `server.https.listen`: an ephemeral
in-process P-256 cert (no openssl, no committed PEM, nothing written to disk;
`key` then optional), SANs covering `localhost` / `127.0.0.1` / `::1` plus the
listen host — a local-dev convenience, not a production path.

v0.44.0 added an optional `onError(err, req, res)` handler to
`server.http.listen` / `server.https.listen`, invoked when a route handler or
middleware throws/rejects in place of the stock 500. It may be `async` and
renders via the usual `res.*` terminals; if it settles without finalizing, or
itself throws, sercon falls back to the stock 500, so a buggy error handler
can't wedge the request. `err` carries the original thrown value.

### Server-Sent Events (v0.50.0)

`res.sse(opts?)` added a one-way `text/event-stream` streaming mode to the
HTTP/HTTPS listener, alongside the existing WebSocket upgrade. Unlike
`res.upgradeWebSocket` (which hijacks the connection), SSE keeps writing to the
same `http.ResponseWriter`, so the request dispatcher parks on a new
`streamDone` channel until the stream closes — otherwise net/http would finish
the response. A dedicated pump goroutine owns the writer (the single-threaded
loop never writes to the socket); `send()` formats frames on the loop and the
pump writes + flushes + acks. The handle exposes `send(data)` (a string, or
`{event, data, id, retry}` with object data JSON-encoded), `close()`, and a
`closed` Promise that resolves on close or client disconnect; `opts` carry
`keepAlive` (periodic `: ping` comments) and `retry`. The loop is kept alive
via `Engine.HoldRun`, the same as WebSocket.

### SMTP inbound + email send (v0.11.0)

`server.smtp.listen` (go-smtp backend): per-stage JS callbacks
(`onMail`/`onRcpt`/`onData`), optional SASL AUTH (PLAIN + LOGIN), STARTTLS,
enmime message parsing into `{from, to, subject, headers, body.text/html,
attachments, raw}`.

`net.email.send`: outbound SMTP with in-tree MIME composition, three TLS
modes, per-recipient outcome capture.

### Raw TCP / UDP (v0.22.0, v0.23.0)

Client sockets (`net.tcp.connect`, `net.udp.open`) shipped in v0.22.0.
Server-side counterparts (`server.tcp.listen`, `server.udp.listen`) in
v0.23.0 — the connection handle shape is identical on both sides. Both
bind synchronously, accept `port: 0`, emit READY lines under `sercon serve`,
and participate in graceful shutdown.

### ICMP listener (v0.31.0)

`server.icmp.listen` receives all host ICMP traffic (root/CAP_NET_RAW) and
`reply(opts?)` sends back using the same `net.icmp` send options. Synchronous
bind, `{ address, close() }` handle.

---

## 6. Browser automation

### `services.agentBrowser` (v0.36.0 – v0.39.0)

A bridge to the `agent-browser` headless-Chrome CLI; no embedded browser —
the external binary must be on PATH.

- **Phase 1 (v0.36.0):** `available`/`version`; `launch(opts?)` handle with
  best-effort close on Run end; navigation (open/back/forward/reload/wait/
  connect); interaction (click/fill/type/press/check/select/scroll/drag/
  upload/download, keyboard/mouse); inspection (get/is*/eval/snapshot/
  console/errors/highlight); locators (find one-shot + locator handle).
- **Phase 2 (v0.37.0):** capture (`screenshot`/`pdf`); `set.*` settings
  (viewport/device/geo/offline/headers/credentials/media); `record.*` video;
  `defaultOptions` bag; flat one-shot shortcuts.
- **Phase 3 (v0.38.0):** network interception/monitoring
  (`network.route`/`unroute`/`requests`/`request`/`har`); cookies; web
  storage; tab management; page diffing. Per-call subprocess timeout
  (default 30 000 ms).
- **Phase 4 (v0.39.0):** debug/perf (`trace`/`profiler`/`inspect`/
  `clipboard`/`vitals`/`pushstate`); React DevTools; live streaming; AI
  `chat`; escape hatch (`cmd(command, ...args)` / `batch(cmds, { bail })`);
  auth vault (`auth.save`/`list`/`show`/`delete`, `auth.login`).
- **Frames (v0.57.0):** `frame(target)` switches the session's frame context to
  an iframe (CSS selector or `@ref`) or back with `"main"`, a CDP frame switch
  so subsequent click/fill/get/snapshot act inside it. **Single-level only** —
  `agent-browser` resolves the selector against the main document and can't
  descend into nested frames; use `services.webdriver` (`frameChain`) for nested
  / cross-origin cases like Klarna Checkout.

### `services.webdriver` (v0.40.0 – v0.41.0)

A W3C WebDriver client backed by `github.com/tebeka/selenium` (pure Go).
No browser bundled; drivers must be installed separately.

- **v0.40.0 (Phase 1):** `available`/`probe({url})` feature detection;
  `connect(opts?)` attaches to a running driver URL or starts an installed
  `chromedriver`/`geckodriver`; stateful element handles from `find`/`findAll`;
  navigation, page source/screenshot, `executeScript`, cookies, waits.
  Sessions quit on Run end.
- **v0.40.1:** Fixed Chrome url-base mismatch (`/wd/hub` prefix); fixed
  `webElement.getAttribute` returning null for absent attributes.
- **v0.41.0 (Phase 2):** Window/tab handles (`windowHandles`/`currentWindow`/
  `switchToWindow`/`newWindow`/`closeWindow`); frame switching; alert
  handling (`acceptAlert`/`dismissAlert`/`alertText`/`sendAlertText`);
  window rect (`maximize`/`minimize`/`fullscreen`/`setWindowRect`/
  `getWindowRect`); real W3C action chains (`hover`/`dragAndDrop`/`keyChord`/
  `performActions`/`releaseActions`); `executeScript`/`executeScriptAsync`
  returning element handles. Built on an internal raw-W3C-command primitive
  because tebeka's legacy mouse API is rejected by modern W3C drivers.
- **v0.46.0:** `connect` gained `commandTimeout` (ms, default 30000) — a
  per-request deadline on the low-level W3C command client. Each raw command
  now carries a context with that deadline, and `quit()` / Run-end cleanup
  cancels any in-flight command, so a driver blocked behind an open alert or
  an unreachable endpoint fails promptly instead of wedging the call.
- **Nested frames (v0.57.0):** `switchToFrame` now also accepts a **CSS
  selector** (find-then-switch), alongside the frame index and element-handle
  forms; `frameChain([t1, t2, …])` switches from the top document through each
  level in one call (queries are frame-scoped after a switch). Built on the W3C
  `/frame` protocol, so **nested cross-origin** frames work (the Klarna Checkout
  inner-iframe case). This release also **fixed** the element/selector switch:
  the old `/frame` body carried only the W3C element key, which chromedriver
  accepts (2xx) but silently ignores — so element-handle switching had never
  actually switched (only the index form worked); the element/selector paths now
  switch via tebeka's `SwitchFrame` / a dual-key web-element reference.
- **Reliable waits / clicks (v0.59.0):** `clickWhenReady(by, value, opts?)`
  polls `find` **in the active frame** until the element is present and (by
  default) visible + enabled, then issues a **native trusted click** (the W3C
  Element-Click endpoint, which fires React handlers); `waitFor` gained an
  `enabled` option (wait past a disabled→enabled flip). Together with
  `switchToFrame`/`frameChain` this reliably drives buttons that render or
  enable asynchronously inside nested cross-origin iframes (e.g. a Klarna
  Checkout "Pay order"). Investigating feedback 0004 confirmed via cross-origin
  nested fixtures that `find` already follows the active frame (native `find`
  and `executeScript` agreed across round-trips / `frameChain` / in-frame
  navigation) — the reported "find doesn't follow the chain" was a timing race,
  not a frame-context bug.
- **Trusted CDP clicks (v0.60.0)** — `cdpClick(by, value, opts?)` activates a
  button inside a **same-process** cross-origin iframe that the W3C Element Click
  hit-tests to the parent `<iframe>` and intercepts: it locates the element
  across the pierced frame tree (`DOM.getDocument{pierce}` + `querySelectorAll` /
  `performSearch`), reads its true viewport coordinates with
  `DOM.getContentQuads`, scrolls it into view, and dispatches a trusted
  `Input.dispatchMouseEvent` over chromedriver's page-session CDP passthrough.
  Raw `cdp(command, params?)` escape hatch alongside. Chrome-only. (Closed the
  same-process half of 0004; the genuine out-of-process case turned out to need
  v0.61.0 — the v0.60.0 fixture was a same-*site* two-port iframe, i.e. the same
  renderer process, so it never exercised a real OOPIF.)
- **Cross-OOPIF clicks + browser-level CDP (v0.61.0)** — completes the Klarna
  case. `cdpClick` now reaches buttons inside **true out-of-process iframes**: a
  cross-*site* iframe (different eTLD+1) runs in its own renderer process, which
  chromedriver's page-session CDP passthrough cannot touch. sercon opens a
  **browser-level CDP WebSocket** (dialed from chromedriver's `debuggerAddress`
  via `/json/version`, the way Puppeteer/Playwright drive input), attaches to the
  page **and every iframe target**, locates the element in its **owning session**
  (coordinates local to that frame's widget — no offset math), and dispatches the
  trusted `Input.dispatchMouseEvent` there, where browser-level input *is*
  hit-tested across OOPIF boundaries. Targets are **re-enumerated every poll**, so
  an iframe injected by JS seconds after load (the real checkout pattern) is still
  found. New scriptable target API: `targets()` lists the browser's CDP targets
  and `attach(target)` → `{ targetId, sessionId, cdp(method, params?), detach() }`
  drives a specific target (e.g. an OOPIF) directly. Falls back to the v0.60.0
  page-session path when browser-level CDP is unavailable (e.g. a remote Grid not
  advertising `se:cdp`). Chrome-only. (Closes 0005 and the real 0004; a full
  Klarna KCO test order was confirmed end-to-end on v0.61.0.)

---

## 7. Release engineering

### Early flow (v0.1.0 – v0.4.4)

The project had no CI or release automation initially. `make release`
(slim flags: `-trimpath -ldflags='-s -w'`) was the local build target.

### Goreleaser + CI (v0.4.5)

v0.4.5 added `.github/workflows/ci.yml` (Go matrix: two versions ×
ubuntu+macOS, with vet/test/lint) and `.github/workflows/release.yml`
(goreleaser on `v*.*.*` tag push). `.goreleaser.yml` cross-compiles
darwin-{amd64,arm64} / linux-{amd64,arm64} / windows-amd64 and bundles
LICENSE/README/CHANGELOG/MANUAL.md/MANUAL.pdf in each archive.
`make release-prep VERSION=x.y.z` + `make version-check` were added for
manual release preparation.

### Release-please era (v0.4.21)

v0.4.21 wired `release-please` to automate PR-and-tag from Conventional
Commits, with version markers in `version.go` and MANUAL.md updated via
`x-release-please-version` comments.

### Return to manual flow (v0.7.0)

release-please was dropped in v0.7.0 because its state desynced after a
manual v0.6.0 tag was cut during an unrelated Actions permission issue. The
`release-please.yml`, `CHANGELOG-AUTO.md`, and config files were removed.
`make release-prep` (with the next-step checklist) is now the authoritative
release driver. goreleaser continues to fire on tag push and publishes
binaries to GitHub Releases.

### Homebrew auto-bump (v0.35.1 era, documented in v0.35.1 CI)

A `homebrew` job was added to `release.yml` (shipped in the CI/homebrew
branch, merged as part of the v0.35.1 / pre-v0.36.0 period): on release
it checks out `codedeviate/homebrew-cli` and runs that tap's
`scripts/bump.sh sercon <version>` to recompute the source-tarball sha256
and rewrite `Formula/sercon.rb`. Requires a `HOMEBREW_TAP_GITHUB_TOKEN`
secret; skips cleanly if absent. v0.35.1 was the last hand-bumped version.

## 8. Documentation tooling

### Typst PDF pipeline

`make manual` moved off the earlier HTML-driven renderer onto recon's **typst**
engine. The PDF now ships a `--cover` title page (title, subtitle, version, date,
author), a page-number footer, and a page-numbered table of contents, set in
recon-native **IBM Plex Sans** with no vendored font. MANUAL.md no longer carries
a raw-HTML cover block or a hand-curated TOC — both are emitted by the renderer —
so `make manual` dropped `--unsafe-html`. To keep the markdown source
tooling-friendly while satisfying typst, the file is piped through
`scripts/typst-safe.awk` at render time: it escapes prose angle brackets and strips
HTML comments that fall outside code fences. `version-check` / `release-prep`
collapsed to a single footer version marker.

### Completeness sweep + guard

Every binding member across all reserved-global namespaces (plus `console` and
`server`) was brought to a full structured-docs standard — each carries a Summary,
Params, ReturnType, Returns, Errors, and Example — which enriches both the §16
binding reference and the `.d.ts` editor hovers. `paymentproviders` gained a full
per-provider reference. `cmd/sercon/docs_completeness_test.go` makes this permanent:
`TestDocsComplete` fails if any swept member is missing a field, and
`TestDocsComplete_CoversAllNamespaces` fails if a namespace is left unswept.

## 9. Cloud

### `cloud` namespace + Google provider

A new reserved global, `cloud`, joined the surface as a container for
provider callables — one per cloud vendor, Google first. `cloud.google(opts?)`
resolves ADC (`gcloud auth application-default login`,
`GOOGLE_APPLICATION_CREDENTIALS`, or attached-metadata identity) unless
`opts.credentials` supplies a service-account key (file path or inline JSON),
and returns a handle exposing four typed services plus a generic escape hatch:
`storage()` (Cloud Storage: bucket/object CRUD + `readObject`/`putObject`),
`compute()` (Compute Engine: instance list/get/create/delete/start/stop, plus
zone/disk listing), `iam()` (service-account CRUD, keys, `getIamPolicy`/
`setIamPolicy`), and `secrets()` (Secret Manager: secret CRUD, add/access
version — `accessSecretVersion` base64-decodes the payload so scripts never
see the raw wire format). `call({api, version?, httpMethod?, path, params?,
body?})` is the generic path-based REST fallback for any `*.googleapis.com`
endpoint without a typed service above, sharing the same auth as the parent
handle. Built on `google.golang.org/api` (pure Go, no cgo); every SDK
response round-trips through JSON into a plain map/slice so goja emits clean
objects instead of leaking SDK-internal fields.

Errors are normalised through a shared `mapGoogleError`: a `*googleapi.Error`
becomes `{ code, status, message, details }` (the API's HTTP status and body);
anything else (DNS, TLS, timeout, connection refused) becomes `{ code: 0,
status: "TRANSPORT", message }`. This rides on an additive, backward-compatible
`ErrorFields() map[string]any` hook added to the async-rejection path
(`scriptengine.PromisifyAsync`'s `rejectionValue`): any rejected error
implementing it gets its fields copied onto the JS `Error` object, so scripts
read `e.code`/`e.status`/`e.message` directly in a `catch` without a
provider-specific unwrap step — a pattern future providers (AWS, Azure) reuse
rather than each inventing their own error shape.

Documentation follows the same structured `MemberDoc` model as every other
namespace (`docs_cloud.go`), with one twist: `storage()`/`compute()`/`iam()`/
`secrets()` and `call()` are all built at script-run time inside the
`google()` accessor (not as static namespace members), so the automatic
`.d.ts`/reference-generator reflection can't recover their shape on its own.
The `google` entry's `ReturnType` hands the emitter a hand-written but
accurate composite type covering every method's signature, so
`cloud.google(...)`'s return still type-checks with real per-method shapes in
the emitted `sercon.d.ts` — not just an opaque `unknown`. A maintainer-run
live smoke test (`examples/scripts/cloud-google-smoke.ts`, ADC-gated,
self-skips without `PROJECT` set) exercises all four services plus `call()`
against a real project; it is intentionally excluded from `make demo`/CI,
which stays fully offline via the `httptest`-mocked Go unit tests in
`cloud_google_*_test.go`.

### AWS provider

`cloud.aws(opts?)` followed the same shape as Google: it resolves the
standard AWS credential chain (environment variables, `~/.aws/config` +
`~/.aws/credentials` profiles/SSO, or an attached EC2/ECS/Lambda identity via
IMDS) unless `opts.credentials` supplies static keys, and `opts.region`/
`opts.profile` steer region and named-profile selection. The handle exposes
nine typed services — `s3()` (bucket/object CRUD, `getObject`/`putObject`),
`ec2()` (instance describe/run/terminate/start/stop, volume and availability-
zone listing), `iam()` (user/role/policy listing, user CRUD, policy
attachment), `secretsmanager()` (secret CRUD, `getSecretValue`/
`putSecretValue` — `getSecretValue` returns `{ value }` decoded once, never
the raw SDK envelope), `sts()` (`getCallerIdentity`, `assumeRole`,
`getSessionToken`), `lambda()` (function list/get/create/delete plus
`invoke`, which hand-builds its result since the invoked function's raw
JSON response `Payload` must stay a string rather than round-tripping into a
numeric byte array), `sqs()` (queue CRUD, send/receive/delete message, queue
attributes), `cloudwatch()` (metric/alarm listing plus pass-through
`getMetricData`/`getMetricStatistics`/`putMetricData` for their
deeply-nested query shapes), and `cloudwatchlogs()` (log group/stream
listing, `getLogEvents`/`filterLogEvents`, Logs Insights `startQuery`/
`getQueryResults`) — built on the pure-Go `aws-sdk-go-v2` (no cgo). Unlike
Google, there is no generic path-based REST escape hatch: AWS has no single
uniform REST protocol across services (query/XML for EC2/IAM/S3/STS/
CloudWatch, JSON for Secrets Manager/Lambda/SQS/CloudWatch Logs), so a
`call()` fallback would need a different wire encoder per service family
rather than one shared client.

Errors reuse the same `ErrorFields()` pattern Google established: a
`smithy.APIError` becomes `{ code, status, message, details }`; anything else
(DNS, TLS, timeout, connection refused) carries `code: ""`/`status: 0` with
the raw error text as `message`. Documentation follows the same twist as
Google's: `s3()`/`ec2()`/`iam()`/`secretsmanager()`/`sts()`/`lambda()`/
`sqs()`/`cloudwatch()`/`cloudwatchlogs()` are all built at script-run time
inside the `aws()` accessor, so the `aws` `MemberDoc` entry's `ReturnType`
(a hand-written, multi-line composite covering all nine services) is what
gives `cloud.aws(...)`'s return real per-method shapes in the emitted
`sercon.d.ts`, rather than flat per-method entries (which the automatic
`.d.ts`/reference-generator reflection can't reach for a runtime-built
handle either way). A maintainer-run live smoke test
(`examples/scripts/cloud-aws-smoke.ts`, credential-chain-gated, self-skips
without `AWS_REGION`/`AWS_DEFAULT_REGION` set) exercises `sts()`, `s3()`, and
`cloudwatchlogs()` against a real account; like its Google counterpart it is
intentionally excluded from `make demo`/CI, which stays fully offline via the
`httptest`-mocked Go unit tests in `cloud_aws_*_test.go`.

### Azure provider (PROVISIONAL — unverified against a live Azure account)

`cloud.azure(opts?)` completes the 3-provider arc (aws, google, azure) under
the same `cloud.<vendor>` shape, but with an explicit caveat the other two
don't carry: there is no Azure credential/subscription available to the team
that built it, so **nothing about its wire behaviour has been verified
against a real Azure account** — every method is exercised only by
`httptest`-mocked Go unit tests (`cloud_azure*_test.go`). Treat any claim
about actual Azure REST/SDK response shapes in the docs as unverified until
a maintainer with real Azure access runs the smoke script below.

`cloud.azure(opts?)` resolves credentials via a client-secret (service-
principal) credential when `opts.tenantId`/`opts.clientId`/`opts.clientSecret`
are all supplied, else falls back to `DefaultAzureCredential`'s chain
(environment variables, managed identity, `az login`, and the other links in
the default chain). Unlike Google/AWS, Azure splits its surface into two
distinct shapes:

- **ARM (Resource Manager) services** — `resourceGroups()` (list/get/create/
  delete), `compute()` (Virtual Machines: `listVirtualMachines`/
  `getVirtualMachine`/`start`/`powerOff`/`deallocate`/`delete`), and
  `resources()` (the generic resource-graph API: `listByResourceGroup`/
  `getById`) — plus a generic `call({path, apiVersion, method?, params?,
  body?})` ARM REST escape hatch for APIs without a typed service above. All
  four require a subscription id (`opts.subscriptionId`, else
  `AZURE_SUBSCRIPTION_ID`); long-running operations (`Begin*` SDK calls) are
  polled to completion (`PollUntilDone`) before the promise resolves, and
  list-style methods page through every page the SDK's pager returns before
  resolving with `{ value: [...] }`.
- **Data-plane services** — `blob(accountUrl)` (container/blob CRUD:
  `listContainers`/`listBlobs`/`download`/`upload`/`deleteBlob`) and
  `keyvaultSecrets(vaultUrl)` (`listSecrets`/`getSecret`/`setSecret`/
  `deleteSecret`) — take the target endpoint URL directly as an argument
  rather than resolving one from a subscription, and have no subscription
  requirement of their own. `blob`'s list/download results come back with
  PascalCase keys (`Name`, `Properties`, ...) since the Storage Blob SDK's
  generated structs have no JSON marshalling of their own (the wire protocol
  is XML) and `toPlain`'s JSON round-trip falls back to the Go field names
  verbatim; `keyvaultSecrets`' results come back lowercase-camelCase (`id`,
  `attributes`, `contentType`, ...) since `azsecrets` structs do define their
  own `MarshalJSON`, same convention as the ARM services.

Errors reuse the same `ErrorFields()` pattern Google/AWS established: an
`azcore.ResponseError` becomes `{ code, status, message, details }`; anything
else (DNS, TLS, timeout, connection refused, credential/token acquisition
failure) carries `code: ""`/`status: 0` with the raw error text as `message`.
Documentation follows the AWS convention (not Google's): no flat per-method
`MemberDoc` entries are written out for `resourceGroups()`/`compute()`/
`resources()`/`blob()`/`keyvaultSecrets()`, since they're all built at
script-run time inside the `azure()` accessor same as AWS's/Google's runtime-
built handles — the `azure` entry's `ReturnType` (a hand-written, multi-line
composite covering all six method groups) is what gives `cloud.azure(...)`'s
return real per-method shapes in the emitted `sercon.d.ts`. A maintainer-run
live smoke test (`examples/scripts/cloud-azure-smoke.ts`,
`AZURE_SUBSCRIPTION_ID`-gated with optional `AZURE_TENANT_ID`/
`AZURE_CLIENT_ID`/`AZURE_CLIENT_SECRET`/`AZURE_STORAGE_ACCOUNT_URL`/
`AZURE_KEYVAULT_URL`, self-skips without a subscription id) is provided for
exercising `resourceGroups()`, `compute()`, `call()`, `blob()`, and
`keyvaultSecrets()` against a real subscription — like its Google/AWS
counterparts it is excluded from `make demo`/CI, which stays fully offline
via the `httptest`-mocked Go unit tests — but unlike them, **this script has
never actually been run against live Azure**, so its first real run is the
actual validation of the `cloud.azure` surface, not this document.

## 10. MCP server (`mcp.*`)

### `mcp` namespace — server, Phase 1 (v0.91.0)

A new reserved global, `mcp`, lets a script author a Model Context Protocol
server — the standard tools/resources/prompts surface that LLM apps (Claude
Desktop, IDEs) consume. `mcp.serve({name, version})` returns a handle on which
`tool()`/`resource()`/`prompt()` register primitives with async JS handlers,
served over two transports: **stdio** (the classic subprocess transport clients
launch — Unix-only this phase; Windows throws a clean error and uses HTTP) and
**Streamable HTTP** (`listen({port})`, cross-platform). Built on the official
pure-Go `github.com/modelcontextprotocol/go-sdk` (pinned v1.6.1).

The engine work is the bridge, not the protocol. The SDK invokes handlers on its
own goroutines while goja is single-threaded, so `callJSHandler` funnels every
handler through the event loop — converting args/results **on-loop** and carrying
only native Go data across the goroutine boundary — mirroring `server_http`'s
Promise-settlement pattern (with a `defer recover()` so a marshalling panic
becomes a rejected call rather than a process crash). stdio's hard rule (stdout
carries JSON-RPC only) is met with an fd-level redirect (`dup2` fd 1 → stderr,
armed at `serve()` time) while JSON-RPC is written to the saved real stdout via
the SDK's `IOTransport` — necessary because goja_nodejs's console captures
`os.Stdout` at init, so a Go-level swap can't move it. Tool-handler errors
surface as `isError` results; resource/prompt errors are protocol errors.

### `mcp` server — Phase 2 (v0.91.0)

The runtime niceties. Primitives can now be added/removed after serving (auto
`list-changed` via `removeTool`/`removeResource`/`removePrompt`); the request
`ctx` gained `progress(p, total?)` and `log(level, msg, data?)` (progress-token-
correlated; `log` reaches only clients that called `SetLoggingLevel`);
`resourceTemplate()` (RFC-6570 templates), resource subscriptions
(`resourceUpdated()` + lazy-watch `onSubscribe`/`onUnsubscribe`), a single
`completion()` dispatcher, and a `pageSize` option. Server→client sends route
through a shared `asyncSettle` helper that holds the loop (`HoldRun`) for the
round-trip so a top-level `await` can't have its settlement silently dropped when
the loop would otherwise go idle. Sampling/elicitation/roots and HTTP OAuth are
the planned Phase 3; a client (`mcp.connect`) is a separate effort.

### `mcp` server — Phase 3 (v0.92.0)

Full-spec completion: the client-dependent calls and HTTP auth. The request
`ctx` gained three server→client requests — `ctx.sample()` (run a completion on
the *client's* LLM via `sampling/createMessage`), `ctx.elicit()` (ask the user,
through the client's UI, to confirm or fill a small form), and `ctx.roots()`
(read the client's current filesystem/URI roots) — each reusing a value-returning
`asyncSettleResult` (the `asyncSettle` shape, but settling with the round-trip's
result via a JSON round-trip) and each pre-checking the negotiated client
capability so an unsupported call throws a clear `"mcp: client does not support
X"` instead of an opaque protocol error or a hang. The mid-handler round-trip
does not deadlock the single-threaded loop because the SDK reads the client's
reply on an independent goroutine while the loop is merely *held* alive. A
`srv.onRootsChanged(fn)` push counterpart mirrors `ctx.roots()`. `srv.listen`
gained an `auth` option turning the Streamable HTTP transport into an OAuth 2.1
**resource server**: a script `verify(token, req)` callback validates bearer
tokens (funnelled onto the loop per request, and *failing closed* on any non-
identity return), RFC 9728 protected-resource metadata is served at
`/.well-known/oauth-protected-resource`, and scopes are enforced with a correct
`WWW-Authenticate` challenge on `401`. Token issuance and dynamic client
registration stay out — those are the authorization server's role. Shipped
alongside an advanced-examples cookbook (MANUAL §5.15.2 + nine runnable/
illustrative scripts, the client-dependent ones Go-tested via the in-memory and
subprocess SDK clients). The MCP **server** is now full-spec; an MCP **client**
(`mcp.connect`) and Windows stdio remain the open follow-ups.

### `mcp` client — Phase 1 (v0.93.0)

The other half of the protocol: sercon can now *consume* MCP servers, not just
be one. `mcp.connect` is a namespace of per-transport factories —
`mcp.connect.stdio({command, env?, cwd?})` launches a server as a subprocess
and speaks JSON-RPC over its stdio (the mirror of `srv.stdio()`, and how hosts
like Claude Desktop start a server), while `mcp.connect.http(url, {headers?})`
attaches to an already-listening Streamable HTTP endpoint (the `headers` can
carry a bearer token for a `listen({auth})`-protected server). Both resolve one
session handle: `serverInfo`/`capabilities` from the initialize handshake, then
`listTools`/`callTool`, `listResources`/`listResourceTemplates`/`readResource`,
`listPrompts`/`getPrompt`, `ping`, and an idempotent `close`. A tool failure
comes back as `{isError: true}` rather than a throw (the same soft-failure
contract the server side emits); transport and protocol failures do throw. The
`asyncSettleResult` helper built for the server's mid-handler sends was lifted
to a shared free function so every client call reuses it — the SDK request runs
off the event loop and settles on it — and the connection holds the loop open
(`HoldRun`) for its lifetime, released exactly once on `close`. Two SDK result
fields (`CallToolResult.IsError`, `PromptArgument.Required`) are `omitempty`
booleans, so a naive JSON round-trip would turn a legitimate `false` into
`undefined`; small view structs restore them. Phase 1 is consume-only —
answering the server's own sampling/elicitation/roots requests, change
notifications and subscriptions, and an OAuth *client* are the planned later
phases.

### `mcp` client — Phase 2 (v0.94.0)

The client stops being purely pull-driven and starts reacting to what the
server pushes. Six single-slot notification setters register callbacks —
`onToolsChanged`/`onResourcesChanged`/`onPromptsChanged` (list changed),
`onResourceUpdated(uri)`, `onLoggingMessage({level,logger?,data})`, and
`onProgress({progressToken,progress,total?,message?})` — wired unconditionally
at connect to dispatchers that read a mutex-guarded slot and invoke the script
callback on the loop via `LoopCallable`, exactly the discipline the server side
uses for `onSubscribe`. `subscribe`/`unsubscribe` manage resource
subscriptions, `setLoggingLevel` opts into server logs (which otherwise never
arrive), and `complete(ref, argName, partial)` requests argument completions
(returning `{values, total?, hasMore?}`, symmetric with the server's
completion result). Underneath, the connection gained a proper lifecycle: a
connection-scoped cancellable context replaced the per-call
`context.Background()`, a `sess.Wait()` death-watcher releases the loop hold
when the session ends, and the auto-pagination loops gained a page cap. One
honest limitation is documented: over Streamable HTTP a *graceful* server
shutdown keeps the client's SSE stream alive, so the death-watcher only fires
on stdio subprocess exit or an abrupt failure — an idle HTTP client to a
gracefully-stopped server still relies on the run ending. Host responders
(answering sampling/elicitation, exposing roots) and an OAuth client remain
Phases 3–4.

### `mcp` client — Phase 3 (v0.95.0)

The client becomes a full MCP *host*, answering the server's own mid-request
calls — the mirror image of the server's Phase-3 `ctx.sample`/`ctx.elicit`/
`ctx.roots`. Three optional connect options: `onSample(req)` answers
`sampling/createMessage` (return a string, wrapped as text, or a
`{content,model?,stopReason?,role?}` object), `onElicit(req)` answers
`elicitation/create` (`{action,content?}`), and `roots: [{uri,name?}]` exposes
the client's roots to `roots/list`, with a handle method `setRoots` to update
them at runtime (firing `roots/list_changed`). Sampling and elicitation are
advertised to the server only when the responder is provided — a client that
can't answer doesn't claim the capability — while the SDK advertises roots
unconditionally, so the `roots` option just seeds the initial set. The two
request-response responders reuse the server's `callJSHandler` bridge (lifted
to a shared package-level function): the SDK invokes them on its own goroutine,
`callJSHandler` runs the script handler on the loop, awaits a returned Promise,
and hands back native Go — so an async responder is awaited correctly and no
goja value ever crosses a goroutine. Only the OAuth client and the SSE
transport (Phase 4) remain.

### `mcp` client — Phase 4 (v0.96.0) — the client is full-spec

The final phase closes out the client: a legacy transport, reconnection
tuning, and OAuth. `mcp.connect.sse(url, {headers?})` adds the HTTP+SSE
transport (a third factory beside `stdio`/`http`, routed through the same
connection machinery, so notifications and host responders work over it too).
`connect.http` gained `maxRetries` (caps the Streamable-HTTP transport's
reconnect attempts; `0` or negative disables — sercon maps non-positive values
to the SDK's disable sentinel so the documented contract holds). And an OAuth
client: `connect.http(url, { auth: { getToken } })` wraps a script callback in
the SDK's `auth.OAuthHandler` — `getToken` (sync or async, must return a
non-empty string) is invoked via the same on-loop `callJSHandler` bridge to
supply the bearer token, and is re-invoked when a request comes back 401/403 so
the script can refresh an expired token. The script owns *how* it obtains the
token (a static value, a `net.http` client-credentials call, its own store), so
sercon needs no browser-based authorization-code flow. This required promoting
`golang.org/x/oauth2` from an indirect to a direct dependency (still pure-Go).
With this, the MCP **client** matches the **server**: both halves of the
protocol — consume, react, host, and authenticate — are now scriptable.

### `mcp` context/cancellation hardening (v0.97.0)

A robustness pass over both MCP halves, closing the two tracked follow-ups
from the phase reviews. The on-loop bridge that runs every script handler
(`callJSHandler`) now selects on its request context, so a handler whose
Promise never settles no longer pins the SDK request goroutine forever — and a
new `handlerTimeout` option (on `mcp.serve` and every `mcp.connect.*`,
milliseconds; default 5 minutes, `0` disables) bounds that wait, surfacing a
tool timeout as an `isError` result. The default is generous on purpose: a
handler legitimately awaiting `ctx.elicit` (a human) or `ctx.sample` (an LLM)
must not be cut off, and the deadline reclaims only the *waiting goroutine* —
the handler's JavaScript keeps running on the loop (a runaway synchronous
script remains the `--timeout` / `vm.Interrupt` job). Separately, the client's
connection context is now rooted at the engine Run context (via a newly
exported `scriptengine.RunContextFromVM`), so a script timeout / Run-end
cancels in-flight calls and reaps the stdio subprocess instead of leaking them
until process exit.

## Bundled libraries

### `paymentproviders` (v0.63.0)

A TypeScript payments library **compiled into the sercon executable** and served
to scripts via an `Options.ModuleLoader` over an `//go:embed`-ed, esbuild-bundled
artifact (`cmd/ppbundle` builds it; `make paymentproviders-check` guards drift).
`import { kcov3 } from "paymentproviders"` works with no install. Cycle 1 ships
**KCO v3** (Kustom Order-Management + Checkout) over a shared core (`net.http`
transport, Basic `merchantId:sharedSecret` auth, `Klarna-Idempotency-Key`,
`PaymentError`, minor-unit money). Decomposed so Nets/Svea/Qliro (Cycle 2) and
SwedbankPay (Cycle 3) are additive namespaces.

### Cycle 2 — Nets / Svea / Qliro (v0.64.0)

The core gained a **pluggable per-request signer** (`ctx.sign(method,path,body)
→ headers`) so each provider supplies its own auth; `kcov3` migrated onto it
(Basic). Added `netsv1` (secret-key header), `sveacheckout2` (Svea token:
`base64(merchantId:UPPER(SHA512(body+secret+ts)))` + `Timestamp` header), and
`qlirov2` (`Qliro base64(SHA256(body+secret))`). A library-local byte-base64
(`core/crypto.ts`) supplies Qliro's raw-digest base64 (sercon's `crypto.hash`
returns hex; `text.str.base64Encode` is UTF-8-only). Namespaces are
**version-labelled** so a new provider API version ships as a sibling. Cycle 3 =
SwedbankPay (`swedbankpayv2` + `swedbankpayv3`).

### Cycle 3 — SwedbankPay v2 / v3 (v0.65.0)

Added `swedbankpayv2` + `swedbankpayv3` — Bearer auth (simple) over SwedbankPay's
**HAL/hypermedia** model: a payment-order response carries an `operations` array
(`{rel, href, method}`), and capture/refund/cancel POST to the matching operation's
absolute `href` rather than a fixed endpoint. New `core/hal.ts` (`findOperation`,
exact-or-hyphen-delimited-suffix rel match) + an absolute-URL tweak to `apiRequest`;
a shared `swedbankpay/common.ts` builder backs both thin version namespaces. Exposes
a low-level `operation(paymentOrderOrUrl, rel, body?)` primitive plus the unified
`capturePayment`/`refundPayment`/`cancelPayment`. Completes the provider set
(`kcov3`, `netsv1`, `sveacheckout2`, `qlirov2`, `swedbankpayv2`, `swedbankpayv3`).
