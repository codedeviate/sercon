# tsrun / scriptengine

Embeddable TypeScript script engine in Go. Pure Go (no cgo), no Node.

- Library: `pkg/scriptengine` — embed in your own Go program and register Go-callable bindings.
- CLI: `cmd/tsrun` — runs `.ts` files with a small example `api` surface.

Runtime: [goja](https://github.com/dop251/goja). TypeScript is transpiled
in-process with [esbuild](https://github.com/evanw/esbuild/tree/main/pkg/api).
Promises, `setTimeout`, and `require` come from [goja_nodejs](https://github.com/dop251/goja_nodejs).

## Quickstart

```
go build -o tsrun ./cmd/tsrun

cat > hello.ts <<'EOF'
api.log("hello", await api.http.get("https://example.com").then(r => r.status));
EOF

./tsrun hello.ts
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
./tsrun -emit-dts api.d.ts
```
