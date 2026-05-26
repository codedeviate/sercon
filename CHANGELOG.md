# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
See [CLAUDE.md](./CLAUDE.md) for the project's commit-message conventions.

## [Unreleased]

Nothing yet.

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

- `LICENSE` — MIT, copyright 2026 Thomas Bjork.
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
