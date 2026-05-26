# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
See [CLAUDE.md](./CLAUDE.md) for the project's commit-message conventions.

## [Unreleased]

### Fixed

- `make manual` no longer passes `--toc`. recon's auto-injected TOC
  was being placed above the cover-page `<div>`, pushing the cover
  to page 2. `MANUAL.md` ships its own curated `## Table of contents`
  section, which stays in flow and renders in the correct order.

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
