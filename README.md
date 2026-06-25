# sercon

[![GitHub](https://img.shields.io/badge/github-codedeviate%2Fsercon-181717?logo=github)](https://github.com/codedeviate/sercon)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?logo=opensourceinitiative)](LICENSE)
[![Go 1.25+](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go)](https://go.dev)
[![ci](https://github.com/codedeviate/sercon/actions/workflows/ci.yml/badge.svg)](https://github.com/codedeviate/sercon/actions/workflows/ci.yml)
[![integration](https://github.com/codedeviate/sercon/actions/workflows/integration.yml/badge.svg)](https://github.com/codedeviate/sercon/actions/workflows/integration.yml)
<br/>
[![Latest release](https://img.shields.io/github/v/release/codedeviate/sercon?logo=semanticrelease&label=release&color=blue)](https://github.com/codedeviate/sercon/releases)
[![pkg.go.dev](https://img.shields.io/badge/pkg.go.dev-scriptengine-007d9c?logo=go)](https://pkg.go.dev/github.com/codedeviate/sercon/pkg/scriptengine)
[![Homebrew](https://img.shields.io/badge/homebrew-codedeviate%2Fcli%2Fsercon-fbb040?logo=homebrew)](https://github.com/codedeviate/homebrew-cli)

***Sercon – Reconnaissance, shaped by code***


`sercon` is a CLI tool for running TypeScript scripts for reconnaissance,
troubleshooting, and testing. Write a short `.ts` file, hand it to `sercon`,
and probe a service, inspect an endpoint, reproduce a bug, or script a quick
check — without spinning up a Node project or pulling in a dependency tree.
A small set of built-in globals gives scripts HTTP, shell exec, logging, and
more, and the whole thing ships as a single static binary. Pure Go (no cgo),
no Node.

Under the hood it's a TypeScript script engine in Go:

- CLI: `cmd/sercon` — **the supported product.** Runs `.ts` files against
  the built-in global surface. Reach for it when you need a repeatable,
  scriptable alternative to a pile of ad-hoc `curl`/`jq`/shell one-liners
  for recon, debugging, and test checks. Available via the `codedeviate/cli`
  Homebrew tap: `brew install codedeviate/cli/sercon`.
- Library: `pkg/scriptengine` — the engine the CLI is built on. You *can*
  embed it in your own Go program and register Go-callable bindings, but
  **library use is unsupported**: the package exists to serve the CLI, its
  API may change without notice, and there are no stability or sandboxing
  guarantees. Use it as a library at your own risk.

Runtime: [goja](https://github.com/dop251/goja). TypeScript is transpiled
in-process with [esbuild](https://github.com/evanw/esbuild/tree/main/pkg/api).
Promises, `setTimeout`, and `require` come from [goja_nodejs](https://github.com/dop251/goja_nodejs).

## Documentation

- [`MANUAL.md`](MANUAL.md) — the full reference: library API, CLI, the reserved
  script globals, the `server` namespace, and the generated binding reference
  (§17). Also `sercon --help` and `sercon --examples` from the command line.
- [`CHANGELOG.md`](CHANGELOG.md) — per-version change log (Keep a Changelog).
- [`HISTORY.md`](HISTORY.md) — thematic capability history: when each subsystem
  arrived and how it grew, from v0.1.0 onward.
- [`OUT-OF-SCOPE.md`](OUT-OF-SCOPE.md) — open backlog: outstanding/parked items,
  each with the reason it's not done yet.

## Quickstart

```
go build -o sercon ./cmd/sercon

cat > hello.ts <<'EOF'
log("hello", await http.get("https://example.com").then(r => r.status));
EOF

./sercon hello.ts
```

Library usage:

```go
eng := scriptengine.New(scriptengine.Options{
    Timeout:    5 * time.Second,
    ScriptRoot: "./scripts",
})
eng.Register("greet", func(name string) string { return "hi " + name })
_, err := eng.RunFile(ctx, "./scripts/main.ts")
```

Register an I/O-bound binding that returns a Promise:

```go
eng.RegisterFactory("httpGet", func(vm *goja.Runtime, loop *eventloop.EventLoop) any {
    return scriptengine.PromisifyAsync(vm, loop,
        func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
            // do work, return map / struct / error
        })
})
```

Set up editor autocomplete + hover docs (any TypeScript-aware editor — VSCode,
Zed, Neovim, …) in a script directory with one command:

```
sercon init           # drops sercon.d.ts + jsconfig.json into the current dir
sercon init ./scripts # …or a target dir
```

`sercon init` writes the binding declarations (`sercon.d.ts`) plus a
`jsconfig.json` that points the editor's language server at them — no plugin
needed. To just emit the declarations (e.g. for your own config), use
`./sercon -emit-dts sercon.d.ts`.

## Database engines — test status

`sercon` ships SQL clients (`db.sqlite`, `db.postgres`, `db.mysql`,
`db.mssql`, `db.clickhouse`, `db.oracle`) and a few non-SQL stores
(`db.redis`, `db.memcached`, `db.ldap`, `db.dict`). They share one
proven handle shape, but not all are exercised against a real server in CI:

- **Verified end-to-end:** `db.sqlite` (in-memory), `db.redis` (miniredis),
  `db.memcached` and `db.dict` (in-process stub servers).
- **Not yet verified against a live server — use at your own risk:**
  `db.postgres`, `db.mysql`, `db.mssql`, `db.clickhouse`, `db.oracle`
  (DSN assembly and connection wiring are unit-tested, but there's no
  functional round-trip against the real engine), plus `db.ldap`
  (error paths only). They follow the same pattern as the verified
  engines and *should* work, but haven't been confirmed against the real
  servers — treat them as provisional.

This list is updated as engines are manually verified; feedback on any of
the provisional ones is welcome.
