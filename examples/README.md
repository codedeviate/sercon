# sercon examples

Run the bundled example scripts:

```
make demo
```

Or pick individual scripts:

```
./sercon examples/scripts/smoke.ts examples/scripts/async.ts
```

## What's here

| Script | Demonstrates |
|---|---|
| `smoke.ts` | `api.log` and `api.assert.*` |
| `async.ts` | `api.http.get`, `api.time.sleep`, top-level `await`, `import` from a sibling helper |
| `hash.ts` | `api.hash.*` — all nine algorithms |
| `strings.ts` | `api.str.*` — trim/pad/reverse/strip/encode/sprintf |
| `path-and-time.ts` | `api.path.*` and `api.time.format` (strftime tokens, IANA zones) |
| `default-export.ts` | ESM `export default` interop via the entry rewriter |
| `tsx-demo.ts` | `.tsx` module loading with a local `@jsx h` factory |
| `json-import.ts` | `import data from "./data.json"` and `require("./data.json")` |
| `pkg-resolution.ts` | `package.json` `source` field preferred over `main` |
| `net-probe.ts` | `api.net.tcp/dns/tls/ntp/whois` — hits the real network (not in CI). |
| `email-auth.ts` | `api.email.spf/dmarc/mtaSts/tlsRpt/bimi/all` — hits real DNS (not in CI). |
| `compression.ts` | `api.compression.compress/decompress` round-trip across all 9 algos. |
| `barcode.ts` | `api.barcode.encode` over all 10 supported symbologies. |
| `hang.ts` | Timeout demo; intentionally non-zero exit. Run separately. |

Helpers under `helpers/` are sibling-imported by the above; they aren't
runnable on their own.

## Adding a new binding

Bindings are wired in `cmd/sercon/main.go` inside `registerExampleAPI`. The
example surface is a single namespace (`api`) registered via
`RegisterNamespaceFactory` so each `Run` gets its own VM + event loop in
scope when constructing the bindings.

To add a synchronous binding, add an entry to the members map:

```go
"upper": func(s string) string { return strings.ToUpper(s) },
```

To add an I/O / async binding, use `PromisifyAsync` so the work runs in a
goroutine and the resolution is scheduled back onto the event loop:

```go
"sleep": scriptengine.PromisifyAsync(vm, loop,
    func(ctx context.Context, call goja.FunctionCall) (any, error) {
        time.Sleep(time.Duration(call.Argument(0).ToInteger()) * time.Millisecond)
        return nil, nil
    }),
```

## Regenerating `api.d.ts`

After changing bindings, emit a fresh declaration file:

```
go run ./cmd/sercon -emit-dts examples/scripts/api.d.ts
```

Editors that pick up sibling `.d.ts` files (VS Code with the TS plugin, for
example) will then show types and signatures for the `api.*` calls inside
the example scripts.
