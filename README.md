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


`sercon` runs TypeScript scripts for reconnaissance, troubleshooting, and
testing. Write a short `.ts` file, hand it to `sercon`, and probe a service,
inspect an endpoint, reproduce a bug, drive a browser, query a database, or
stand up a quick server — without spinning up a Node project or pulling in a
dependency tree.

Scripts get **fifteen** always-present top-level globals covering HTTP and
networking, crypto, databases, files, encoding/decoding, image and audio,
web scraping, cloud provider APIs, terminal UIs, inbound servers, external-CLI
wrappers, and the Model Context Protocol (both a server and a client). No
`import`, no `npm install` — the batteries are built in, and the whole thing
ships as a single static binary. Pure Go (no cgo), no Node.

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

## What's in the box

Fifteen reserved globals (use them directly — no namespace import). Full
signatures live in `MANUAL.md` §17 and `sercon.d.ts`; `sercon --examples` is a
guided tour.

| Global | What it's for |
|---|---|
| `runtime` | script-host scaffolding: `argv`, `env`, secrets, `log`, `sleep`, assertions |
| `net` | HTTP client (incl. multipart/binary), email send, DNS, TCP/UDP, ping |
| `db` | SQL (`sqlite`/`postgres`/`mysql`/`mssql`/`clickhouse`/`oracle`) + `redis`/`memcached`/`ldap`/`dict` |
| `crypto` | hashing, JWT, age encryption, random bytes |
| `text` | charset decode, string utilities, base64, `jq` |
| `codec` | barcodes, check digits, compression, docs, dotenv, perl/php dumps, spreadsheets, TOML, XML |
| `fs` | file read/write, `mkdir`, `stat`, `exists`, `remove` |
| `image` | decode/encode/transform (resize/crop/rotate/filter/compose), SVG rasterize, WebP |
| `audio` | read/write/convert audio, steganography |
| `web` | fetch + parse: feeds, sitemaps, HTML (CSS/XPath queries) |
| `cloud` | provider APIs — AWS, Azure, Google Cloud (object storage, …) |
| `server` | inbound listeners: HTTP/HTTPS (routes, middleware, WebSocket), SMTP |
| `services` | external-CLI wrappers: `git`, `gh`, `ai`, `agentBrowser`, `webdriver` |
| `tui` | terminal-UI widgets |
| `mcp` | Model Context Protocol — author a server (`mcp.serve`) or drive one as a client (`mcp.connect`), over stdio or Streamable HTTP |

## Documentation

- [`MANUAL.md`](MANUAL.md) — the full reference: CLI, library API, all fifteen
  reserved globals (incl. `mcp`, `cloud`, `web`, `server`), and the generated
  per-binding reference (§17). Also `sercon --help` and `sercon --examples`
  from the command line.
- [`CHANGELOG.md`](CHANGELOG.md) — per-version change log (Keep a Changelog).
- [`HISTORY.md`](HISTORY.md) — thematic capability history: when each subsystem
  arrived and how it grew, from v0.1.0 onward.
- [`OUT-OF-SCOPE.md`](OUT-OF-SCOPE.md) — open backlog: outstanding/parked items,
  each with the reason it's not done yet.

## Quickstart

```
go build -o sercon ./cmd/sercon
# …or: brew install codedeviate/cli/sercon

cat > hello.ts <<'EOF'
const r = await net.http.get("https://example.com");
runtime.log("status", r.status, "bytes", r.body.length);
EOF

./sercon hello.ts
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

## Library usage (unsupported — see above)

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

## Database engines — test status

`sercon` ships SQL clients (`db.sqlite`, `db.postgres`, `db.mysql`,
`db.mssql`, `db.clickhouse`, `db.oracle`) and non-SQL stores (`db.redis`,
`db.memcached`, `db.ldap`, `db.dict`). They share one proven handle shape;
coverage differs by engine:

- **Verified against a real server in CI** (the `integration` workflow spins up
  containers on every push): `db.postgres`, `db.mysql` (MySQL **and** MariaDB),
  `db.clickhouse`, `db.redis` (Redis **and** Valkey), `db.memcached`, `db.ldap`.
- **Verified in-process:** `db.sqlite` (in-memory) and `db.dict` (in-process
  server).
- **Not yet exercised against a live server — use at your own risk:**
  `db.mssql` and `db.oracle`. DSN assembly and connection wiring are
  unit-tested and they follow the same pattern as the verified engines, but
  there's no functional round-trip against the real engine yet — treat them as
  provisional.

This list is updated as engines gain live coverage; feedback on the
provisional ones is welcome.
