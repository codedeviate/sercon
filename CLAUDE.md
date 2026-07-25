# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common commands

```bash
make build                                 # debug CLI -> ./sercon
make release                               # slim release CLI (-trimpath -s -w; ~30% smaller)
make manual                                # MANUAL.md -> MANUAL.pdf via `recon --md-to-pdf`
make test                                  # go test ./...
make test-integration                      # spin up dbplayground, run db.* integration tests, tear down
make vet                                   # go vet ./...
make lint                                  # golangci-lint v2 against .golangci.yml (one-shot via go run if not installed)
make demo                                  # run every success-path script under examples/scripts/ (excludes hang.ts)
make types                                 # regenerate examples/scripts/sercon.d.ts from the current CLI bindings
make release-prep VERSION=x.y.z            # bump version markers + print the next-step checklist (CHANGELOG still manual)
make release-verify VERSION=x.y.z          # poll until the goreleaser GitHub release for vX.Y.Z is published (run after tag push, before homebrew-bump)
make version-check                         # verify pkg/scriptengine/version.go and MANUAL.md cover/footer agree
make clean                                 # rm -f sercon MANUAL.pdf

CGO_ENABLED=0 go build ./...               # whole repo (must stay cgo-free)
go test ./pkg/scriptengine -run TestRun_PromiseResolveAwait   # single test
go test ./pkg/scriptengine -run TestWriteTypes_Golden -update # refresh golden .d.ts
./sercon examples/scripts/smoke.ts examples/scripts/async.ts  # smoke + async demo
./sercon -emit-dts /tmp/sercon.d.ts           # emit declaration file for the CLI's reserved-global surface
./sercon -timeout 200ms examples/scripts/hang.ts              # timeout demo (exits non-zero ~213ms)
./sercon --help | --examples | --version   # in-depth colourised help / feature tour / version
```

## Positioning: CLI-first, library unsupported

`sercon` is **CLI-first**. The `sercon` command (reconnaissance,
troubleshooting, testing) is the supported product. `pkg/scriptengine`
exists to serve that CLI; embedding it as a library in another Go program
is **unsupported and at the user's own risk** — its API may change without
notice and there are no stability or sandboxing guarantees. This matters
when weighing design trade-offs: constraints that exist only to make the
package a well-behaved embeddable guest (strict per-`Run` isolation as a
*public contract*, avoiding any process-owning behaviour) can be relaxed
when the CLI is the only consumer. The internal isolation choices below are
kept because they keep the engine correct across multiple scripts and
`--watch` re-runs, not because we promise them to library callers.

## Architecture

The engine (`pkg/scriptengine`) executes TypeScript via goja, with esbuild used in-process as the TS→JS transpiler and `goja_nodejs/eventloop` + `goja_nodejs/require` providing Promises and module loading. There are several non-obvious design choices a reader should know before editing.

### Per-Run runtime, shared registry

`Engine.Run` creates a fresh `eventloop.EventLoop` (and therefore a fresh `goja.Runtime`) every call. Registrations are reapplied per Run inside the loop callback, so no globals or module state leak between runs. The `*require.Registry` is the only piece that's reused across runs — it caches compiled bytecode (`*goja.Program`) per absolute path. Each Run still gets fresh module *exports* because `registry.Enable(vm)` builds a new `RequireModule` with its own `modules` map. Don't move the Registry construction into the Run path or you'll lose the compile cache; don't push the runtime onto the `Engine` or you'll start leaking state.

### Two transpile modes (entry vs required module)

esbuild rejects top-level `await` under `Format: FormatCommonJS`. The required-module path (`transpileTS`) uses `FormatCommonJS` and is straightforward. The entry-script path (`transpileEntry`) emits `FormatESModule`, then `rewriteEntryESMToCJS` line-scans the output, converts `import` statements to `require()` declarations, and wraps the remaining body in `(async () => { ... })().then(__resolve, __reject)`. The engine sets `__resolve` / `__reject` on the VM to capture the top-level Promise settlement. Any changes to entry-script semantics (top-level await, import handling) live in `transpile.go`.

### Keeping the event loop alive across async work

`eventloop.EventLoop.Run` exits as soon as `jobCount` drops to zero. `jobCount` is incremented only by `setTimeout` / `setInterval` / `setImmediate` — `RunOnLoop` does **not** count. So a host Promise-returning binding that does `go func() { ... loop.RunOnLoop(...) }()` will lose the race: the loop returns before the goroutine schedules its callback. `PromisifyAsync` parks a 24-hour `loop.SetTimeout` as a sentinel while async work is in flight and clears it on resolution. If you write a new I/O binding, route it through `PromisifyAsync` or replicate that pattern, otherwise the script will appear to "succeed" silently without running the async tail.

There is a second, subtler hazard the sentinel alone does **not** cover: the Go-side `loop.SetTimeout` / `loop.RunOnLoop` APIs defer their `jobCount` bookkeeping into an *aux job*, whereas the JS-facing `setTimeout` and `setImmediate` increment `jobCount` **synchronously**. In the sequence `await <setTimeout-backed promise>; await <host binding>`, the eventloop's `doTimeout` decrements `jobCount` to zero *before* running the timer callback; that callback runs the continuation which invokes the host binding and parks the sentinel — but the sentinel's increment is still queued in an aux job, so the run loop's top-level `for jobCount > 0` guard exits before it runs, silently dropping the host call's continuation. `PromisifyAsync` and `HoldRun` therefore call `bumpLoopSync(vm)` right after parking the sentinel: it schedules a no-op **`setImmediate`** (synchronous `jobCount++`, self-clears next tick) to bridge that transient-zero window. Any new keep-alive that relies on `loop.SetTimeout` must do the same, or it will lose the timer→host race.

### Require source loader (Node ext-fallback + .ts)

`goja_nodejs/require` only tries appending `.js` / `.json` when resolving paths. `Engine.newSourceLoader` adds `.ts` / `.tsx` extension fallback and index-file resolution, and transpiles `.ts` / `.tsx` sources before handing them to the registry. The registry's compile cache keys by whatever path the resolver passed in, which may have a `.js` suffix even when the on-disk file is `.ts` — that's fine, the cache is correct as long as the path is stable.

### Interrupt + cancel watcher

`Engine.Run` spawns one watcher goroutine that selects on `ctx.Done()`, an optional timeout, and a `done` channel closed after `loop.Run` returns. On firing it calls `vm.Interrupt` (aborts sync JS) and `loop.Terminate` (drains the loop). The watcher must always exit via `done` so it doesn't accumulate. `timeout.go` only holds the `ErrScriptTimeout` sentinel now — the watcher itself is inline in `engine.go` because it needs the per-Run atomic flags (`timedOut`, `canceled`) to distinguish `ErrScriptTimeout` from `ctx.Err()` after the fact.

### `.d.ts` generation — cycle protection is mandatory

Goja's internal types are massively self-referential (`Value` interface methods take/return `Value`, `*Object` exposes methods that take `*Object`, etc.). `dts.go` threads a `*typeCtx` through every recursion with a `visiting` set and a depth cap (`maxTypeDepth = 4`). When editing the d.ts generator, never call `tsType` / `structShape` / `funcSig` without forwarding the context — without it you get a runtime stack overflow on any registration that touches a goja type. Special cases for `goja.FunctionCall` (→ `(...args: unknown[])`) and `goja.Value` (→ `unknown`) live in `funcSig` and `tsType` and exist purely to make the output usable.

### Registration kinds

- `Register(name, value)` — plain global; value goes through `vm.Set` at apply time.
- `RegisterNamespace(name, members)` — object literal; members map built up front.
- `RegisterConstructor(name, ctor)` — distinct from `Register` only for the d.ts emitter (`declare class`).
- `RegisterFactory(name, factory)` / `RegisterNamespaceFactory` — for bindings that need the per-Run `*goja.Runtime` and `*eventloop.EventLoop` in scope. The factory is invoked once per Run inside the loop callback. **Required** for `PromisifyAsync` and anything else that calls `vm.NewPromise()` or `loop.SetTimeout`.

The d.ts generator can introspect namespace factories by calling them with `nil, nil`. This is only safe because the factory bodies in the example CLI build a members map without dereferencing the vm/loop arguments — closures capture them but aren't invoked at d.ts time. Any new factory must preserve that property or wrap the introspection in `defer recover()` (which `writeNamespaceDecl` already does).

### Options.DisableConsole (negative sense)

The spec called for `EnableConsole bool` defaulting to true, which collides with Go's zero-value convention. The field is exposed as `DisableConsole` so the zero value (`false`) means the console is on, matching the documented default without requiring a pointer.

## Where things live

- `pkg/scriptengine/engine.go` — `Engine`, `Options`, `Run`, `RunFile`, registration apply.
- `pkg/scriptengine/transpile.go` — esbuild wrapper, entry-script ESM→CJS rewriter, import statement parser.
- `pkg/scriptengine/bindings.go` — `registration` struct, `PromisifyAsync`, `Promised[T]` marker.
- `pkg/scriptengine/require.go` — TS-aware source loader and path resolver.
- `pkg/scriptengine/timeout.go` — `ErrScriptTimeout` sentinel only; the live cancellation watcher is inline in `engine.go`.
- `pkg/scriptengine/dts.go` — declaration generator with cycle detection.
- `pkg/scriptengine/engine_test.go` — 11 cases covering the 10 spec-required scenarios; uses `-update` flag for the golden `.d.ts`.
- `pkg/scriptengine/loop_callable.go` — `LoopCallable` wrapper + `NewLoopCallable` constructor; lets a captured `goja.Callable` be invoked from any goroutine. `.Call(buildArgs)` for off-loop callers; `.CallOnLoop(vm, args...)` for on-loop sites (using `Call` from a loop callback deadlocks).
- `pkg/scriptengine/hold_run.go` — `Engine.HoldRun(reason)` + sentinel-timer bookkeeping; keeps `loop.Run` alive while long-lived bindings (HTTP listeners, etc.) hold a refcount. Drained on Run end as a safety net.
- `pkg/scriptengine/polyfill.go` — per-Run `Symbol.asyncIterator = Symbol.for("Symbol.asyncIterator")` install so esbuild's `__forAwait` lowering and user code agree on the same iteration key.
- `cmd/sercon/main.go` — CLI plus the fifteen reserved top-level globals (`runtime`, `crypto`, `text`, `codec`, `fs`, `net`, `db`, `cloud`, `services`, `tui`, `image`, `web`, `audio`, `server`, `mcp`), each registered as a namespace factory by `registerSurface`.
- `cmd/sercon/image.go` — top-level `image` namespace factory + chainable `Image` handle: pure decode/encode/format helpers (`decodeImage`/`encodeImage`/`inferFormatFromPath`), WebP encode (`nativewebp`), SVG rasterize (`oksvg`/`rasterx`), and the per-call goja handle with resize/crop/rotate/flip/adjust/filter/compose ops. Pure-Go (imaging + x/image).
- `cmd/sercon/web.go` — top-level `web` namespace factory + the shared fetch helper (`webFetch`) backing every `*.load(url, opts?)`: reuses the `net.http` option surface, sends a default `sercon-web/<version>` User-Agent, throws on non-2xx. Thin dispatcher to the per-family sub-factories.
- `cmd/sercon/web_feed.go` — `web.feed.parse`/`load`: `mmcdole/gofeed` parse normalized to one model (`feedType`, unified title/link/published/…) with a per-item `raw` escape hatch for format-specific extras.
- `cmd/sercon/web_sitemap.go` — `web.sitemap.parse`/`load`: urlset / sitemapindex parsing, transparent gzip (`.xml.gz` magic-byte detection), `{expand:true}` bounded index recursion merging child URLs (per-child failures collected in `errors[]`).
- `cmd/sercon/web_html.go` — `web.html.parse`/`load` + the chainable `Node` handle: lenient HTML parse (`golang.org/x/net/html`), CSS queries via `andybalholm/cascadia` (`find`/`findAll`) and XPath via `antchfx/htmlquery` (`xpath`/`xpathAll`), with node accessors `text`/`html`/`innerHTML`/`tag`/`attr`/`attrs`.
- `cmd/sercon/docs_web.go` — structured `MemberDoc` docs for the `web` surface (drives the d.ts signatures + §17 reference).
- `cmd/sercon/server.go` — top-level `server` namespace factory; thin dispatcher to per-protocol sub-factories.
- `cmd/sercon/server_http.go` — `server.http` / `server.https` listener: options parsing, route compilation (stdlib `http.ServeMux` Go 1.22+ patterns), middleware chain, request marshalling, fluent `res` builder.
- `cmd/sercon/server_static.go` — `server.http.static({dir, stripPrefix, …})` returning a marker the route compiler unwraps into a stdlib `http.FileServer` mounted under `http.StripPrefix`.
- `cmd/sercon/server_ws.go` — `res.upgradeWebSocket(opts?)`; per-connection goroutine + buffered channel pump; async iterator that resolves frames on the loop via `LoopCallable`. Backed by `github.com/coder/websocket`.
- `cmd/sercon/server_smtp.go` — `server.smtp.listen`: go-smtp backend wrapping per-stage JS callbacks (`onMail`/`onRcpt`/`onData` + optional `auth`), enmime message parsing, custom LOGIN `sasl.Server`, STARTTLS. Backed by `github.com/emersion/go-smtp` + `github.com/jhillyerd/enmime`.
- `cmd/sercon/email_send.go` — `net.email.send`: stdlib `net/smtp` transport + in-tree MIME composition (text / multipart-alternative / multipart-mixed); per-recipient outcome capture; `starttls`/`tls`/`none` modes.
- `cmd/sercon/serve_cmd.go` — `sercon serve` subcommand: flag parsing (`--shutdown-timeout`, `--port-override`), access-log writer, READY-line writer on stdout, SIGTERM-graceful shutdown.
- `examples/scripts/` — runnable sample scripts; `hang.ts` is the timeout demo and must stay a single `while(true){}`. The `server-*.ts` demos bind a high random port (38080–38082), self-test, and `await srv.close()`.
- `claude-code-prompt.md` — the original spec for this build. Refer to it before redesigning anything significant.

## Editing rules of thumb

- Don't add cgo. The README and spec both lock this in; `CGO_ENABLED=0 go build ./...` is a deliverable check. sercon's own binary stays pure-Go and statically linked, no exceptions.
- Minimise external-CLI dependencies, and **never introduce one on your own initiative.** Each external tool must be explicitly requested or authorized by the maintainer before it's wired in — a candidate listed in `OUT-OF-SCOPE.md` is *not* standing authorization; it still needs an explicit go-ahead. (The existing wrappers — `services.git` / `services.gh` / `services.ai` / `services.agentBrowser` — were each authorized; treat them as precedent for *how*, not licence to add more.)
- Once a tool is authorized, an external CLI is a sanctioned escape valve, not a no-cgo violation — it links zero C into our binary (the tool is the user's separately-installed dependency). Use it only when a capability has no trustworthy pure-Go path, and shape it as an **optional, feature-detected fallback**: gate on the binary being on `PATH` via `exec.LookPath` (expose an `available` boolean), trap with a clean thrown error when it's absent, and keep the static binary fully functional without it (enrichment only, never a hard runtime requirement). Validate args before shelling out (no shell injection, no arbitrary paths without intent). See `OUT-OF-SCOPE.md` → *External-CLI fallbacks* for the candidate tools and open design calls.
- Don't introduce package-level state in `pkg/scriptengine` — everything hangs off `Engine`.
- Errors returned as the second value of a Go binding surface as thrown JS exceptions automatically (via `vm.NewGoError`). Don't swallow them at the binding layer.
- If you change the registered example surface in `cmd/sercon/main.go`, regenerate the golden in `pkg/scriptengine/testdata/` only if you also touched bindings used by `TestWriteTypes_Golden` (it has its own minimal fixture set, not the CLI's reserved globals).
- For bindings that need to invoke a captured JS Callable from a goroutine: use `scriptengine.NewLoopCallable(loop, fn)` + `.Call(buildArgs)`. On-loop callers should use `.CallOnLoop(vm, args...)` instead — invoking `Call` from a loop callback re-enters `RunOnLoop` and deadlocks. Do NOT call the Callable directly from a non-loop goroutine; goja's runtime is single-threaded.
- For long-lived bindings (servers, listeners, anything that doesn't have a single Promise to keep the loop's `jobCount` nonzero): call `eng.HoldRun(reason)` to keep `loop.Run` alive; release via the returned function on close. `HoldRun` is refcounted; multiple concurrent holds compose; `release` is idempotent and the cleanup drain on Run end catches any that leaked.

## Keeping docs in lockstep

Eight artifacts must stay aligned whenever the script/binding/feature surface changes:

- `MANUAL.md` — long-form reference; covers the library API, CLI, the fifteen reserved script globals, the `server` namespace (§6), goja built-ins, eventloop additions.
- `MANUAL.pdf` — regenerated from `MANUAL.md` via `make manual` (which calls `recon --md-to-pdf`). It's typst-rendered: a recon-native **IBM Plex Sans** body (no vendored font), a `--cover` title page, page-number footer, and a page-numbered TOC, with the markdown piped through `scripts/typst-safe.awk` at render (escapes prose angle brackets + strips HTML comments outside code). Run this whenever `MANUAL.md` changes and include the resulting `MANUAL.pdf` in the same commit. Release cuts (which bump the version strings in `MANUAL.md` via `make release-prep`) need a `make manual` pass too — the `make release-prep` next-step checklist calls this out explicitly.
- `cmd/sercon/help.go::showHelp` — the `--help` / `-h` screen. Flags table must mirror the actual flags defined in `main.go`.
- `cmd/sercon/help.go::showExamples` — the `--examples` walkthrough. The `exampleCount` constant must equal the number of `header(N, …)` calls.
- `examples/scripts/` — runnable `.ts` (or `.tsx`) demo files. **Any change to a user-visible binding, flag, or script-facing behaviour requires updating or adding the relevant example here.** Verify with `make demo`, which runs every success-path script (and skips `hang.ts`, the intentional timeout demo). New example files must also be added to `DEMO_SCRIPTS` in the `Makefile` and the table in `examples/README.md`.
- `examples/scripts/sercon.d.ts` — auto-generated TypeScript declaration file mirroring the CLI's reserved-global surface (`runtime`, `crypto`, `text`, `codec`, `fs`, `net`, `db`, `cloud`, `services`, `tui`, `image`, `web`, `audio`, `server`, `mcp`) plus `console`. Regenerate via `make types` whenever the CLI binding set or the d.ts emitter changes. Tracked in git so editor autocomplete and PR reviewers see the surface without running the binary.
- `MANUAL.md § 17. Binding reference` — a GENERATED section between `<!-- BEGIN/END GENERATED REFERENCE -->` markers. Do NOT hand-edit it; run `make reference` (builds sercon, runs `--emit-reference`, splices via awk — idempotent). `make manual` runs `make reference` first. The reference headings are hierarchically numbered (`### 17.N <namespace>`, `#### 17.N.M <member>`) by `Engine.WriteReferenceNumbered`; the chapter prefix `"17"` is passed from `cmd/sercon/main.go` — if the manual ever renumbers chapters, update that literal to match (the rest of the manual's heading numbers are hand-maintained, since `recon --md-to-pdf` has no auto-numbering). The reference is generated from the structured `scriptengine.MemberDoc` docs in `cmd/sercon/docs.go`; richer per-function docs come from filling each member's `Params`/`ReturnType`/`Returns`/`Errors`/`Example` (which also upgrades the d.ts signature + `@param`). All fifteen reserved-global namespaces (including `web`) + `console` are migrated to the structured model; keep new bindings structured (fill Params/ReturnType/Returns/Errors/Example) so the d.ts + reference stay complete. `cmd/sercon/docs_completeness_test.go` enforces this: `TestDocsComplete` requires every member of each reserved-global namespace (+ `console` + `server`) to have a non-empty Summary/Params/ReturnType/Returns/Errors/Example, and `TestDocsComplete_CoversAllNamespaces` fails if a namespace is left unswept.
- `CHANGELOG.md` — every user-visible change lands here under `## [Unreleased]` (or the active version section) per Keep a Changelog.

Whenever you add a flag: update the flag block in `main.go`, the `FLAGS` section in `showHelp`, mention it in `MANUAL.md § CLI`, add a CHANGELOG entry. Whenever you add a script-side binding: update `showExamples` (and bump `exampleCount`), add the signature to `MANUAL.md § Reserved globals`, add a one-line JSDoc summary to `cmd/sercon/docs.go` so the emitted `sercon.d.ts` grows readable editor hover, run `make types` to refresh `examples/scripts/sercon.d.ts` (and the golden if it touches `TestWriteTypes_Golden`), add or update an example file under `examples/scripts/`, run `make demo` to confirm it passes, add a CHANGELOG entry.

Pure library-side changes (e.g. `WithScriptRoot`, `Engine.Reset()`) only need `MANUAL.md` + `CHANGELOG.md`; they aren't reachable from a `.ts` script, so the example scripts don't need to grow for them.

**Manual recipe convention (cookbook sections).** When thickening a manual section with task guidance, add it as **per-section subsections**, not a central cookbook: an optional `#### N.N Concepts` primer (prose + a small table, only where a domain model needs explaining) followed by a `#### N.N Recipes` subsection. Each recipe is a `#####` heading **phrased as a goal** (e.g. "Filter a column by lane", not the API name), a 1–2 sentence framing, one self-contained fenced `ts` snippet using the real binding/library API with realistic placeholder ids, and optional **Notes** bullets for gotchas. Recipe snippets are **illustrative** (not run by `make demo`) unless explicitly marked runnable; cross-link the generated §17 reference for full signatures rather than restating them. The `favro` section (§16.2.5–16.2.6) is the reference exemplar.

Version bumps: **manual via `make release-prep`** as of v0.7.0 (previously release-please drove this in CI from v0.4.21 to v0.6.0; the workflow was dropped because its state desynced with manual tags we cut while debugging an unrelated Actions permission issue, and re-anchoring it was more work than running the cut by hand). The recipe: `make release-prep VERSION=x.y.z` bumps `pkg/scriptengine/version.go`, the two MANUAL.md version strings (located via `x-release-please-version` end-of-line comments — kept for future automation), and the `HISTORY.md` "covered … through vX.Y.Z (date)" span line (so the span ships in the cut commit instead of perpetually trailing a release; capability narrative is still added per-feature by hand), then prints a checklist (edit CHANGELOG `[Unreleased]` → versioned section, `make manual && make types && make test && make vet && make lint && make demo`, commit, tag, push). `release.yml` fires on the `vX.Y.Z` tag push and runs goreleaser. `make version-check` is the standalone sanity check. `--version` reads `scriptengine.Version`, so it follows the constant automatically — goja / esbuild versions in the same output come from `runtime/debug.ReadBuildInfo` and update with `go.mod`.

## WISHLIST.md inbox → OUT-OF-SCOPE.md

`WISHLIST.md` is the maintainer's scratch inbox for backlog ideas — a place to jot
things without hand-editing `OUT-OF-SCOPE.md`. **When there's nothing else to do
(idle moments, end of a task), check `WISHLIST.md`.** If it has any content beyond
its scaffold header:

1. For each idea, move it into the matching `##` section of `OUT-OF-SCOPE.md`
   (e.g. *Encoding / decoding / barcodes*, *Databases*, *Networking — servers*,
   *External-CLI fallbacks*, *Tracked code follow-ups*; create a new `##` section
   only if none fits), **rewritten to that file's terse style**: a bolded lead,
   one-or-two-sentence description, and a **Reason:** it's parked. Honour
   OUT-OF-SCOPE's entry rule (must have a viable pure-Go or feature-detected
   external-CLI path; never park a cgo-only capability).
2. Reset `WISHLIST.md` to just its scaffold (heading + instruction + the
   `<!-- Write ideas below this line -->` marker) — that scaffold-only state means
   "inbox empty".
3. Don't invent detail the maintainer didn't write; if an idea is too vague to
   place or to assign a Reason, leave it in `WISHLIST.md` and flag it rather than
   guessing. Confirm before moving anything ambiguous.

This is housekeeping, not a release artifact — a normal `docs:` commit; no version
bump, no docs-lockstep obligations.

## CI and release flow

- **`.github/workflows/ci.yml`** runs on every push and PR. Matrix: Go 1.25 + latest stable, on ubuntu-latest and macos-latest. Each job runs `go build` (slim flags), `go vet`, `go test ./...`, and the offline subset of `examples/scripts/*` (excludes network-dependent demos — covered locally via `make demo`). A separate `lint` job runs `golangci-lint` v2.12.2 (pinned to match `make lint`'s fallback).
- **`.github/workflows/release.yml`** fires on `v*.*.*` tag push (cut manually by the maintainer via `make release-prep` + `git tag` + `git push`). Calls goreleaser with `.goreleaser.yml`, which cross-compiles darwin-{amd64,arm64} / linux-{amd64,arm64} / windows-amd64, mirrors `make release`'s `-trimpath -ldflags='-s -w'`, and uploads tarballs/zip + checksums to the auto-created GitHub Release. Each archive bundles LICENSE, README, CHANGELOG, MANUAL.md, and MANUAL.pdf. The release workflow does **not** touch Homebrew (a CI job there can't write to a second repo without a PAT, and releases are cut by hand anyway).
- **Verify the release published.** After pushing the tag, run `make release-verify VERSION=x.y.z` (step 6 of the `make release-prep` checklist). goreleaser creates the GitHub Release asynchronously (~2 min to cross-compile + upload); this polls `gh release view vX.Y.Z` (via `RELEASE_REPO`, default `codedeviate/sercon`) until the release is published, not a draft, and carries its `checksums.txt` asset — i.e. the goreleaser job actually succeeded — failing after ~10 min so a silently-failed build is caught instead of assumed. Note the Homebrew formula is source-based and `bump.sh` fetches the auto-generated `/archive/refs/tags/` tarball (present the instant the tag is pushed, independent of the Release), so `homebrew-bump` would "work" even if goreleaser failed — which is exactly why this explicit verify step exists between the tag push and the bump.
- **Homebrew bump is a local release step.** After the release is verified, run `make homebrew-bump VERSION=x.y.z` (step 7 of the `make release-prep` checklist). It runs the `codedeviate/homebrew-cli` tap's own `scripts/bump.sh sercon <version>` (which fetches the release tarball to recompute its sha256 and rewrites `Formula/sercon.rb`), then commits + pushes the formula. Tap location is the `HOMEBREW_TAP` Makefile variable (default `/Users/thomas/Development/Thomas/Rust/homebrew-cli`); its origin uses the `github-codedv8` SSH host alias, so the push carries the codedeviate identity with no `gh auth switch`. Must run **after** the tag is pushed (bump.sh needs the tag reachable + the repo public); idempotent (re-running an already-bumped version commits nothing). The formula stays a source formula (`brew install codedeviate/cli/sercon`, macOS + Linux); we did not switch to goreleaser/casks. (Previously a CI `homebrew` job did this, gated on a `HOMEBREW_TAP_GITHUB_TOKEN` PAT secret that was never set, so it always skipped — removed in favour of this local step.)
- Tags pushed before `v0.4.5` won't have triggered any release workflow — the YAML wasn't in those commits. That's by design; their existing source-only releases stay intact.

## Versioning and commits

This repo follows **Semantic Versioning** (semver.org): `MAJOR.MINOR.PATCH`.

- **MAJOR** — incompatible changes to the `pkg/scriptengine` exported API or to the `sercon` CLI flags/exit semantics.
- **MINOR** — new bindings, new registration kinds, new flags, additive d.ts emitter coverage. Backwards-compatible.
- **PATCH** — bug fixes, doc updates, internal refactors with no API impact.

Tag releases as `vX.Y.Z` on the matching commit. Pre-1.0, breaking changes in MINOR are tolerated but should still be called out in the commit body.

Commit messages follow **Conventional Commits** (conventionalcommits.org). Header: `<type>(<scope>): <subject>`.

- Types: `feat`, `fix`, `perf`, `refactor`, `docs`, `test`, `build`, `chore`, `ci`, `revert`.
- Scopes used in this repo: `scriptengine`, `sercon`, `transpile`, `require`, `dts`, `eventloop`, `examples`, `docs`. Keep scopes lowercase and singular.
- Breaking changes: add `!` after the scope (`feat(scriptengine)!: ...`) **and** include a `BREAKING CHANGE:` footer describing the migration. This is what drives the MAJOR bump.
- Subject is imperative, lowercase, no trailing period; soft cap 72 chars.
- Body explains the *why* and any non-obvious mechanics (e.g. "esbuild forbids TLA in cjs so we …"). Wrap at ~72 chars.
- Multiple logical changes → multiple commits. Don't bundle a `fix` and a `feat` into one commit.

Example:

```
feat(scriptengine): expose RegisterConstructor for Go-defined classes

Lets host code surface `new Foo(...)` to scripts. The d.ts emitter
already had a branch for this kind; this just wires the public API
through to it.
```
