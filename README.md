# sercon

[![GitHub](https://img.shields.io/badge/github-codedeviate%2Fsercon-181717?logo=github)](https://github.com/codedeviate/sercon)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue?logo=opensourceinitiative)](LICENSE)
[![Go 1.25+](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go)](https://go.dev)
<br/>
[![Latest release](https://img.shields.io/github/v/release/codedeviate/sercon?logo=semanticrelease&label=release&color=blue)](https://github.com/codedeviate/sercon/releases)
[![pkg.go.dev](https://img.shields.io/badge/pkg.go.dev-scriptengine-007d9c?logo=go)](https://pkg.go.dev/github.com/codedeviate/sercon/pkg/scriptengine)
[![Homebrew](https://img.shields.io/badge/homebrew-codedeviate%2Fcli%2Fsercon-fbb040?logo=homebrew)](https://github.com/codedeviate/homebrew-cli)


`sercon` is a CLI tool for running TypeScript scripts for reconnaissance,
troubleshooting, and testing. Write a short `.ts` file, hand it to `sercon`,
and probe a service, inspect an endpoint, reproduce a bug, or script a quick
check — without spinning up a Node project or pulling in a dependency tree.
A small built-in `api` surface gives scripts HTTP, shell exec, logging, and
more, and the whole thing ships as a single static binary. Pure Go (no cgo),
no Node.

Under the hood it's an embeddable TypeScript script engine, usable two ways:

- CLI: `cmd/sercon` — runs `.ts` files against the built-in `api` surface.
  Reach for it when you need a repeatable, scriptable alternative to a pile
  of ad-hoc `curl`/`jq`/shell one-liners for recon, debugging, and test checks.
  Available via the `codedeviate/cli` Homebrew tap: `brew install codedeviate/cli/sercon`.
- Library: `pkg/scriptengine` — embed in your own Go program and register
  Go-callable bindings to expose a safe, sandboxed scripting surface.

Runtime: [goja](https://github.com/dop251/goja). TypeScript is transpiled
in-process with [esbuild](https://github.com/evanw/esbuild/tree/main/pkg/api).
Promises, `setTimeout`, and `require` come from [goja_nodejs](https://github.com/dop251/goja_nodejs).

## Quickstart

```
go build -o sercon ./cmd/sercon

cat > hello.ts <<'EOF'
api.log("hello", await api.http.get("https://example.com").then(r => r.status));
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

Generate a `.d.ts` for editor autocomplete:

```
./sercon -emit-dts api.d.ts
```
