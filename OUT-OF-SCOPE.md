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
- **Release automation.** No tagged release yet; once one ships, a
  release-please or similar step keyed off Conventional Commits would
  match the conventions in `CLAUDE.md`.
