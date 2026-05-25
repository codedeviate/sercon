# Claude Code prompt: Embeddable TypeScript script engine in Go

Copy everything below the `---` into Claude Code as the initial prompt. Adjust the module path, Go version, and example bindings to taste before sending.

---

## Goal

Build a Go-based scripting engine that runs **TypeScript** scripts against a host-provided API. The engine is delivered as two things:

1. A reusable **Go library** (`pkg/scriptengine`) that other Go code embeds to register bindings and execute scripts.
2. A thin **CLI** (`cmd/tsrun`) that wraps the library: takes one or more `.ts` files and runs them with a small built-in example binding surface.

Scripts are used for ad-hoc testing — they need to call into Go-exposed functions and objects, await async work, share helpers via `require`, and be killable on timeout.

## Locked-in stack choices (do not substitute)

- **Runtime**: `github.com/dop251/goja` (pure Go JS engine, ES5.1 + most ES6).
- **TS → JS transpile**: `github.com/evanw/esbuild/pkg/api` used as a Go library (`api.Transform` with `Loader: api.LoaderTS`). No external `tsc`, no node, no cgo.
- **Event loop / Promises / `setTimeout`**: `github.com/dop251/goja_nodejs/eventloop`.
- **CommonJS `require`**: `github.com/dop251/goja_nodejs/require` with a custom source loader so `.ts` files are transpiled on demand.
- **Console**: `github.com/dop251/goja_nodejs/console` wired through the require registry.

Pure Go, no cgo. Target **Go 1.22+**.

## Project layout

```
.
├── go.mod
├── cmd/
│   └── tsrun/
│       └── main.go              # CLI entry point
├── pkg/
│   └── scriptengine/
│       ├── engine.go            # Engine type, lifecycle, Run/RunFile
│       ├── bindings.go          # Helpers for registering Go funcs/objects + namespaces
│       ├── transpile.go         # esbuild wrapper, source-map-aware error mapping
│       ├── require.go           # Custom source loader (resolves .ts and .js)
│       ├── timeout.go           # Interrupt-based timeout wrapper
│       ├── dts.go               # .d.ts generation from registered bindings (reflection)
│       └── engine_test.go       # Tests covering each feature
├── examples/
│   ├── scripts/
│   │   ├── smoke.ts             # Uses the default example bindings
│   │   ├── async.ts             # Demonstrates Promises / await
│   │   └── helpers/
│   │       ├── fixtures.ts      # Imported via require/import
│   │       └── assert.ts
│   └── README.md
└── README.md
```

## Library API surface (`pkg/scriptengine`)

Aim for an API along these lines — exact names can be refined, but the shape should match:

```go
type Engine struct { /* unexported */ }

type Options struct {
    Timeout       time.Duration // per-script wall clock; 0 = no timeout
    ScriptRoot    string        // base dir for require() resolution
    EnableConsole bool          // default true
}

func New(opts Options) *Engine

// Registration — must be safe to call before Run, not during.
func (e *Engine) Register(name string, value any) error
func (e *Engine) RegisterNamespace(name string, members map[string]any) error
func (e *Engine) RegisterConstructor(name string, ctor any) error

// Execution
func (e *Engine) Run(ctx context.Context, name string, source string) (goja.Value, error)
func (e *Engine) RunFile(ctx context.Context, path string) (goja.Value, error)

// .d.ts emission for the registered surface
func (e *Engine) WriteTypes(w io.Writer) error
```

Notes on behavior:

- `Run`/`RunFile` must respect both the `ctx` passed in **and** the `Options.Timeout` — whichever fires first wins. Cancellation must use `vm.Interrupt(...)` from a separate goroutine so a `while(true){}` cannot hang the host.
- The Goja runtime must use `goja.TagFieldNameMapper("json", true)` so JSON struct tags control JS-visible names and unexported fields stay hidden.
- All registrations are applied to the runtime when `Run` is called (or once, lazily, the first time). Re-running a script must not leak state from a previous run — give each `Run` a fresh `eventloop.EventLoop` and a fresh `goja.Runtime`, but reuse the compiled programs for shared helpers (see require caching below).

## CLI behavior (`cmd/tsrun`)

```
tsrun [flags] <script.ts> [script.ts ...]

Flags:
  -timeout duration   Per-script timeout (default 10s)
  -root string        Script root for require resolution (default: dirname of first script)
  -emit-dts string    Write the .d.ts for the example bindings to this path and exit
  -v                  Verbose (log timing, transpile output on error)
```

Exit code is non-zero if any script throws or times out. Print a clear summary line per script: `PASS scripts/smoke.ts (123ms)` or `FAIL scripts/async.ts: <error message at script.ts:line:col>`.

The CLI registers a small **example binding surface** so `tsrun` is useful out of the box. Bindings (all under a single `api` namespace except `console`):

- `api.log(...args)` — alias of `console.log`.
- `api.assert.equal(actual, expected, msg?)` — throws on mismatch.
- `api.assert.ok(cond, msg?)`
- `api.http.get(url): Promise<{ status: number; body: string }>`
- `api.http.post(url, body): Promise<{ status: number; body: string }>`
- `api.time.nowMs(): number`
- `api.time.sleep(ms): Promise<void>`
- `api.env.get(name): string | undefined`

`api.http.*` should be backed by `net/http` with a 5s default timeout and return real Promises through the event loop. Do not block the goroutine — use the eventloop's `RunOnLoop` callback pattern.

## Feature requirements (all four are required)

### 1. Async / Promises via eventloop

Use `goja_nodejs/eventloop`. The `Engine.Run` flow is:

1. Create `loop := eventloop.NewEventLoop(eventloop.WithRegistry(reg))` where `reg` is the require registry.
2. `loop.Start()`.
3. Submit script execution via `loop.RunOnLoop(func(vm *goja.Runtime) { ... compile + run ... })`.
4. Wait for completion using a `done` channel signaled by the callback (capture both the result value and any error).
5. `loop.Stop()` in a deferred call.

Go functions exposed to scripts that perform I/O must return a Promise. Pattern:

```go
func httpGet(loop *eventloop.EventLoop) func(call goja.FunctionCall) goja.Value {
    return func(call goja.FunctionCall) goja.Value {
        vm := call.Runtime()
        url := call.Argument(0).String()
        promise, resolve, reject := vm.NewPromise()
        go func() {
            resp, err := http.Get(url)
            loop.RunOnLoop(func(vm *goja.Runtime) {
                if err != nil { reject(vm.ToValue(err.Error())); return }
                defer resp.Body.Close()
                body, _ := io.ReadAll(resp.Body)
                resolve(vm.ToValue(map[string]any{
                    "status": resp.StatusCode,
                    "body":   string(body),
                }))
            })
        }()
        return vm.ToValue(promise)
    }
}
```

Document this pattern in `bindings.go` with a helper, e.g. `PromisifyAsync(loop, func(ctx) (any, error)) func(goja.FunctionCall) goja.Value`, and use it for all I/O bindings.

### 2. CommonJS `require` for shared helpers

Use `goja_nodejs/require` with a custom `SourceLoader`. The loader must:

- Resolve relative paths against `Options.ScriptRoot` (and `__dirname` of the importing module if available).
- Resolve both `./helpers/fixtures` and `./helpers/fixtures.ts` (and `.js`) — implement the same extension fallback Node uses (`""`, `.ts`, `.js`, `/index.ts`, `/index.js`).
- For `.ts` files, transpile via esbuild before handing source to the registry.
- Cache compiled `*goja.Program` per absolute path so repeated requires within a single `Run` don't retranspile. The cache lives on the `Engine`, not on the runtime, but loaded module *exports* must be per-runtime (Node semantics).

Scripts should be able to write either `const { foo } = require('./helpers/fixtures')` **or** ES-module-style `import { foo } from './helpers/fixtures'` — esbuild will rewrite the import to CommonJS at transpile time (set `Format: api.FormatCommonJS` in transform options). Confirm this works for the example scripts.

### 3. Timeouts via `vm.Interrupt`

`timeout.go` exposes a helper used by `Engine.Run`:

```go
func WithInterrupt(ctx context.Context, timeout time.Duration, vm *goja.Runtime, fn func() error) error
```

It launches a goroutine that watches both `ctx.Done()` and a `time.After(timeout)` channel, and calls `vm.Interrupt("script timeout")` on either. The returned error distinguishes timeout (`ErrScriptTimeout`) from context cancellation (`ctx.Err()`) from script errors. The goroutine must always exit (use a `stop` channel closed in defer) so timeout watchers don't accumulate.

Test: a script containing `while (true) {}` must terminate within `timeout + 100ms` and return `ErrScriptTimeout`. Add a test that asserts this with `Timeout: 200 * time.Millisecond`.

### 4. `.d.ts` generation from registered bindings

`dts.go` walks the registered names and produces a TypeScript declaration file. Approach:

- For each registration, capture the Go value via `reflect`.
- For `reflect.Func`, emit `declare function name(arg0: T0, arg1: T1, ...): TReturn;` — map Go types to TS types via a small mapping table: `string→string`, numeric→`number`, `bool→boolean`, `[]T→T[]`, `map[string]T→Record<string, T>`, `any/interface{}→unknown`, `error` returns become `throws` (omit from return type; if function returns `(T, error)`, the TS return is `T`).
- For struct values, emit `declare const name: { methodA(...): ...; methodB(...): ...; }` enumerating exported methods (use the JSON tag mapper for naming consistency).
- For `RegisterNamespace`, emit a single `declare const name: { ... }` with all members.
- For `RegisterConstructor`, emit `declare class Name { constructor(...); }` plus exported methods of the returned type.
- Functions that return `*goja.Promise` (detected by Go return type or a marker) should become `Promise<T>` in TS. Provide a `Promised[T any]` marker type the host can use to disambiguate when the Go signature is `func(...) goja.Value`.
- Unknown types fall back to `unknown` with a `// TODO:` comment, not a panic.

The CLI's `-emit-dts` flag writes the result so users can drop it next to their scripts as `api.d.ts` for editor autocomplete.

## Example scripts that MUST pass

`examples/scripts/smoke.ts`:

```typescript
api.log("hello from ts");
api.assert.equal(1 + 1, 2);
api.assert.ok(api.time.nowMs() > 0);
```

`examples/scripts/async.ts`:

```typescript
import { check } from "./helpers/assert";

const r = await api.http.get("https://example.com");
check(r.status === 200, `expected 200, got ${r.status}`);
api.log("body length:", r.body.length);
await api.time.sleep(50);
api.log("done");
```

`examples/scripts/helpers/assert.ts`:

```typescript
export function check(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}
```

Both example scripts must run via `tsrun examples/scripts/smoke.ts examples/scripts/async.ts` and print PASS lines.

## Constraints and conventions

- Pure Go, no cgo. `CGO_ENABLED=0 go build ./...` must succeed.
- No global state in `pkg/scriptengine` — everything hangs off the `Engine`.
- Errors returned from Go bindings (second return value of type `error`) must surface as thrown JS exceptions with the original message.
- Use `context.Context` everywhere a long operation could block. Don't store contexts on structs.
- Standard project hygiene: `go vet ./...` clean, `gofmt -s` clean. If you add `golangci-lint`, keep it minimal.
- README at repo root: 30-line quickstart, no marketing fluff. README in `examples/` explains how to add a new binding and regenerate `.d.ts`.

## Tests (required, not optional)

`pkg/scriptengine/engine_test.go` covers:

1. Simple sync script returning a value.
2. A binding that returns `(T, error)` — the error path throws in JS and is catchable.
3. Promise from Go resolves and the script sees the resolved value via `await`.
4. Promise from Go rejects and `try/catch` in the script catches it.
5. `require('./helper')` resolves a sibling `.ts` file and shares state correctly.
6. `import { x } from './helper'` works (esbuild rewrites to CommonJS).
7. `while(true){}` is interrupted within timeout + 100ms.
8. `ctx` cancellation from the host interrupts the script.
9. `.d.ts` output for a small fixture binding set matches a golden file (use `-update` flag pattern to regenerate).
10. Running two scripts in sequence on the same `Engine` does not leak globals between them.

Use `testing.T` and table-driven tests where it makes sense. No external test frameworks.

## Deliverable checklist

When you're done, the following must all be true:

- [ ] `go build ./...` with `CGO_ENABLED=0` succeeds.
- [ ] `go test ./...` is green.
- [ ] `go vet ./...` is clean.
- [ ] `tsrun examples/scripts/smoke.ts examples/scripts/async.ts` prints two PASS lines.
- [ ] `tsrun -emit-dts /tmp/api.d.ts` writes a syntactically valid `.d.ts` (verify with `npx -y typescript@5 tsc --noEmit /tmp/api.d.ts` if Node is around, otherwise eyeball it).
- [ ] `tsrun -timeout 200ms examples/scripts/hang.ts` (a one-liner `while(true){}`) exits non-zero within ~300ms.

## How to work

1. Sketch the package skeleton and `go.mod` first. Commit.
2. Get the simplest sync `Run` working end-to-end before adding the event loop. Commit.
3. Wire esbuild transpile. Commit.
4. Add the event loop and rework `Run` around it. Commit.
5. Add `require` with the source loader and TS extension fallback. Commit.
6. Add interrupt/timeout. Commit.
7. Add `.d.ts` generation. Commit.
8. Write the example scripts and the CLI. Commit.
9. Write tests. Commit.
10. Polish READMEs.

Keep commits small and self-contained. If you hit a design choice that isn't covered above, prefer the option that keeps the host API smaller and the script-visible API closer to standard JS/TS.
