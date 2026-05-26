# Out of scope / backlog

Ideas, follow-ups, and known gaps that aren't implemented yet. Promotion
from this list to a real issue or commit is the only way these become
"real" work. Keep entries terse; expand into a spec or plan once picked up.

## Engine

- **Source-map-aware error mapping.** `transpile.go` claims this in its
  package comment but esbuild's source maps are not yet wired into
  `*goja.Exception` stack frames. Errors in `.ts` scripts currently point
  at the transpiled JS line numbers.
- **Top-level export capture.** `Engine.Run` resolves with whatever
  `__resolve` receives, which is always `undefined` today. Wiring the
  entry-script body so its trailing expression flows into the resolve
  call would let hosts get a return value back.
- **Per-Run `ScriptRoot` override.** Currently a single `Engine`-level
  setting. A `RunOptions` overload (or variadic) would let callers point
  different runs at different directories without rebuilding the engine.
- **True `RegisterConstructor` runtime semantics.** The d.ts emitter
  produces `declare class`, but at runtime the constructor is treated
  like a plain `vm.Set`. Hooking it up so `new Foo(...)` works in JS
  and respects the returned Go type's methods is open work.
- **Reset / clear registrations.** No way to remove a binding once
  added. Probably fine for the intended ad-hoc-testing use case, but
  worth flagging.

## Transpile / entry rewriter

- **Robust import parsing.** `rewriteEntryESMToCJS` is a line scanner
  with a handful of regexes. It handles the cases in the test suite,
  but multi-line imports with comments, complex destructuring, or unusual
  whitespace are not guaranteed. A small AST-based parser (using
  `esbuild` Parse output, or a tiny hand-rolled one) would be more
  durable.
- **ESM default-export interop.** The `__esModule ? .default : m`
  pattern is in place but lacks a dedicated test fixture for a TS file
  that uses `export default`.
- **JSX / TSX end-to-end.** The source loader recognises `.tsx` and
  esbuild is configured for it, but there's no example script or test
  proving the path works.

## Require / module loading

- **Custom `PathResolver`.** Currently relies on
  `require.DefaultPathResolver`. Hosts that want sandboxed or virtualised
  module trees (in-memory FS, network sources) need to fall back to
  registering their own `Registry`, bypassing parts of the engine.
- **`package.json` `main` honoured for `.ts` projects.** The registry
  reads `package.json` but assumes JS entry points. A TS-aware variant
  could prefer a `types` or `source` field.
- **JSON / data imports through the TS loader.** Today `.json` is
  passed through; `import data from './data.json'` works because esbuild
  rewrites it to a require, but there's no test pinning that down.

## `.d.ts` generator

- **Promised[T] introspection in real bindings.** The marker type and
  detection logic exist (`returnType` looks for `"Promised["`), but the
  example CLI uses `PromisifyAsync` which returns a plain
  `func(goja.FunctionCall) goja.Value`. Result: every async binding's
  return type renders as `unknown` instead of `Promise<T>`. Either
  switch `PromisifyAsync` to return `Promised[T]`, or have the
  generator infer the wrapper from the factory's call signature.
- **Struct method receiver handling.** `funcSig` doesn't currently
  strip the receiver when reflecting on `reflect.Method.Type`, so
  methods of registered structs may emit an extra leading parameter.
- **JSDoc comments on emitted declarations.** Generated output is
  pure types — no `/** ... */` blocks. Pulling doc strings from a
  per-binding metadata map would make editor hover useful.

## CLI

- **`-v` does very little.** Parsed but only used to print a duration
  on failure. Could surface the transpiled JS, the rewritten entry
  script, or per-require resolution traces.
- **Stdin script support.** `sercon -` to read a script from stdin would
  be useful for one-liners and shell pipelines.
- **Watch mode.** Re-run on file change for iterative work.
- **Exit-code matrix.** Today the CLI returns 1 for any failure. Distinct
  codes for transpile error / timeout / script throw would help scripting.

## Repo / tooling

- **`golangci-lint` config.** The spec said "minimal if added" and we
  didn't add one. A small `.golangci.yml` with at least `govet`,
  `staticcheck`, and `errcheck` would catch regressions early.
- **CI workflow.** No GitHub Actions / equivalent yet. A simple
  `go build`/`go test`/`go vet` matrix on Go 1.22+ would be a sensible
  starting point.
- **Release automation.** Releases are currently cut by hand
  (bump `scriptengine.Version`, move `CHANGELOG.md` entries, tag,
  `gh release create`). A release-please / cliff-style step keyed off
  Conventional Commits would match the conventions in `CLAUDE.md`.
- **Prebuilt binaries on releases.** GitHub releases currently ship
  only the source tarball. Cross-compile darwin-{arm64,amd64},
  linux-{amd64,arm64}, windows-amd64 with `make release`'s slim flags
  and `gh release upload`. Needed before the Homebrew tap formula
  can pull pinned binaries.

## Recon-inspired feature backlog

The following capabilities were extracted by surveying
`../../Rust/recon/script/*.rhai` (89 scripts). Anything pulled in
should match sercon's scope: **script-engine-only**, surfaced through
the `api.*` namespace (or close cousins), no new top-level CLI
subcommands. Network-touching bindings must follow the existing
`PromisifyAsync` pattern. Items that need an external tool are
called out under [External tool integrations](#external-tool-integrations).

### Protocol probes & connectivity

- **`http(url, opts?)`** — HTTP(S) requests with status assertion, body
  handling, proxy opts, retry, auth, conditional fetches (script:
  `http.rhai`). Extends the current `api.http.get/post`.
- **`tcp(url, opts?)`** — TCP connect probe with resolved IP and
  latency reporting (script: `tcp.rhai`).
- **`ping(host, count?)`** — TCP or ICMP ping with RTT min/avg/max
  and packet loss (script: `ping.rhai`).
- **`dns(host, types?)`** — DNS lookups with flexible record-type
  filtering (script: `dns.rhai`).
- **`tls(host, port?)`** — Certificate inspection: CN, issuer, expiry,
  days-remaining (script: `tls.rhai`).
- **`ntp(host)`** — NTP clock offset and round-trip delay measurement
  (script: `ntp.rhai`).
- **`whois(domain)`** — Two-hop WHOIS with registrar referral (script:
  `whois.rhai`).
- **`smtp(url, opts?)`** — SMTP capability probe, STARTTLS availability,
  AUTH mechanisms (script: `smtp.rhai`).
- **`wss(url)`** — WebSocket handshake with ping/pong round-trip
  (script: `ws.rhai`).
- **`netstatus::check()`** — Aggregate connectivity probe set (script:
  `netstatus.rhai`).

### Remote services & caching

- **`redis(url, command?)`** — Redis RESP protocol client (PING or
  custom commands) (script: `redis.rhai`).
- **`memcached(url)`** — Memcached text protocol, version and stats
  (script: `memcached.rhai`).
- **`ldap(url)`** — Anonymous LDAP bind + RootDSE attribute query
  (script: `ldap.rhai`).
- **`dict(url, word?)`** — RFC 2229 DICT protocol word lookup (script:
  `dict.rhai`).
- **`sqlite(path_or_memory)`** — In-memory or file-backed SQLite;
  `exec()`, `query()`, `query_value()` (script: `sqlite.rhai`).

### TLS / encryption / signing

- **`jwt::sign(claims, secret)`** / **`jwt::view(token)`** /
  **`jwt::validate(token, secret)`** — Create, decode, verify JWTs
  (script: `jwt.rhai`).
- **`encrypt::keygen()`** — Generate a fresh age X25519 keypair
  (script: `encrypt.rhai`).
- **`encrypt::encrypt(data, recipients)`** /
  **`encrypt::encrypt_armored(...)`** — Encrypt for age public keys
  (script: `encrypt.rhai`).
- **`encrypt::decrypt(cipher, identities)`** — Decrypt with age
  identities (script: `encrypt.rhai`).
- **`encrypt::rekey(cipher, old_ids, new_recipients)`** — Re-encrypt
  between recipient sets (script: `encrypt.rhai`).
- **`encrypt::detect_backend(recipient_str)`** — Dispatch age vs PGP
  by recipient format (script: `encrypt.rhai`).

### Email authentication (SPF / DKIM / DMARC / MTA-STS / BIMI / TLS-RPT)

- **`email::all(domain)`** — Aggregate SPF, DMARC, MTA-STS, TLS-RPT,
  BIMI verdicts (script: `email.rhai`).

### Hashing & compression

- **`hash(algo, payload)`** — md5, sha1, sha256, sha384, sha512,
  sha3_256, sha3_512, blake3, crc32 (script: `hash.rhai`).
- **`compression::compress(algo, bytes)`** /
  **`compression::decompress(...)`** — gzip, deflate, zstd, brotli,
  bzip2, lz4, xz, snappy, zlib (script: `compression.rhai`).

### Encoding / decoding / barcodes

- **`encode::qr(data)`** — QR code as PNG blob (script: `encode.rhai`).
- **`encode::datamatrix(data)`** — DataMatrix 2D code as PNG (script:
  `encode.rhai`).
- **`encode::barcode(format, data)`** — Linear barcode (UPC, Code128,
  …) as PNG (script: `encode.rhai`).
- **`encode::decode(image)`** — Scan PNG/JPEG/WebP for barcodes or 2D
  codes (script: `decode.rhai`).
- **`text::detect(bytes)`** — Charset detection with BOM awareness
  (script: `text.rhai`).
- **`text::decode(bytes, charset)`** / **`text::encode(string, charset)`**
  — Charset round-tripping (script: `text.rhai`).
- **`text::normalize_newlines(s, style)`** — `lf` / `crlf` / `cr`
  conversion (script: `text.rhai`).
- **`checkdigit::inspect(algo, input)`** — Luhn, ISBN, EAN, …
  verification/creation (script: `checkdigit.rhai`).

### Archives & document handling

- **`archive::create(path, files)`** / **`archive::extract(path, dest)`**
  — Create/extract zip archives (script: `archive.rhai`).
- **`pdf_export_page(src, page, dest_or_opts?, opts?)`** — Render one
  PDF page to PNG/JPEG/WEBP (script: `pdf.rhai`).

### Data comparison

- **`compare(a, b)`** — Unified diff of two strings or blobs; returns
  diff, added, removed, binary flag (script: `compare.rhai`).

### Browser-like HTTP session

- **`browser()`** — Stateful HTTP session with automatic cookie jar
  and header replay. Methods: `set_user_agent`, `set_header`, `get`,
  `post` (Maps auto-serialised to JSON), `cookies` (script: `browser.rhai`).

### String utilities & formatting

- **`trim`**, **`ltrim`**, **`rtrim`**, **`strrev`**, **`strip_html`**,
  **`nl2br`** / **`br2nl`** — String shaping (script: `strutil.rhai`).
- **`preg_match`** / **`preg_replace`** — PHP-delimited regex capture
  and substitution (script: `strutil.rhai`).
- **`base64_encode`** / **`base64_decode`** /
  **`urlencode`** / **`urldecode`** / **`html_entity_decode`** —
  Encoding round-trips (script: `strutil.rhai`).
- **`str_pad`** / **`lpad`** / **`rpad`** — Padding (script: `strutil.rhai`).
- **`sprintf`** / **`printf`** — printf-style formatting (script:
  `strutil.rhai`).
- **`dirname`** / **`basename`** — POSIX path operations (script:
  `strutil.rhai`).
- **`date_format(unix_ts, fmt, tz?)`** — strftime-based timestamp
  formatting (script: `strutil.rhai`).

### JSON / querying

- **`jq(data, filter)`** / **`jq_all(data, filter)`** — jq-style
  first/all-results extraction (script: `jq.rhai`).

### Agent-browser automation

`agent-browser` is recon's headless-Chrome driver. The recon script
bindings are extensive and worth a dedicated namespace
(e.g. `api.agentBrowser.*`). All of these require the
`agent-browser` CLI on `PATH`; gate calls on an
`agentBrowser.available` boolean and surface a clean error otherwise.

- **Navigation** — `open(url, opts?)`, `close()`, `close_all()`,
  `back()`, `forward()`, `reload()` (script: `agent-browser-navigation.rhai`).
- **Locating** — `find(selector, opts?)` by role / text / label /
  placeholder / testid (script: `agent-browser-find.rhai`).
- **Interaction** — `click`, `dblclick`, `hover`, `fill`, `type_text`,
  `press`, `keyboard_type`, `keyboard_insert`, `check`, `uncheck`,
  `scroll`, `scrollintoview`, `focus` (script: `agent-browser-interaction.rhai`).
- **Inspection** — `get(selector, key?)`, `is_visible`, `is_enabled`,
  `is_checked`, `eval_js(code)` (script: `agent-browser-inspect.rhai`).
- **Capture** — `screenshot()`, `snapshot(include_interactive?)`,
  `pdf(opts?)` (scripts: `agent-browser-screenshot.rhai`,
  `agent-browser-snapshot.rhai`, `agent-browser-pdf.rhai`).
- **Defaults / escape hatch** — `default_options`, `clear_default_options`,
  `set_default_options`, `cmd(command, args)` for cookies / storage /
  tabs / network / console (scripts: `agent-browser-options.rhai`,
  `agent-browser-cmd.rhai`).

### AI agent integrations

- **`ai::request()`** — Builder chain: `.system()`, `.context()`,
  `.prompt()`, `.timeout()`, `.send()` (script: `ai.rhai`). Requires
  one of `claude` / `codex` / `copilot` / `gemini` on `PATH`.

### External tool integrations

- **HTTP via `recon` with `curl` fallback.** `api.http.*` currently
  goes straight through `net/http`. Add an opt-in path that shells out
  to `recon` (curl-compatible HTTP surface, ships in the same
  ecosystem) with a fallback to `curl` only when `recon` is not on
  `PATH`. Surface the choice as a single binding such as
  `api.exec.http(method, url, opts)` so scripts don't need to care
  which backend ran. Do **not** add a generic `curl` binding —
  prefer `recon`.
- **`agent-browser`.** Headless-Chrome automation; see the dedicated
  section above. Optional dependency, gated on `agentBrowser.available`.
- **`shell(cmd, opts?)`** / **`shell_stream(cmd, cb)`** — Run a
  subprocess with cwd / env / timeout; stream stdout/stderr line by
  line into a JS callback (script: `shell.rhai`). Needed as a building
  block for the recon-fallback path above.
- **`git(repo_path)`** — Wrap the `git` CLI: `branch()`, `is_clean()`,
  `rev_parse()`, `status()`, `add()`, `commit()`, `log()`,
  `diff_stat()`, `run_text()` (script: `git.rhai`). Requires `git` on
  `PATH`.
- **`gh()`** — Wrap the GitHub `gh` CLI: `auth_status()`,
  `pr_list()`, `repo_view()` (script: `gh.rhai`). Requires `gh` on
  `PATH` and authentication.
