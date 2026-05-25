# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Common commands

```bash
make build                                 # debug CLI -> ./sercon
make release                               # slim release CLI (-trimpath -s -w; ~30% smaller)
make manual                                # MANUAL.md -> MANUAL.pdf via `recon --md-to-pdf`
make test                                  # go test ./...
make vet                                   # go vet ./...
make clean                                 # rm -f sercon MANUAL.pdf

CGO_ENABLED=0 go build ./...               # whole repo (must stay cgo-free)
go test ./pkg/scriptengine -run TestRun_PromiseResolveAwait   # single test
go test ./pkg/scriptengine -run TestWriteTypes_Golden -update # refresh golden .d.ts
./sercon examples/scripts/smoke.ts examples/scripts/async.ts  # smoke + async demo
./sercon -emit-dts /tmp/api.d.ts           # emit declaration file for the CLI's api surface
./sercon -timeout 200ms examples/scripts/hang.ts              # timeout demo (exits non-zero ~213ms)
./sercon --help | --examples | --version   # in-depth colourised help / feature tour / version
```

## Architecture

The library (`pkg/scriptengine`) executes TypeScript via goja, with esbuild used in-process as the TS→JS transpiler and `goja_nodejs/eventloop` + `goja_nodejs/require` providing Promises and module loading. There are several non-obvious design choices a reader should know before editing.

### Per-Run runtime, shared registry

`Engine.Run` creates a fresh `eventloop.EventLoop` (and therefore a fresh `goja.Runtime`) every call. Registrations are reapplied per Run inside the loop callback, so no globals or module state leak between runs. The `*require.Registry` is the only piece that's reused across runs — it caches compiled bytecode (`*goja.Program`) per absolute path. Each Run still gets fresh module *exports* because `registry.Enable(vm)` builds a new `RequireModule` with its own `modules` map. Don't move the Registry construction into the Run path or you'll lose the compile cache; don't push the runtime onto the `Engine` or you'll start leaking state.

### Two transpile modes (entry vs required module)

esbuild rejects top-level `await` under `Format: FormatCommonJS`. The required-module path (`transpileTS`) uses `FormatCommonJS` and is straightforward. The entry-script path (`transpileEntry`) emits `FormatESModule`, then `rewriteEntryESMToCJS` line-scans the output, converts `import` statements to `require()` declarations, and wraps the remaining body in `(async () => { ... })().then(__resolve, __reject)`. The engine sets `__resolve` / `__reject` on the VM to capture the top-level Promise settlement. Any changes to entry-script semantics (top-level await, import handling) live in `transpile.go`.

### Keeping the event loop alive across async work

`eventloop.EventLoop.Run` exits as soon as `jobCount` drops to zero. `jobCount` is incremented only by `setTimeout` / `setInterval` / `setImmediate` — `RunOnLoop` does **not** count. So a host Promise-returning binding that does `go func() { ... loop.RunOnLoop(...) }()` will lose the race: the loop returns before the goroutine schedules its callback. `PromisifyAsync` parks a 24-hour `loop.SetTimeout` as a sentinel while async work is in flight and clears it on resolution. If you write a new I/O binding, route it through `PromisifyAsync` or replicate that pattern, otherwise the script will appear to "succeed" silently without running the async tail.

### Require source loader (Node ext-fallback + .ts)

`goja_nodejs/require` only tries appending `.js` / `.json` when resolving paths. `Engine.newSourceLoader` adds `.ts` / `.tsx` extension fallback and index-file resolution, and transpiles `.ts` / `.tsx` sources before handing them to the registry. The registry's compile cache keys by whatever path the resolver passed in, which may have a `.js` suffix even when the on-disk file is `.ts` — that's fine, the cache is correct as long as the path is stable.

### Interrupt + cancel watcher

`Engine.Run` spawns one watcher goroutine that selects on `ctx.Done()`, an optional timeout, and a `done` channel closed after `loop.Run` returns. On firing it calls `vm.Interrupt` (aborts sync JS) and `loop.Terminate` (drains the loop). The watcher must always exit via `done` so it doesn't accumulate. `timeout.go::withInterrupt` exists as a standalone helper but the live cancellation path is the inline watcher in `engine.go`. The two `atomic.Bool` flags (`timedOut`, `canceled`) distinguish `ErrScriptTimeout` from `ctx.Err()` after the fact.

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
- `pkg/scriptengine/timeout.go` — `ErrScriptTimeout`, `withInterrupt` helper.
- `pkg/scriptengine/dts.go` — declaration generator with cycle detection.
- `pkg/scriptengine/engine_test.go` — 11 cases covering the 10 spec-required scenarios; uses `-update` flag for the golden `.d.ts`.
- `cmd/sercon/main.go` — CLI plus the example `api` namespace (registered as a factory).
- `examples/scripts/` — runnable sample scripts; `hang.ts` is the timeout demo and must stay a single `while(true){}`.
- `claude-code-prompt.md` — the original spec for this build. Refer to it before redesigning anything significant.

## Editing rules of thumb

- Don't add cgo. The README and spec both lock this in; `CGO_ENABLED=0 go build ./...` is a deliverable check.
- Don't introduce package-level state in `pkg/scriptengine` — everything hangs off `Engine`.
- Errors returned as the second value of a Go binding surface as thrown JS exceptions automatically (via `vm.NewGoError`). Don't swallow them at the binding layer.
- If you change the registered example surface in `cmd/sercon/main.go`, regenerate the golden in `pkg/scriptengine/testdata/` only if you also touched bindings used by `TestWriteTypes_Golden` (it has its own minimal fixture set, not the CLI's `api`).

## Keeping docs in lockstep

Five artifacts must stay aligned whenever the script/binding/feature surface changes:

- `MANUAL.md` — long-form reference; covers the library API, CLI, script `api`, goja built-ins, eventloop additions.
- `MANUAL.pdf` — regenerated from `MANUAL.md` via `make manual` (which calls `recon --md-to-pdf`). Run this whenever `MANUAL.md` changes and include the resulting `MANUAL.pdf` in the same commit.
- `cmd/sercon/help.go::showHelp` — the `--help` / `-h` screen. Flags table must mirror the actual flags defined in `main.go`.
- `cmd/sercon/help.go::showExamples` — the `--examples` walkthrough. The `exampleCount` constant must equal the number of `header(N, …)` calls.
- `CHANGELOG.md` — every user-visible change lands here under `## [Unreleased]` (or the active version section) per Keep a Changelog.

Whenever you add a flag: update the flag block in `main.go`, the `FLAGS` section in `showHelp`, mention it in `MANUAL.md § CLI`, add a CHANGELOG entry. Whenever you add a script-side binding: update `showExamples` (and bump `exampleCount`), add the signature to `MANUAL.md § Built-in script api`, refresh `.d.ts` documentation (and the golden if it touches `TestWriteTypes_Golden`), add a CHANGELOG entry.

Version bumps: edit `pkg/scriptengine/version.go`, add a new section to `CHANGELOG.md`, then tag `vX.Y.Z`. `--version` reads `scriptengine.Version`, so it follows the constant automatically — goja / esbuild versions in the same output come from `runtime/debug.ReadBuildInfo` and update with `go.mod`.

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
