<div class="cover">
<h1>sercon</h1>
<div class="subtitle">User Manual</div>
<hr>
<div class="version">Version 0.25.0</div> <!-- x-release-please-version -->
<div class="date">2026-05-31</div>
<div class="meta">
Repository · https://github.com/codedeviate/sercon<br>
License · MIT
</div>
</div>

# sercon manual

Long-form reference for the `pkg/scriptengine` library, the `sercon` CLI,
the built-in script API, and the JavaScript runtime surface that
`scriptengine` exposes via [goja] and [goja_nodejs]. Examples are runnable —
save snippets as `.ts` files and execute with `sercon file.ts` unless
otherwise noted.

[goja]: https://github.com/dop251/goja
[goja_nodejs]: https://github.com/dop251/goja_nodejs

---

## Table of contents

1. [Overview](#1-overview)
2. [Quickstart](#2-quickstart)
3. [Library API — `pkg/scriptengine`](#3-library-api--pkgscriptengine)
4. [CLI — `sercon`](#4-cli--sercon)
5. [Reserved globals (script surface)](#5-reserved-globals-script-surface)
6. [Servers](#6-servers)
7. [JavaScript runtime built-ins (goja)](#7-javascript-runtime-built-ins-goja)
8. [Async runtime additions (goja_nodejs)](#8-async-runtime-additions-goja_nodejs)
9. [TypeScript support](#9-typescript-support)
10. [Top-level `await`](#10-top-level-await)
11. [Module resolution](#11-module-resolution)
12. [Timeouts and cancellation](#12-timeouts-and-cancellation)
13. [Error semantics](#13-error-semantics)
14. [Type generation (.d.ts)](#14-type-generation-dts)
15. [Limitations and gotchas](#15-limitations-and-gotchas)

---

## 1. Overview

`sercon` is a Go program that runs TypeScript files. It's built from two
parts:

- `pkg/scriptengine` — a library you embed in your own Go code. You
  register Go-callable bindings, then call `Run` / `RunFile` to execute a
  `.ts` script that talks to them.
- `cmd/sercon` — a thin CLI built on the library, useful for ad-hoc test
  scripts and as a working example of every binding kind.

The runtime is pure Go: [goja] is the JS engine, [esbuild] (used as a Go
library) is the TS→JS transpiler, and [goja_nodejs] provides Promises,
`setTimeout`, `console`, and CommonJS `require`. No cgo, no Node.

[esbuild]: https://github.com/evanw/esbuild/tree/main/pkg/api

## 2. Quickstart

Install:

```bash
go install github.com/codedeviate/sercon/cmd/sercon@latest
```

Run one of the bundled examples:

```bash
sercon examples/scripts/smoke.ts examples/scripts/async.ts
```

…or run them all via `make demo`. `examples/scripts/` ships a runnable
`.ts` (or `.tsx`) file per feature area — `hash.ts`, `strings.ts`,
`path-and-time.ts`, `default-export.ts`, `tsx-demo.ts`, and the
original `smoke.ts` / `async.ts` / `hang.ts` (`hang.ts` is the timeout
demo and intentionally exits non-zero).

Generate a declaration file for editor autocomplete:

```bash
sercon --emit-dts sercon.d.ts
```

Embed in your own Go program:

```go
eng := scriptengine.New(scriptengine.Options{
    Timeout:    5 * time.Second,
    ScriptRoot: "./scripts",
})
eng.Register("upper", strings.ToUpper)
val, err := eng.RunFile(ctx, "scripts/main.ts")
```

## 3. Library API — `pkg/scriptengine`

### `Engine`, `Options`, `New`

```go
type Options struct {
    Timeout        time.Duration // wall-clock limit per Run; 0 disables
    ScriptRoot     string        // base for require/import resolution
    DisableConsole bool          // turn off the goja_nodejs console module
    Verbose        io.Writer     // diagnostic traces, prefixed "[sercon] "
    ModuleLoader   func(candidatePath string) (source string, found bool, err error)
}

func New(opts Options) *Engine
```

`ModuleLoader`, when non-nil, is consulted for every
require/import candidate path **before** the filesystem — the hook
for embedders that want to serve modules from somewhere other than
disk (an in-memory FS, a network source, an embedded bundle).
goja probes several candidates per specifier (`./x`, `./x.ts`,
`./x/index.ts`, …); the engine also tries the bare path plus the
usual extension fallbacks (`.ts` / `.tsx` / `.js` / `.mjs` /
`.cjs` / `.json`) against the loader, so a loader can match on a
plain suffix. Return `(source, true, nil)` to serve the module
(transpiled when the path ends in `.ts` / `.tsx`, exactly like a
disk read); `("", false, nil)` to fall through to the filesystem;
`("", false, err)` to abort resolution.

```go
eng := scriptengine.New(scriptengine.Options{
    ModuleLoader: func(path string) (string, bool, error) {
        if strings.HasSuffix(path, "greeting.ts") {
            return `export const hi = (n: string) => "hi " + n;`, true, nil
        }
        return "", false, nil // fall through to disk
    },
})
```

`Engine` is the host of all registered bindings. Construct it once, then
call `Run`/`RunFile` as many times as needed — each call gets a *fresh*
`*goja.Runtime` so script state never leaks between invocations.

`ScriptRoot` defaults to the working directory at `New` time if left
empty. Both `Timeout` and `ScriptRoot` can be overridden on a per-Run
basis only by constructing a fresh `Engine`; see
[Out-of-scope](./OUT-OF-SCOPE.md) for the per-Run override on the
roadmap.

### Registering bindings

| Function | Use when… |
|---|---|
| `Register(name, value)` | The binding is a pure function, struct, or primitive that doesn't need access to the runtime. |
| `RegisterNamespace(name, members map[string]any)` | You want to expose `name.x`, `name.y` without defining a Go struct. |
| `RegisterConstructor(name, ctor)` | The binding is a Go function whose return value should be treated as a class in the emitted `.d.ts`. (Runtime semantics today are identical to `Register`; the d.ts emitter is the only differentiator.) |
| `RegisterFactory(name, factory func(vm, loop) any)` | The binding needs the per-Run `*goja.Runtime` or `*eventloop.EventLoop` in scope (typical for Promise-returning I/O). |
| `RegisterNamespaceFactory(name, factory)` | Same, but the factory returns the full member map. |

Example — synchronous function binding:

```go
eng.Register("greet", func(name string) string { return "hi " + name })
```

Example — namespace with a sync member:

```go
eng.RegisterNamespace("math2", map[string]any{
    "square": func(n float64) float64 { return n * n },
    "pi":     3.14159,
})
```

Example — async (Promise-returning) binding via `PromisifyAsync`:

```go
eng.RegisterFactory("httpGet", func(vm *goja.Runtime, loop *eventloop.EventLoop) any {
    return scriptengine.PromisifyAsync(vm, loop,
        func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
            url := call.Argument(0).String()
            resp, err := http.Get(url)
            if err != nil {
                return nil, err
            }
            defer resp.Body.Close()
            body, _ := io.ReadAll(resp.Body)
            return map[string]any{"status": resp.StatusCode, "body": string(body)}, nil
        })
})
```

### `Run`, `RunFile`

```go
func (e *Engine) Run(ctx context.Context, name, source string, opts ...RunOption) (goja.Value, error)
func (e *Engine) RunFile(ctx context.Context, path string, opts ...RunOption) (goja.Value, error)
```

`Run` executes `source` as an entry-script TS. `name` is used in stack
traces. The returned value is currently always `undefined` — top-level
expression capture is on the backlog.

Both methods respect `Options.Timeout` *and* `ctx` cancellation, whichever
fires first; the resulting error is either `ErrScriptTimeout`, `ctx.Err()`,
or the underlying JS exception.

#### Per-Run options

```go
type RunOption func(*runConfig)

func WithScriptRoot(dir string) RunOption
```

Reuse a single Engine across many scripts that each live in their own
directory by passing `WithScriptRoot(dir)` to `Run` / `RunFile`. The
override applies only to this call; `Options.ScriptRoot` is used for
every other Run.

```go
_, _ = eng.Run(ctx, "main.ts", source, scriptengine.WithScriptRoot("/path/to/run42"))
```

```go
func WithArgs(args []string) RunOption
```

Set the per-script argument vector. The script reads it as
`runtime.argv`, a global with Node/Bun layout: `argv[0]` is
`Options.ProgramName` (defaults to `filepath.Base(os.Args[0])`;
the CLI sets it to `"sercon"`), `argv[1]` is the running script's
path, and the `WithArgs` values follow from index 2. The global is
always present — with no args it is just `[programName, scriptPath]`.

```go
_, _ = eng.Run(ctx, "main.ts", source, scriptengine.WithArgs([]string{"--port", "8080"}))
// inside the script: runtime.argv === [ProgramName, "/abs/main.ts", "--port", "8080"]
```

### `Reset`

```go
func (e *Engine) Reset()
```

Clears every registered binding. Useful when a long-lived Engine is
reused across unrelated batches of scripts that each want a clean
global namespace. **Not safe to call concurrently with Run / RunFile.**

### `WriteTypes`

```go
func (e *Engine) WriteTypes(w io.Writer) error
```

Emits a `.d.ts` describing the registered surface. See section
[14. Type generation](#14-type-generation-dts) for what the mapping
looks like.

### `SetDocs` and `SetMemberDocs`

```go
func (e *Engine) SetDocs(path, doc string)
func (e *Engine) SetMemberDocs(namespace string, docs map[string]string)
```

Attach JSDoc strings to registered bindings so the emitted `.d.ts`
grows readable editor hover. `SetDocs` takes a dotted path —
either a bare top-level name (`"greet"`, `"http"`) or
`"namespace.member"` for namespace members (`"http.get"`,
`"exec.shell"`); `SetMemberDocs` is the convenience for documenting
many members of a namespace at once (the namespace itself can still
be documented separately via `SetDocs`).

Multi-line docs are written as `\n`-separated strings; the emitter
expands them into a standard `* `-prefixed JSDoc block.
Single-line docs collapse to `/** … */`. Calling `SetDocs` with an
empty string removes any previous doc for that path. Bindings
without a doc entry get no JSDoc block at all — the emitter doesn't
insert empty `/** */` placeholders.

`SetDocs` may be called before or after the matching `Register…`
call; docs are looked up by name at emit time. Like the
registration methods, `SetDocs` must not race with a `Run` /
`WriteTypes` / `Reset` on the same engine.

```go
eng.Register("greet", func(name string) string { return "hi " + name })
eng.SetDocs("greet", "Greet someone by name.")

eng.RegisterNamespace("math2", map[string]any{
    "pi":      3.14159,
    "squared": func(n float64) float64 { return n * n },
})
eng.SetDocs("math2", "Tiny math helpers.")
eng.SetMemberDocs("math2", map[string]string{
    "pi":      "Circumference / diameter ratio.",
    "squared": "Multiply a number by itself.",
})
```

### `PromisifyAsync[T]` and `AsyncBinding`

```go
type AsyncBinding struct {
    Func         func(goja.FunctionCall) goja.Value
    TSReturnType string
}

func PromisifyAsync[T any](vm *goja.Runtime, loop *eventloop.EventLoop,
    work func(ctx context.Context, call goja.FunctionCall) (T, error),
) AsyncBinding
```

`PromisifyAsync` turns blocking Go work into a JS Promise. It launches the
work in a goroutine, parks a `SetTimeout` sentinel so `loop.Run` doesn't
return early, and schedules the resolve/reject back onto the event loop.
**Required** for any Promise-returning binding — `RunOnLoop` alone is not
counted as a live job by the event loop.

`PromisifyAsync` returns an `AsyncBinding` carrier rather than the bare
callback. The engine unwraps it to `.Func` at `vm.Set` time so goja's
special-case detection of `func(goja.FunctionCall) goja.Value`
host-callbacks still fires; the `.d.ts` emitter reads `.TSReturnType`
to emit `Promise<T>` (mapped from the generic `T` via reflect at
construction time) instead of the previous `unknown`. Hosts assigning
the result into a `map[string]any` for `RegisterNamespace` /
`RegisterNamespaceFactory` get the unwrap recursively too — no manual
work required.

### Version

```go
import "github.com/codedeviate/sercon/pkg/scriptengine"

fmt.Println(scriptengine.Version) // "0.1.0"
```

Bumped in lockstep with the git tag.

## 4. CLI — `sercon`

```
sercon [flags] <script.ts> [script.ts ...]
sercon [flags] -                # read entry script from stdin
sercon --examples | --help | --version
```

| Flag | Purpose |
|---|---|
| `-timeout DURATION` | Wall-clock limit per script (default `10s`; `0` disables). |
| `-root DIR` | Override the script root for `require`/`import` resolution. |
| `-emit-dts PATH` | Write the example bindings' `.d.ts` to `PATH` and exit. |
| `-v` | Verbose: trace the rewritten entry-script JS and each module resolution to stderr; also print duration on failure. |
| `-h`, `--help` | In-depth colorized help. |
| `--examples` | In-depth colorized walkthrough of every feature. |
| `--no-pager` | Don't page `--help` / `--examples`. By default, when stdout is a terminal they pipe through `$PAGER` (falling back to `less` with `LESS=FRX`, color preserved); a pipe/redirect, `--no-pager`, or `PAGER=cat` renders directly. |
| `--version` | Print the engine version (plus goja/esbuild build-info versions). |
| `--watch` | Re-run on file change under the script root. Debounced 150 ms. Ctrl-C exits. |

Each positional argument is either a path to a `.ts` / `.tsx` file or
`-` to read an entry script from standard input:

```bash
echo 'runtime.log(1 + 2);' | sercon -
sercon prelude.ts -                 # prelude then stdin
```

### Passing arguments: `--` and `runtime.argv`

Everything after a standalone `--` is the script argument vector,
exposed to every script via `runtime.argv`:

```bash
sercon run.ts -- --port 8080 hello
```

`runtime.argv` uses the Node/Bun layout `[programName, scriptPath,
...userArgs]` — `argv[0]` is `"sercon"`, `argv[1]` is the path of the
script currently executing, and the post-`--` arguments start at index
2 (so `runtime.argv.slice(2)` is just your args). All scripts in one
invocation share the same `--` tail, but each sees its own path at
`argv[1]`. The global is always present; with no `--` it is just
`[programName, scriptPath]`.

```ts
const args = runtime.argv.slice(2);   // ["--port", "8080", "hello"]
```

Exit codes are distinct per failure type so shells can react sensibly.
When several scripts run, the highest applicable code wins:

| Code | Meaning |
|---:|---|
| `0` | every script passed. |
| `1` | CLI usage error (unknown flag, missing script argument, …). |
| `2` | at least one script failed to transpile — never ran. |
| `3` | at least one script timed out (`-timeout`) or was context-cancelled. |
| `4` | at least one script ran and threw a JS exception. |

`-v` writes lines prefixed with `[sercon] ` to stderr. The traces
include the full rewritten entry-script JS (the form goja actually
runs, after the ESM→CJS rewrite + async IIFE wrapper) and every
module-resolution event, so debugging an unexpected resolve target or a
mis-rewritten import is straightforward.

The CLI registers ten reserved top-level globals; see the next section.

### `--watch`: re-run on file change

`sercon --watch <script.ts>` runs the script once and then blocks,
re-running it every time a watched file changes under the script
root. Useful for iterating on a script body or on the binding
surface during development.

```bash
sercon --watch examples/scripts/smoke.ts
```

What's watched:

- The script root (resolved the same way as the one-shot mode —
  `-root DIR` if set, else `dirname` of the first script
  argument).
- Recursively. Symlinks aren't followed.
- New directories that appear during the session are picked up
  automatically.

What's filtered:

- **File extensions**: `.ts` / `.tsx` / `.js` / `.jsx` / `.json`
  trigger a re-run; `.d.ts` files match via suffix too. Anything
  else (Markdown, images, editor swap files, build artefacts) is
  ignored.
- **Directories**: hidden directories (`.git`, `.vscode`,
  anything starting with `.`) and `node_modules` are excluded —
  both because they generate floods of irrelevant events and
  because recursing into them inflates the watcher's directory
  count for no gain.

Re-runs are **debounced 150 ms**. Editors typically fire several
events per save (write → rename → chmod); collapsing them into a
single re-run keeps the loop responsive without thrashing. The
debounce window resets on every event, so a rapid burst of saves
becomes one re-run after the burst settles.

Each run is delimited by a line like `--- sercon re-run @ HH:MM:SS ---`
so the output is visually distinct from the previous run.

`Ctrl-C` (SIGINT) or `SIGTERM` exit cleanly. Per-script failures
inside a watch session log as `FAIL` but don't terminate the loop
— a syntax error you're trying to fix shouldn't kick you out of
watch mode.

The watcher reuses the same `Engine` across runs. Each `Run` already
gets a fresh `*goja.Runtime`, so re-running is just re-invoking
`runOne` on the existing engine — there's no per-iteration setup
cost beyond the transpile + module-resolution work.

**Module-graph invalidation.** Watch mode tracks each entry
script's import graph (its own file plus every module it resolves,
captured via `Engine.SetResolveHook`) during the run. On a file
change it re-runs **only the entries whose graph includes the
changed file** — editing a helper that just one of three entry
scripts imports re-runs that one entry, not all three. An entry's
own file counts (editing it re-runs it); stdin entries and any
entry whose graph isn't known yet re-run unconditionally
(conservative). Before each re-run the module cache is busted
(`Engine.ResetModuleCache`) so an edited import's new source
actually takes effect — the registry otherwise caches compiled
bytecode across runs.

### Editor autocomplete (`sercon init`)

```
sercon init [dir]
```

Drops two files into `dir` (default the current directory) so any editor
backed by the TypeScript language server (VSCode, Zed, Neovim+coc,
Sublime LSP, …) gives completion and hover docs for the reserved globals
with no plugin or manual config:

- **`sercon.d.ts`** — the binding declarations (same content as
  `sercon -emit-dts`): ambient `declare const` blocks for `runtime`,
  `crypto`, `text`, `codec`, `fs`, `net`, `db`, `server`, `services`,
  `tui`, and `console`, each with JSDoc.
- **`jsconfig.json`** — points the language server at `sercon.d.ts` and
  sets `moduleResolution: "Bundler"` (so the extensionless relative
  imports sercon scripts use resolve) and `types: []` (sercon isn't
  Node, so no stray `@types/*`).

Existing files are left untouched unless `--force` is given. The
`examples/scripts/` directory ships with both files already, so the
bundled demos autocomplete out of the box. For a single file without a
config, the no-setup fallback is a leading
`/// <reference path="./sercon.d.ts" />`.

### Shebang lines and executable scripts (`sercon run`)

A script may begin with a `#!` shebang line. It is stripped before
transpile (the line is blanked in place, so transpile/syntax error line
numbers still match the source), which means a `.ts`/`.tsx`/`.js` file
can be made directly executable:

```bash
#!/usr/bin/env sercon
runtime.log("hello from an executable script");
```

```bash
chmod +x hello.ts
./hello.ts
```

The kernel launches a shebang script as `sercon <script> <args...>`,
and in the default mode every positional is treated as a *separate
script* — so positional arguments to a shebang script would be mistaken
for additional script paths. For an executable script that takes
arguments, use the **`run` subcommand** in the shebang:

```
sercon run [flags] <script.ts> [args...]
```

```bash
#!/usr/bin/env -S sercon run
runtime.log("args:", JSON.stringify(runtime.argv.slice(2)));
```

`sercon run` executes exactly one script and hands every token after
the script path to it as `runtime.argv[2:]` (Node/Bun layout:
`[program, script, ...args]`) — no standalone `--` separator is needed
(and a shebang line can't inject one). The `env -S` form splits the
single shebang argument so the kernel runs
`sercon run /abs/path/script.ts arg1 arg2 …`. Flags (`-timeout`,
`-root`, `-v`) are accepted before the script path; everything from the
script path onward is positional and becomes script args. `env -S` is
supported by GNU coreutils, macOS, and the BSDs.

```bash
sercon run script.ts --port 8080 alice    # argv[2:] = ["--port","8080","alice"]
```

### `sercon serve`: long-running scripts with production niceties

```
sercon serve [flags] <script.ts> [-- args...]
```

`sercon serve` is a sibling subcommand that wraps the same engine
but adds the conveniences a process supervisor (systemd, docker,
launchd) usually wants from a long-lived script. Use it whenever a
script binds an HTTP/HTTPS listener (or any future `server.*`
protocol) and is expected to keep running. Vanilla `sercon
script.ts` also keeps the loop alive while listeners are bound, but
gives you none of the deltas below.

| Flag | Purpose |
|---|---|
| `--shutdown-timeout DURATION` | Graceful-shutdown deadline on SIGTERM/SIGINT (default `30s`). After the deadline the engine hard-cancels. |
| `--port-override N` | If non-zero, replaces every literal `port:` in `server.*.listen({...})` calls with `N`. Useful for spinning up the same script on a different port without editing source. Only literal port values are rewritten — computed expressions (`{port: getPort()}`) are left alone. |
| `-timeout DURATION` | Per-script wall-clock limit. Defaults to `0` (disabled) under `serve` so listeners don't get cut off; opt back in if you want one. |
| `-root DIR` | Script root for `require`/`import` resolution (same semantics as vanilla `sercon`). |
| `-v` | Verbose engine tracing to stderr. |

Behavioural deltas vs vanilla `sercon`:

- **Access log to stderr.** Each request emits one line in the
  format `ts remote method path status dur_µs`, e.g.
  `2026-05-29T10:23:14Z 127.0.0.1:54321 GET /users/42 200 1843µs`.
  WebSocket upgrades log only the upgrade line; per-frame traffic
  is suppressed.
- **Readiness signal to stdout.** When each listener calls back
  into the binding with `bound`, the binding prints `READY
  listening on tcp/0.0.0.0:8080` to stdout. Multiple listeners
  produce one READY line each. Supervisors that read-line on
  startup (`systemd-notify`, `docker-compose` healthchecks) can
  wait on it.
- **Graceful shutdown.** SIGTERM / SIGINT triggers `srv.close()`
  on every active listener concurrently, then waits up to
  `--shutdown-timeout` for the script to drain. Clean exit is
  exit code `0`. Past the deadline, `loop.Terminate()` fires
  and the process exits with the usual classification.
- **No `--watch`.** Re-running while a listener is bound would
  race port rebinding; `sercon serve --watch …` exits with a
  usage error. Use vanilla `sercon --watch script.ts` for the
  hot-reload development loop.

```bash
sercon serve examples/scripts/server-http.ts
sercon serve --port-override 9090 server.ts
sercon serve --shutdown-timeout 10s server.ts
```

## 5. Reserved globals (script surface)

sercon scripts get ten reserved top-level globals. Use them directly —
there is no enclosing namespace. The full per-binding reference is the
generated `examples/scripts/sercon.d.ts`; this section is the at-a-glance
prose.

**Reserved names:** `runtime`, `crypto`, `text`, `codec`, `fs`, `net`,
`db`, `server`, `services`, `tui`. User code can shadow these with a
local `let`/`const`/`var` per normal JavaScript scoping — sercon does
not intervene at runtime. `server` (added in v0.10.0) covers inbound
listeners — see [section 6](#6-servers) for the long-form treatment.

**Deterministic key order.** Objects returned from bindings enumerate
their keys in a stable order (declaration order for fixed-shape results;
source / column / insertion order for dynamic ones), so
`JSON.stringify(result)` is byte-stable run-to-run — safe to hash for
canonical serialization (payment signing, webhook signatures). The lone
exception is `text.jq`, whose underlying engine discards key order
internally.

### `console` (browser/Node compatibility)

Beyond the ten reserved globals, sercon provides a `console` global so
scripts pasted from a browser or Node run unchanged:

| Method | Stream |
|---|---|
| `console.log` / `console.info` / `console.debug` | stdout |
| `console.warn` / `console.error` | stderr |

Each prints one space-joined line — clean and prefix-free (no
timestamp), unlike the goja default console. Primitives print raw;
objects and arrays render as JSON (`console.log({a:1})` → `{"a":1}`),
with circular references falling back to `[object Object]` rather than
throwing. The CLI disables the engine's built-in console so this
stream-correct shim is the only `console` a script sees. `runtime.log`
is the native equivalent of `console.log` (stdout) and formats the same
way. Library embedders get the goja_nodejs console by default instead;
toggle it with `Options.DisableConsole`.

### Migration from v0.8.0

| v0.8.0 | v0.9.0 |
|---|---|
| `api.runtime.log` | `runtime.log` |
| `api.crypto.hash.sha256` | `crypto.hash.sha256` |
| `api.text.str.trim` | `text.str.trim` |
| `api.format.barcode.encode` | `codec.barcode.encode` |
| `api.fs.path.basename` | `fs.path.basename` |
| `api.net.http.get` | `net.http.get` |
| `api.db.sqlite.open` | `db.sqlite.open` |
| `api.tools.exec.shell` | `services.exec.shell` |
| `api.tools.git.commit` | `services.git.commit` |
| `api.ui.tui.layout` | `tui.layout` |
| `Sercon.argv` | `runtime.argv` |

The `api` global no longer exists; reading it throws a
`ReferenceError`. The `Sercon` global is also gone.

A mechanical sed pass over your scripts handles the prefix drop and the
three renames — see the `CHANGELOG.md` `[0.9.0]` "Changed" entry
for a sketch.

### `runtime`

Script-host scaffolding. Members:

- `runtime.log(...args)` — print a space-joined line (primitives raw,
  objects/arrays as JSON).
- `runtime.assert.equal(actual, expected, msg?)` /
  `runtime.assert.ok(cond, msg?)` — throw on mismatch / falsy.
  `assert.equal` is **deep**: distinct objects/arrays with identical
  contents compare equal (structural, recursive — key order irrelevant).
- `runtime.time.nowMs()` — current wall-clock in milliseconds.
- `runtime.time.sleep(ms)` — Promise that resolves after `ms` on the
  event loop.
- `runtime.time.format(unixMs, layout, tz?)` — Go-style time layout
  (e.g. `"2006-01-02 15:04:05"`).
- `runtime.env.get(name)` — process environment variable; returns
  `undefined` when unset (never throws).
- `runtime.argv` — `[programName, scriptPath, ...userArgs]`. User args
  come from the CLI args after a `--` separator. The layout mirrors
  Node/Bun: `argv[0]` is `"sercon"`, `argv[1]` is the path of the
  currently-executing script (so each script in a multi-arg invocation
  sees its own path), and `argv.slice(2)` is just your args. The
  property is always present; with no `--` it is just
  `[programName, scriptPath]`.

```ts
runtime.log("hello");
runtime.assert.equal(2 + 2, 4);
const args = runtime.argv.slice(2);   // post-`--` user args
```

### `crypto`

Hashing, JWT, and age-style encryption. Members:

- `crypto.hash.*` — `md5`, `sha1`, `sha256`, `sha384`, `sha512`,
  `sha3_256`, `sha3_512`, `blake3`, `crc32`. All take a `string` and
  return the hex digest.
- `crypto.jwt.sign(claims, key, opts?)` / `crypto.jwt.view(token)` /
  `crypto.jwt.validate(token, key, opts?)` — HS256/RS256/ES256 JWTs.
- `crypto.encrypt.{keygen, keygenPgp, encrypt, decrypt, rekey,
  detectBackend}` — age-format symmetric encryption with PGP-compat
  key wrapping.

### `text`

String, regex, charset, and data manipulation. Members:

- `text.str.*` — `trim`, `ltrim`, `rtrim`, `reverse`, `stripHtml`,
  `nl2br`, `br2nl`, `base64Encode`, `base64Decode`, `urlEncode`,
  `urlDecode`, `htmlEntityDecode`, `pad`/`lpad`/`rpad`, `sprintf`,
  `printf`, `normalizeNewlines`.
- `text.preg.*` and `text.preg2.*` — PCRE-style regex (PHP-compatible
  named-capture, lookbehind, etc.).
- `text.charset.detect(data)` / `decode(data, charset)` /
  `encode(text, charset)` — character-set detection and conversion.
- `text.jq.query(json, expr)` / `queryAll(json, expr)` — jq filtering
  against parsed JSON values.
- `text.diff.compare(a, b, opts?)` — unified / patience / inline diffs
  between two strings.

### `codec`

Binary-format codecs (was `format` in v0.8.0). Members:

- `codec.compression.{algos, compress, decompress}` — gzip, deflate,
  zlib, bzip2, zstd, brotli, lz4, xz, snappy.
- `codec.barcode.{formats, decodableFormats, encode, decode}` — QR,
  DataMatrix, Aztec, PDF417, Code128, Code39, Codabar, EAN-13/8, UPC-A,
  ITF. `encode` returns PNG bytes; `decode` reads image bytes.
- `codec.checkdigit.{algos, validate, compute, inspect}` — Luhn,
  ISBN-10/13, EAN-13/8, UPC-A.
- `codec.php.{serialize, unserialize, varExport, parseVarExport,
  varDump, parseVarDump}` — read and write PHP data dumps.
- `codec.perl.{dumper, parseDumper}` — read and write Perl
  `Data::Dumper` output.

#### `codec.php` — PHP dump formats

Three PHP textual formats, each with an encoder and a matching parser:

- `codec.php.serialize(value, opts?): string` — PHP `serialize()`. Emits
  the canonical wire form (`s:`, `i:`, `d:`, `b:`, `N;`, `a:`, `O:`).
- `codec.php.unserialize(text, opts?): value` — inverse of `serialize`.
- `codec.php.varExport(value, opts?): string` — PHP `var_export()`: valid
  PHP source (`array ( … )`, `\Cls::__set_state( … )`).
- `codec.php.parseVarExport(text, opts?): value` — read a `var_export`
  literal back to a value.
- `codec.php.varDump(value, opts?): string` — PHP `var_dump()`:
  human-readable debug output with type/length annotations.
- `codec.php.parseVarDump(text, opts?): value` — **best-effort** read of
  `var_dump` output.

**Array heuristic.** A JS array, or a JS object whose keys are exactly the
contiguous integer sequence `0..n-1`, maps to a PHP list array. Any other
object maps to a PHP associative array (string/int keys preserved).

**The `__class` sentinel.** An object carrying a `__class` string key
(configurable via `opts.classKey`, default `"__class"`) round-trips as a
PHP *object* — `serialize` emits `O:…`, `var_export` emits
`\Cls::__set_state(…)`, `var_dump` emits `object(Cls)#…`. The remaining
keys become the object's properties. Decoding restores the `__class` key.
Without the sentinel a value is treated as an array.

**Shared references and cycles.** `serialize`/`unserialize` model PHP's
`r:` / `R:` reference markers: when the same object appears more than once,
the decoder resolves both occurrences to the *same* JS object (a DAG is
preserved). A true cycle (an object reachable from itself) **throws** on
encode — there is no JS value that can represent it losslessly here.

**`var_dump` parse is lossy.** PHP's `var_dump` is a debug view, not a
serialization format; `parseVarDump` is best-effort and **throws** when it
hits something it cannot faithfully reconstruct: the `*RECURSION*`
marker, a length-truncated string (its declared byte length disagrees
with the bytes present), or visibility-annotated property names
(`["x":"Cls":private]`).

#### `codec.perl` — Perl `Data::Dumper`

- `codec.perl.dumper(value, opts?): string` — emit a `Data::Dumper`-style
  dump (`$VAR1 = … ;`) with normalized indentation.
- `codec.perl.parseDumper(text, opts?): value` — read `Data::Dumper`
  output back to a value.

**The JSON bool convention.** Perl has no native boolean, so JSON modules
represent one as a blessed scalar reference (e.g.
`bless( do{\(my $o = 1)}, 'JSON::XS::Boolean' )`). `dumper` emits JS
booleans in this form, and `parseDumper` decodes a blessed scalar ref in
the JSON-bool family (`JSON::XS::Boolean`, `JSON::PP::Boolean`, …) back to
a JS boolean. A bare `1` / `0` stays a number. The class used on encode is
`opts.perlBoolClass` (default `"JSON::XS::Boolean"`). Blessed hashes carry
the `__class` sentinel, mirroring the PHP side. Cycles **throw**.

#### `opts` and shared caveats

All `codec.php.*` / `codec.perl.*` functions accept an optional `opts` bag:

- `classKey` (default `"__class"`) — the key used as the class sentinel.
- `perlBoolClass` (default `"JSON::XS::Boolean"`) — class for Perl booleans.
- `indent` — override the default 2-space indentation step (`varExport`,
  `dumper`).

Caveats shared with JSON: strings are UTF-8 and lengths are reported in
**bytes** (multibyte chars count as more than one); JS has a single number
type, so `int` and `float` collapse the same way `JSON.stringify` does
(e.g. `1.0` may re-emit as `1`). Decoded objects preserve a stable key
order, so re-encoding is byte-stable — safe for canonical-JSON / payment
hashing.

### `fs`

Filesystem operations:

- `fs.path.dirname(p)` / `fs.path.basename(p, suffix?)` — POSIX-style.
- `fs.archive.create(srcPath, opts?)` /
  `fs.archive.extract(archivePath, destDir, opts?)` — tar/zip with
  optional compression.

### `net`

Network clients and probes:

- `net.http.{get, post, request}` — fetch-style HTTP client; `request`
  accepts headers, body, timeout, retry, follow, and basic auth.
- `net.probe.{tcp, dns, tls, ntp, whois, ping, smtp, wss}` — one-shot
  reachability / handshake probes. Each returns a structured result;
  failures surface as `Error` objects with details.
- `net.netstatus.check(host)` — combined reachability summary.
- `net.email.*` — `spf`, `dmarc`, `mtasts`, `tlsrpt`, `bimi`, and
  `all(domain)` which runs every probe in parallel and aggregates; plus
  `send({…})` — outbound SMTP sender (see §6.7).
- `net.browser.open(url)` — launches the OS default browser.
- `net.tcp.connect(host, port, opts?)` — open a TCP client socket.
- `net.udp.open(opts)` — open a UDP socket (connected or bound).
- `net.icmp.open(opts?)` — open a raw ICMP socket (needs privileges).
- `net.capture.{interfaces, open, openFile, toFile}` — packet capture
  (live + pcap file I/O), pure-Go gopacket.

**Raw sockets (`net.tcp` / `net.udp` / `net.icmp`)** are long-lived,
bidirectional client sockets with a *push / callback* read model — unlike
the one-shot `net.probe.*` helpers, they stay open and deliver inbound
data through callbacks until you `close()` them. Each constructor returns
a `Promise` for a handle object:

- `net.tcp.connect(host, port, opts?)` — dial a TCP peer. opts
  `{ timeout? (ms, default 10000), readBuffer? (inbound channel capacity,
  default 64) }`. The handle has `write(data)` (string → UTF-8 bytes,
  `Uint8Array` → raw bytes; returns a `Promise`), the `remote` / `local`
  address strings, plus the shared callbacks below. Inbound chunks arrive
  via `onData`.
- `net.udp.open(opts)` — two modes selected by opts. **Connected**
  `{ host, port, readBuffer? }` resolves the peer up front and exposes
  `send(data)`. **Bound** `{ bind: "127.0.0.1:0", readBuffer? }` listens on
  a local address and exposes `sendTo(data, host, port)`; its inbound
  events also carry `address` / `port` of the sender. Either way the handle
  has `local` (the bound address — read the chosen port back from it when
  you bind to `:0`) and delivers datagrams via `onMessage`.
- `net.icmp.open(opts?)` — open a raw ICMP socket. **Requires root /
  `CAP_NET_RAW`**; without the privilege `open()` rejects with an error
  naming that requirement. opts `{ network?: "ip4" | "ip6" (default
  "ip4"), readBuffer? }`. `send({ to, type?, code?, id?, seq?, payload? })`
  builds and writes a message; `type` defaults to the network's echo
  request. **The body is always Echo-shaped** (`id` / `seq` / `payload`) —
  only the `type` / `code` are customizable, non-Echo bodies are not
  modelled. The handle has `network` / `local`; inbound events carry
  `address` / `type` / `code` and arrive via `onMessage`.

All three handles share the same callback surface and lifecycle:

- `onData(cb)` (TCP) / `onMessage(cb)` (UDP, ICMP) — register a listener
  for inbound data. The event object carries `bytes` (a `Uint8Array`) and
  `text` (the bytes decoded as UTF-8); UDP-bound / ICMP events add the
  sender metadata noted above.
- `onClose(cb)` — fires once when the socket closes (locally or remotely).
- `onError(cb)` — fires on a read / connection error (benign teardown
  closes are reported through `onClose`, not `onError`).
- `close()` — shut the socket down; returns a `Promise`.

The socket keeps the engine's event loop alive while open (via the
internal `HoldRun` refcount), so a script that only listens won't exit
until it `close()`s. `sercon` scripts can't *bind* a listening TCP/UDP
server socket yet — these are client sockets — so a TCP example needs a
peer to dial; a UDP loopback pair is fully self-contained.

#### Packet capture (`net.capture`)

Packet capture and pcap file I/O, powered by **pure-Go gopacket** (no
`libpcap`, no cgo). Four bindings:

- `net.capture.interfaces()` — **synchronous**; returns an array of
  `{ name, addresses: string[], up, loopback }` for the host's network
  interfaces. Pure-Go (`net.Interfaces`); no privileges, all platforms.
- `net.capture.open({ iface, promisc?, snaplen?, filter? }, pkt => {…})` —
  start a **live** capture on `iface`. Resolves to a handle `{ iface, link,
  close() }`; the per-frame handler receives a decoded packet object (see
  below). `promisc` defaults `true`; `snaplen` defaults `262144`. `filter`
  is an optional tcpdump-like expression string (see **Filtering** below).
  **Linux + macOS only** — Linux uses `AF_PACKET`, macOS uses BPF
  (`/dev/bpf`); **Windows is unsupported** and `open()` rejects there.
  Requires **root / `CAP_NET_RAW` (Linux)** or **`/dev/bpf` read access
  (macOS)**; without the privilege `open()` rejects naming the
  requirement. `close()` returns `Promise<void>`.
- `net.capture.openFile(path, pkt => {…}, opts?)` — read a `.pcap` /
  `.pcapng` file (format auto-detected from the magic bytes), calling the
  handler once per decoded packet. Returns a `Promise` that resolves at
  EOF. Offline; no privileges. `opts` is an optional trailing argument
  `{ filter? }` (the two-argument form still works); `filter` is the same
  tcpdump-like expression string as `open` (see **Filtering** below).
- `net.capture.toFile(path, { linkType?, snaplen? })` — open/create a
  `.pcap` file and return `{ write(bytes, { ts? }), close() }`. `write`
  appends a raw frame (a `Uint8Array`); `opts.ts` (ms) overrides the
  per-frame timestamp (defaults to now). `close()` flushes and returns
  `Promise<void>`. `linkType` defaults to Ethernet, `snaplen` to `262144`.
  Offline; no privileges.

The **decoded packet object** passed to the `open` / `openFile` handlers:

```ts
{
  ts, length, captureLength, link,     // always present
  eth?:  { src, dst, type },
  ip?:   { version, src, dst, protocol, ttl },
  tcp?:  { srcPort, dstPort, seq, ack,
           flags: { syn, ack, fin, rst, psh, urg } },
  udp?:  { srcPort, dstPort, length },
  icmp?: { type, code },
  payload?: Uint8Array,                // application-layer bytes, if any
  bytes:    Uint8Array,                // the full raw frame (always present)
}
```

Layer keys (`eth` / `ip` / `tcp` / `udp` / `icmp` / `payload`) are present
**only when that layer decodes** — a truncated or unrecognised frame still
yields the always-present `ts` / `length` / `captureLength` / `link` /
`bytes`.

**Filtering.** Both `open` and `openFile` accept an optional `filter`
string — a **subset of tcpdump syntax**. Supported grammar:

- protocols: `tcp`, `udp`, `icmp`, `ip`, `ip6`;
- hosts: `host X`, `src host X`, `dst host X` (IPv4 or IPv6 address);
- ports: `port N`, `src port N`, `dst port N`;
- boolean composition: `and`, `or`, `not`, and parentheses;
- **implicit-and** between juxtaposed primaries — `tcp port 80` is exactly
  `tcp and port 80`.

Example: `net.capture.open({ iface: "en0", filter: "tcp and port 80" },
pkt => {…})`, or `net.capture.openFile("cap.pcap", pkt => {…},
{ filter: "udp or icmp" })`.

The filter is a **userspace, post-decode** predicate — it is **not**
compiled to a kernel BPF program. It runs on each frame *after* gopacket
has decoded it, so it saves the JS-callback dispatch and object-conversion
cost for non-matching packets but does **not** avoid the kernel→userspace
copy (a real kernel filter would). `net X/Y` (CIDR) and `portrange` are
**not supported yet**. A malformed expression makes `open` / `openFile`
**reject**.

**Limitations.** No live capture on Windows. The `filter` grammar is the
tcpdump subset above (no CIDR / `portrange`); for anything beyond it,
filter inside the callback on the decoded fields. Common-layer decode only
(Ethernet / IPv4 / IPv6 / TCP / UDP / ICMP); exotic protocols surface only
as raw `bytes`. At high packet rates frames backpressure and drop at the
kernel, exactly like `tcpdump`.

The pcap file API round-trips: write raw frames with `toFile`, read them
back decoded with `openFile`, no privileges required (see
`examples/scripts/capture-file.ts`).

### `db`

Database / KV / directory clients:

- `db.sqlite.open(path)` — embedded SQLite (cgo-free driver).
- `db.postgres.open(dsn | opts)` — PostgreSQL via pure-Go `pgx`.
- `db.mysql.open(dsn | opts)` — MySQL / MariaDB via pure-Go `go-sql-driver`.
- `db.mssql.open(dsn | opts)` — Microsoft SQL Server via pure-Go `go-mssqldb`.
- `db.clickhouse.open(dsn | opts)` — ClickHouse via pure-Go `clickhouse-go` v2 (`opts.secure` for TLS).
- `db.oracle.open(dsn | opts)` — Oracle via pure-Go `go-ora` (`opts.database` is the service name).
- `db.redis.open(addr, opts?)` — Redis client.
- `db.memcached.open(addr)` — Memcached client.
- `db.ldap.open(url, opts?)` — LDAP client (search, bind).
- `db.dict.{define, match}` — local dictionary lookup / fuzzy match.

**SQL engines (`sqlite` / `postgres` / `mysql` / `mssql`)** share one
handle: `open()` resolves to `{ exec, query, queryValue, begin,
prepare, close }` (transactions via `begin()` → `{ exec, query,
queryValue, commit, rollback }`; prepared statements via `prepare(sql)`
→ `{ exec, query, queryValue, close }`). The server engines accept
either a driver **DSN string** or a connection **options object**
(`{ host, port, user, password, database }`, plus `sslmode` for
postgres); credentials in the assembled URL DSNs are percent-escaped.
The connection is pinged on `open()` so a bad DSN or unreachable server
fails there, not at the first query. Bind parameters are positional and
passed after the SQL string — write your engine's placeholder syntax:
`?` (sqlite, mysql), `$1` (postgres), `@p1` (mssql). All four drivers are
pure Go (no cgo). Scripts must `close()` the handle (and commit/rollback
transactions, close prepared statements) — there is no GC finalizer.

### `services`

Subprocess and external-CLI / service wrappers (was `tools` in
v0.8.0). Members:

- `services.exec.shell(cmd, opts?)` — run a shell command; captures
  stdout/stderr or streams into a `tui` pane when `opts.pane` is set.
- `services.exec.http(method, url, opts?)` — shell-level `curl`-style
  HTTP (separate from `net.http`, intended for raw protocol use).
- `services.git.*` — git porcelain wrappers (clone, status, commit,
  log, …) invoking the local `git` binary.
- `services.gh.{authStatus, prList, repoView}` — thin `gh` CLI
  wrappers.
- `services.ai.{providers, send}` — provider-agnostic AI chat client.

### `tui`

Multi-pane terminal UI (was `ui.tui` in v0.8.0). `tui.layout(tree)`
declares panes; `tui.pane(name)` returns a pane handle with `write`,
`writeln`, `clear`, `title`. `services.exec.shell({pane})` streams
subprocess I/O into a pane.

Scripts that orchestrate multiple subprocesses (e.g. `brew`, `npm`,
`cargo` updates running in parallel) can declare a multi-pane terminal
layout and route each subprocess's stdout/stderr into its own pane.
Activation is **script-driven**: the first call to
`tui.layout(...)` enters TUI mode. If stdout is not a TTY (CI,
pipes, `make demo`), the same calls fall back to prefixed plain-text
lines (`[paneName] line`) so the same script runs in both contexts.

#### Layout

```ts
tui.layout({
  rows: [
    { name: "log", title: "Orchestrator", weight: 1 },
    { cols: [
        { name: "brew" },
        { name: "npm" },
      ],
      weight: 2,
    },
  ],
});
```

Each layout node is one of:

- `{ name: string, title?: string, weight?: number }` — a leaf pane.
- `{ rows: LayoutNode[], weight?: number }` — a vertical split.
- `{ cols: LayoutNode[], weight?: number }` — a horizontal split.

`weight` defaults to 1. Pane names must be unique across the whole
tree. `layout` may be called **once** per Run; a second call throws.

#### Pane handles

```ts
const log = tui.pane("log");
log.writeln("Updating Homebrew…");
log.title("Done");
log.clear();
log.write("partial line ");
```

`tui.pane(name)` returns a handle with `write`, `writeln`, `clear`,
`title`. Methods are synchronous from the script's perspective — they
enqueue to the TUI goroutine.

#### Subprocess routing

`services.exec.shell(cmd, opts)` accepts `opts.pane` (a Pane handle or a
pane name string):

```ts
await services.exec.shell("brew update && brew upgrade", { pane: "brew" });
```

When set, the subprocess's stdout **and** stderr stream into the pane
line by line. The returned `{ exitCode, durationMs, success }` is the
same as without `pane:` except `stdout` and `stderr` are empty (data
was streamed, not captured). ANSI colors are translated to tview color
tags and rendered natively; `\r` without `\n` overwrites the current
line so progress spinners render cleanly.

#### Keybindings (TTY mode only)

| Key                  | Action                       |
|----------------------|------------------------------|
| `Tab` / `Shift-Tab`  | Cycle focus through panes    |
| `PgUp` / `PgDn`      | Scroll focused pane one page |
| `↑` / `↓`            | Scroll focused pane one line |
| `Home` / `End`       | Jump to top / bottom         |
| `Ctrl-C`             | Abort the script             |

The focused pane has a yellow border; the bottom status bar shows its
name and the active keys.

#### Limitations (v1)

- `tui` is **incompatible with `--watch`**: calling
  `tui.layout()` under `--watch` throws. Use one or the other.
- No mouse, no runtime layout mutation, no input panes (`tui.input`),
  no snapshot-to-normal-screen on exit. The alt screen is restored at
  script end and the usual `PASS`/`FAIL` line prints.

## 6. Servers

The `server` global (added in v0.10.0) hosts inbound listeners — scripts
that *accept* connections rather than make them. It ships `server.http`
and `server.https` (§6.2), `server.smtp` (§6.7), and the raw
`server.tcp` / `server.udp` listeners (§6.8); future cycles will add
IMAP, FTP, and POP3 against the same engine foundation.

### 6.1 Foundation: `HoldRun` + `LoopCallable`

Two engine primitives back every long-lived binding. They live on
`*scriptengine.Engine` (CLI use of them is internal; the public
contract is "use these if you write a similar binding yourself").

**`Engine.HoldRun(reason string) (release func())`** — keeps
`loop.Run` from exiting while a binding has resources outstanding.
The event loop normally exits as soon as its `jobCount` drops to
zero; `setTimeout` / `setInterval` / `setImmediate` are the only
things that count. `HoldRun` parks a 24-hour sentinel timer under
the hood (same trick `PromisifyAsync` uses for one-shot async work),
returns a `release` function that clears it, and increments a
refcount so multiple concurrent holders compose. `release` is
idempotent. Any sentinels still parked when `Run` returns are
cleanup-drained automatically as a safety net — a binding that
forgets to call `release` does not leak into the next Run.

**`scriptengine.NewLoopCallable(loop, fn)`** — wraps a captured
`goja.Callable` (a JS function reference) so it can be invoked from
*any* goroutine. Goja's runtime is single-threaded; calling a
Callable directly from a non-loop goroutine corrupts engine state.
`LoopCallable.Call(buildArgs)` enqueues a `loop.RunOnLoop` callback,
builds the arg list on the loop (where `vm.ToValue` is valid),
invokes the callable, and returns the result to the caller's
goroutine. `LoopCallable.CallOnLoop(vm, args...)` is the
already-on-the-loop variant — using `Call` from inside a loop
callback deadlocks because the new `RunOnLoop` job can't run until
the current one returns.

You use them together. A typical server binding holds one or two
`LoopCallable`s for the user's handlers, parks one `HoldRun`
sentinel per active listener, and releases on close. The CLI
bindings under `cmd/sercon/api_server*.go` follow this pattern.

**Concurrency model.** Because every handler invocation marshals
back onto the single goja loop, **handlers serialize**: only one
JS handler runs at a time, across all listeners. This is a feature
(no JS data races, no shared-state confusion) and a throughput
ceiling. For CPU-bound work in a handler, offload to a Go
binding that does the work in a goroutine and resolves a Promise.

### 6.2 `server.http.listen` / `server.https.listen`

```ts
// HTTP
const srv = await server.http.listen({
  port:   8080,                              // required
  host:   "0.0.0.0",                         // default
  routes: { "GET /":  (req, res) => res.text("hi") },
  use:    [logger],                          // optional global middleware
});

// HTTPS — identical plus cert/key (file paths OR inline PEM strings)
const srv2 = await server.https.listen({
  port: 8443,
  cert: "/etc/ssl/server.pem",
  key:  "/etc/ssl/server.key",
  routes,
});

srv.address;          // "tcp/0.0.0.0:8080"
await srv.close();    // graceful: stop accepting, drain, release HoldRun
await srv.stopped;    // Promise that resolves when the listener exits
```

**Options:**

| Key | Type | Notes |
|---|---|---|
| `port` | `number` | Required. Bind port. `0` lets the kernel pick; read the chosen port off `srv.address`. |
| `host` | `string` | Default `"0.0.0.0"`. Pass `"127.0.0.1"` for loopback-only. |
| `routes` | `Record<string, RouteValue>` | Required (may be `{}`). Keys are stdlib `http.ServeMux` patterns. |
| `use` | `Middleware[]` | Optional global middleware; runs in array order before any per-route middleware. |
| `cert` | `string` | **HTTPS only.** File path *or* inline PEM. |
| `key`  | `string` | **HTTPS only.** File path *or* inline PEM. |

**Route patterns** are stdlib Go 1.22+ `http.ServeMux` syntax:
`"METHOD /path/{param}/{rest...}"`. The leading method is optional
(omit to match any). `{name}` captures one path segment;
`{name...}` captures the tail (wildcard). No new routing library —
stdlib does the matching, including longest-prefix wins. Captured
values land in `req.params`.

**`RouteValue`** is either a bare handler — `(req, res) =>
res.text("…")` — or an object `{ use?: Middleware[], handler:
HandlerFn }` for per-route middleware (see §6.4).

**Request shape** (`req`):

```ts
type Request = {
  method:    string;                       // "GET"
  url:       string;                       // full URL incl. scheme + host
  path:      string;                       // "/users/42"
  query:     Record<string, string[]>;     // ?a=1&a=2 → {a: ["1","2"]}
  headers:   Record<string, string[]>;     // lowercase keys
  params:    Record<string, string>;       // path-pattern captures
  body:      string;                       // UTF-8 decoded request body
  bodyBytes: Uint8Array;                   // same data as raw bytes
  remote:    string;                       // "1.2.3.4:54321"
  cookies:   Record<string, string>;       // parsed Cookie header
};
```

**Response builder** (`res`) — every method returns `res` for
chaining; the terminal `.json` / `.text` / `.html` / `.bytes` /
`.empty` / `.redirect` sets the body and flips an internal
`finalized` flag:

```ts
type CookieOpts = {
  domain?:   string;
  path?:     string;
  maxAge?:   number;                // seconds; 0 = session; <0 = delete
  expires?:  number;                // unix ms
  secure?:   boolean;
  httpOnly?: boolean;
  sameSite?: "strict" | "lax" | "none";
};

type Response = {
  status(code: number): Response;
  header(name: string, value: string): Response;
  cookie(name: string, value: string, opts?: CookieOpts): Response;
  // Terminals (all return res so they remain chainable but no further
  // write makes sense after one fires; a second terminal throws):
  json(value: unknown): Response;       // → application/json
  text(s: string):     Response;        // → text/plain
  html(s: string):     Response;        // → text/html
  bytes(b: Uint8Array, ct?: string): Response;  // default application/octet-stream
  empty():             Response;        // no body; pair with .status() for 204/304
  redirect(loc: string, code?: number): Response;  // default 302
  // Upgrade (WebSocket):
  upgradeWebSocket(opts?: { readBuffer?: number }): Promise<WebSocket>;
};
```

After the handler's returned Promise resolves, the engine checks
`res.finalized`:

- **true** — buffered status / headers / body get written to the
  underlying `http.ResponseWriter`.
- **false** — engine sends `204 No Content`.
- **handler threw** (sync or async) — engine sends `500 Internal
  Server Error` and logs the error (`stderr` under vanilla
  `sercon`; access log under `sercon serve`). The script keeps
  running; only that one request is affected.

Calling a terminal twice on the same `res` throws a `TypeError`
(the second call sees `finalized === true` and refuses).

**Multiple listeners** compose naturally. A script can run

```ts
const [a, b] = await Promise.all([
  server.http.listen({ port: 80,  routes }),
  server.https.listen({ port: 443, routes, cert, key }),
]);
```

Each `listen` call holds its own `HoldRun` sentinel; closing one
listener does not affect the other.

### 6.3 Static-file serving

```ts
"GET /assets/{rest...}": server.http.static({
  dir:         "/var/www/public",   // root directory on disk
  stripPrefix: "/assets/",          // URL prefix to strip before disk lookup
  index?:      "index.html",        // accepted but currently unused (stdlib default applies)
  etag?:       true,                // accepted but currently unused (stdlib default applies)
}),
```

`server.http.static({...})` returns an opaque handler the route
compiler unwraps and registers directly on the mux. **The route
pattern must include a `{rest...}` wildcard** — without it the
match only ever produces the literal prefix and no subpath ever
resolves on disk. Internally a stdlib `http.FileServer(http.Dir(dir))`
wrapped in `http.StripPrefix(stripPrefix, …)`; ETag and range
requests work out of the box. Symlinks outside `dir` are blocked
(stdlib `http.Dir`'s default). No directory listing customisation
yet — `index` and `etag` options are reserved for future tuning;
v0.10.0 inherits the stdlib defaults.

### 6.4 Middleware

Onion model. A middleware is `(req, res, next) => Promise<void>`.
`next` is an async function that runs the rest of the chain;
awaiting it lets the middleware post-process.

```ts
const logger = async (req, res, next) => {
  const start = runtime.time.nowMs();
  await next();                              // run downstream chain
  runtime.log(req.method, req.path, "→", runtime.time.nowMs() - start, "ms");
};

await server.http.listen({
  port: 8080,
  use: [logger],                             // global middleware
  routes: {
    "GET /api/secure": {                     // per-route middleware
      use: [authCheck],
      handler: (req, res) => res.json({ ok: true }),
    },
  },
});
```

**Attachment points:**

- **Global** — `use:` array in `listen({...})` options. Runs for
  every request, in declaration order, before any per-route
  middleware.
- **Per-route** — when a route value is `{ use: [...], handler:
  fn }`, those middleware run after globals but before the
  handler.

For a request to `POST /api/secure` with global middleware `[A, B]`
and per-route middleware `[C]`, the composition is:

```
A(req, res, () => B(req, res, () => C(req, res, () => handler(req, res))))
```

Each layer can `await next()` then mutate response headers or
record timings, exactly like Express / Koa.

**Short-circuit:** a middleware that does *not* call `await next()`
stops the chain. It must terminate `res` itself (e.g.
`res.status(401).text("denied")`) — otherwise the engine sends
`204 No Content` per the unfinalized-response rule.

**Throwing:** any throw mid-chain (sync or via rejected Promise)
bubbles to the engine's 500 path. Wrap with `try`/`catch` if you
want custom error handling.

### 6.5 WebSocket upgrade

```ts
"GET /ws": async (req, res) => {
  const ws = await res.upgradeWebSocket({ readBuffer: 64 });
  // ws is both an async iterator AND has .send / .close.
  for await (const msg of ws) {
    if (msg.type === "text") {
      await ws.send("echo:" + msg.text);
    } else {
      await ws.send(msg.bytes);              // msg.type === "binary"
    }
  }
  // iterator ends on close; ws.closeCode / ws.closeReason populated
},
```

**Type:**

```ts
type WSMessage =
  | { type: "text";   text:  string     }
  | { type: "binary"; bytes: Uint8Array };

type WebSocket = AsyncIterable<WSMessage> & {
  send(data: string | Uint8Array): Promise<void>;
  close(code?: number, reason?: string): Promise<void>;
  closeCode?:   number;
  closeReason?: string;
  remote:       string;
};
```

**Library:** `github.com/coder/websocket` (the modern successor to
`nhooyr.io/websocket`; gorilla/websocket is in maintenance mode).
Pure Go, zero deps.

**Marshalling.** Per upgraded connection, a Go goroutine reads
frames into a buffered channel (capacity = `readBuffer`, default
64). Each `next()` on the async iterator pulls one message off the
channel and resolves on the loop via `LoopCallable`. `ws.send()`
schedules a write back through the connection-owning goroutine.

**Backpressure.** If the read buffer fills (slow JS consumer),
back-pressure propagates into the websocket library's read loop —
the client eventually sees `1009 message too big` after the
library's own limit (1MB default). Bump `readBuffer` if your
workload bursts and you want more in-flight queueing.

`ws.close()` closes the send channel, drains the receive channel,
sends a close frame, and ends the async iterator on the next
`next()` call. The script's `for await` exits cleanly; the
handler's Promise resolves; the connection's goroutine returns.
The HTTP server's `HoldRun` is **unaffected** — it stays alive for
new connections until `srv.close()`.

`async function*` and `for await (...)` are not natively parsed
by goja. esbuild's `Supported` flag lowers both during transpile
(see CHANGELOG), and every Run installs `Symbol.asyncIterator =
Symbol.for("@@asyncIterator")` so the lowered helper and user code
agree on the same iteration key.

### 6.6 Lifecycle

**Vanilla `sercon script.ts`.** Each `server.*.listen` call parks
a `HoldRun` sentinel and the event loop stays alive while the
listener is bound. SIGINT terminates abruptly: the engine's
interrupt watcher calls `loop.Terminate()`, the cleanup drain runs
(closing each listener), and the process exits. No access log, no
readiness signal, no shutdown timeout.

**`sercon serve script.ts`.** Adds the production niceties
documented in §4: structured access log to stderr, a `READY
listening on tcp/…` line on stdout per listener, and graceful
shutdown with `--shutdown-timeout` (default `30s`). Clean SIGTERM
exits `0`. Choose `serve` whenever the script is supervised
(systemd, docker compose, launchd).

```bash
# Quick local test:
sercon examples/scripts/server-http.ts

# Production-style supervised run:
sercon serve examples/scripts/server-http.ts
```

### 6.7 SMTP

SMTP is a **stateful, multi-stage transaction**: a client connects,
optionally authenticates, declares a sender (`MAIL FROM`), one or more
recipients (`RCPT TO`), then streams the message body (`DATA`). sercon
maps each stage to a JavaScript callback so a script can accept or reject
the transaction incrementally — refuse an unknown sender at `MAIL`,
refuse an unrouteable recipient at `RCPT`, or inspect the parsed message
at `DATA`. The inbound listener is `server.smtp.listen({…})`; the
outbound side is `net.email.send({…})` (covered at the end of this
section).

#### `server.smtp.listen({…})`

Synchronously binds the listening socket (so a port already in use throws
immediately), then serves in the background. Returns a handle once the
loop schedules it.

```ts
const srv = await server.smtp.listen({
  port: 2525,
  hostname: "mx.example.com",
  handlers: {
    onMail: (env)       => env.from.endsWith("@example.com"),
    onRcpt: (env, rcpt) => isLocalMailbox(rcpt),
    onData: (env, msg)  => { store(msg); return true; },
  },
});

// srv.address  → "tcp/0.0.0.0:2525"
// srv.close()  → Promise<void>  (graceful, 30s drain)
// srv.stopped  → Promise<void>  (resolves when the listener exits)
```

**Options:**

| Field | Type | Default | Notes |
|---|---|---|---|
| `port` | `number` | — | **Required.** TCP port to bind. |
| `host` | `string` | `"0.0.0.0"` | Bind address. |
| `hostname` | `string` | `os.Hostname()` | EHLO greeting + `Received:` identity. |
| `handlers` | `{onMail, onRcpt, onData}` | — | **Required.** All three functions; see below. |
| `auth` | `(user, pass, env) => bool \| Promise<bool>` | — | Optional SASL verifier (PLAIN + LOGIN). |
| `starttls` | `{cert, key}` | — | Enables STARTTLS. File paths **or** inline PEM. |
| `allowInsecureAuth` | `boolean` | `false` | Permit AUTH on a plaintext connection (dev only). |
| `maxMessageBytes` | `number` | `10485760` (10 MB) | DATA size cap; a non-positive value keeps the default. |
| `maxRecipients` | `number` | `100` | Max `RCPT TO` per transaction. |
| `sessionTimeout` | `number` | `30000` | Per-session idle timeout, milliseconds. |

**Handlers.** `onMail(envelope)`, `onRcpt(envelope, recipient)`, and
`onData(envelope, message)` are invoked at the matching protocol stage.
All three are required. Each may be `async` — a returned Promise is
awaited before the SMTP response is written. The return value (or a
thrown exception) controls the SMTP reply, uniformly across all three:

| Return value | SMTP response | Use case |
|---|---|---|
| `true` / `undefined` | `250 OK` | Accept the stage |
| `false` | `550 Command refused` | Generic permanent reject |
| `string` | `550 <string>` | Permanent reject with a reason |
| `throw` | `451 Temporary failure` | Transient error (client should retry) |

**Envelope** (passed to all three handlers; `recipients` grows as
`RCPT TO` accumulates):

```ts
type Envelope = {
  from:               string;   // "" for the null sender (DSN bounces)
  recipients:         string[]; // accepted RCPT TO so far
  remote:             string;   // "1.2.3.4:54321"
  helo:               string;   // EHLO/HELO hostname the client gave
  authenticatedUser?: string;   // set if AUTH succeeded
  tls?:               { version: string; cipher: string }; // set under STARTTLS
};
```

**Message** (only `onData`; parsed via `jhillyerd/enmime`, with the
exact original bytes always available as `raw`):

```ts
type Message = {
  from:    string;                   // From: header (may differ from envelope.from)
  to:      string[];                 // To: header
  cc:      string[];                 // Cc: header
  subject: string;
  headers: Record<string, string[]>; // lowercase keys; multi-valued
  body: {
    text:  string;                   // text/plain part; "" if absent
    html:  string;                   // text/html part; "" if absent
  };
  attachments: Array<{
    filename:    string;
    contentType: string;
    bytes:       Uint8Array;
  }>;
  raw: Uint8Array;                   // exact DATA bytes (post dot-unstuffing)
};
```

Forwarders and DKIM-style signature verifiers should work from `raw`;
everyone else uses the parsed fields. enmime is lenient with malformed
messages, so a script needing strict RFC 5322 conformance can re-parse
`raw` itself.

**AUTH.** Supply an `auth` callback to enable SASL. Both **PLAIN** and
**LOGIN** mechanisms are advertised; the callback receives the decoded
`(username, password, envelope)` and returns a boolean (or a Promise of
one). By default AUTH is refused on a plaintext connection — the client
must complete STARTTLS first. Set `allowInsecureAuth: true` to accept
credentials over plaintext; this is a **dev-only** escape hatch for
trusted local networks and should never be set in production.

**STARTTLS.** Pass `starttls: {cert, key}` (file paths or inline PEM,
same convention as `server.https`) to advertise STARTTLS. The upgrade
happens mid-session on the same connection; once active, `envelope.tls`
is populated. The listener itself is plaintext at bind time — there is no
implicit-TLS (SMTPS / port 465) inbound mode.

**Concurrency.** Each connection runs on its own Go goroutine, but every
handler invocation **serializes on the goja loop** — exactly one handler
runs at a time (the same single-threaded model as the HTTP listener in
§6.2). A slow `onData` therefore blocks other connections' handlers. For
slow work, acknowledge the stage (`return true`) and process in the
background (start a Promise without awaiting it).

A short, self-contained round-trip (bind, send to self, assert receipt):

```ts
const port = 38095;
let captured: { subject: string; from: string } | null = null;

const srv = await server.smtp.listen({
  port,
  hostname: "test.local",
  handlers: {
    onMail: () => true,
    onRcpt: () => true,
    onData: (env, msg) => {
      captured = { subject: msg.subject, from: env.from };
      return true;
    },
  },
});

await net.email.send({
  to:      "alice@test.local",
  from:    "bob@test.local",
  subject: "round-trip demo",
  body:    "hello from the SMTP demo",
  server:  { host: "127.0.0.1", port, tls: "none" },
});

runtime.assert.equal(captured!.subject, "round-trip demo", "subject");
await srv.close();
```

#### `net.email.send({…})` — outbound

The outbound counterpart lives under `net.email` (next to the `spf` /
`dmarc` / … probes). It opens **one TCP connection per call** — no
pooling, no queueing, no automatic retries — composes the MIME body
in-tree, and returns a per-recipient outcome.

```ts
const result = await net.email.send({
  to:      ["alice@example.com", "bob@example.com"],  // string OR string[]
  from:    "noreply@my.app",
  subject: "your verification code",
  body:    "your code is 123456",                     // plain text
  html:    "<p>your code is <b>123456</b></p>",       // optional HTML alternative
  attachments: [
    { filename: "qr.png", contentType: "image/png", bytes: qrBytes },
  ],
  headers: { "X-App": "myapp" },                      // optional extra headers
  server: {
    host: "smtp.example.com",
    port: 587,                                        // default 587 (submission)
    auth: { username: "u", password: "p" },           // optional PLAIN auth
    tls:  "starttls",                                 // "starttls" (default) | "tls" | "none"
  },
  timeout: 30000,                                     // ms; default 30s
});

// result = { accepted: string[], rejected: Array<{ address, reason }> }
```

**MIME layout.** `text` only → `text/plain; charset=utf-8` (no
multipart). `text` + `html` → `multipart/alternative`. Any attachments →
`multipart/mixed` (each part base64, `Content-Disposition: attachment`).
`Date`, `Message-ID`, and `MIME-Version` are auto-generated; `headers`
are merged on top.

**TLS modes.** `"starttls"` (default) connects plaintext then upgrades,
refusing AUTH until TLS is active. `"tls"` is implicit TLS from connect
(SMTPS, typically port 465). `"none"` is plaintext throughout, with AUTH
disabled.

**Outcomes.** Transport-level failures (connection refused, TLS
handshake, AUTH, `MAIL FROM` rejected) **throw** a JS exception. The
`rejected` array captures only individual `RCPT TO` addresses the server
permitted us to attempt but then refused — the transaction continues for
the rest, so a partial send still delivers to the accepted recipients.

### 6.8 Raw TCP / UDP

`server.tcp.listen` and `server.udp.listen` are the inbound counterparts
to the `net.tcp.connect` / `net.udp.open` clients (§5 `net`). They bind a
listening socket **synchronously** and return the server handle directly —
a port already in use throws immediately — then serve in the background,
keeping the event loop alive via the same `HoldRun` model as the HTTP and
SMTP listeners. `srv.close()` returns a `Promise<void>` (resolving once the
listener is closed and accepted connections are drained), matching the rest
of the `server.*` family. Passing `port: 0` binds an OS-chosen
ephemeral port; read it back from the returned handle's `address`.

Both return a server handle:

```ts
srv.address;       // "tcp/127.0.0.1:54321"  or  "udp/127.0.0.1:54321"
srv.close();       // Promise<void> — stop accepting, drain, release HoldRun
```

#### `server.tcp.listen({…}, conn => {…})`

The second argument is a **connection handler** invoked once per accepted
socket. The `conn` it receives is the **same handle shape** as
`net.tcp.connect` — there is no separate server-connection type to learn:

```ts
const srv = await server.tcp.listen({ port: 0 }, (conn) => {
  conn.onData(ev => conn.write(ev.bytes));   // ev = { bytes: Uint8Array, text }
  conn.onClose(() => runtime.log("peer disconnected"));
  conn.onError(e => runtime.log("conn error", String(e)));
  // conn.write(data)  — string (UTF-8) or Uint8Array
  // conn.remote / conn.local  — peer / local "host:port"
  // conn.close()  — half/full close this connection
});

const port = Number(srv.address.split(":").pop());   // "tcp/127.0.0.1:PORT"
// ... net.tcp.connect("127.0.0.1", port) to talk to it ...
await srv.close();
```

**Options:** `port` (required; `0` for ephemeral), `host` (default
`127.0.0.1`), `readBuffer` (per-connection read chunk size, same meaning
as `net.tcp.connect`).

#### `server.udp.listen({…}, (msg, reply) => {…})`

UDP is connectionless, so the handler fires **once per inbound datagram**
rather than once per connection. `msg` describes the datagram and its
sender; `reply` sends a datagram back to that same sender:

```ts
const srv = await server.udp.listen({ port: 0 }, (msg, reply) => {
  // msg = { bytes: Uint8Array, text, address, port }  — the sender
  runtime.log("got", msg.text, "from", msg.address + ":" + msg.port);
  reply("ack:" + msg.text);     // string or Uint8Array; returns a Promise
});

const port = Number(srv.address.split(":").pop());   // "udp/127.0.0.1:PORT"
await srv.close();
```

**Options:** `port` (required; `0` for ephemeral), `host` (default
`127.0.0.1`).

**Lifecycle.** Identical to §6.6: vanilla `sercon script.ts` keeps the
loop alive while bound and terminates abruptly on SIGINT; `sercon serve
script.ts` adds the access log, a `READY listening on tcp/…` (or
`udp/…`) line on stdout per listener, and graceful shutdown. As with all
`server.*` bindings, the connection / datagram handlers marshal back onto
the single goja loop, so **handlers serialize** (one at a time across all
listeners).

## 7. JavaScript runtime built-ins (goja)

`scriptengine` runs on goja, which implements **ES5.1 + a large subset of
ES6+**. The following are present and behave per spec:

### Globals

- `Object`, `Array`, `String`, `Number`, `Boolean`, `BigInt`
- `Math`, `Date`, `JSON`, `RegExp`
- `Error`, `TypeError`, `RangeError`, `SyntaxError`, `ReferenceError`,
  `EvalError`, `URIError`
- `Promise` (provided via goja's native implementation; resolution
  microtasks are pumped by the event loop)
- `Symbol` (partial: well-known symbols `Symbol.iterator`,
  `Symbol.asyncIterator`, etc., are supported)
- `Proxy`, `Reflect` (partial)

```ts
runtime.log(Math.PI.toFixed(4), Math.max(1, 9, 3));
runtime.log(new Date().toISOString());
runtime.log(JSON.stringify({ a: 1, b: [2, 3] }));
runtime.log("abc".repeat(3), "abc".padStart(6, "_"));
```

### Collections

- `Map`, `Set`, `WeakMap`, `WeakSet`

```ts
const counts = new Map<string, number>();
for (const ch of "banana") counts.set(ch, (counts.get(ch) ?? 0) + 1);
runtime.log([...counts]); // [["b",1],["a",3],["n",2]]
```

### Typed arrays

- `ArrayBuffer`, `DataView`
- `Int8Array`, `Uint8Array`, `Uint8ClampedArray`, `Int16Array`,
  `Uint16Array`, `Int32Array`, `Uint32Array`, `Float32Array`, `Float64Array`

```ts
const buf = new ArrayBuffer(4);
new DataView(buf).setUint32(0, 0xCAFEBABE);
runtime.log(new Uint8Array(buf)[0].toString(16)); // ca
```

### Iteration and generators

- `for…of`, `for…in`, spread, destructuring
- Generator functions (`function*`)
- Async generators (`async function*`) — supported via goja's event loop

```ts
function* fib() {
  let [a, b] = [0, 1];
  while (true) {
    yield a;
    [a, b] = [b, a + b];
  }
}
const it = fib();
const first10 = Array.from({ length: 10 }, () => it.next().value);
runtime.log(first10);
```

### Not (yet) provided

- `fetch` — use `net.http.*` from the CLI, or register your own binding.
- `Worker`, threading primitives.
- DOM globals (`window`, `document`).
- Native ES modules (`import`/`export` are *transpiled* to CommonJS at
  load time — see [section 9](#9-typescript-support)).

## 8. Async runtime additions (goja_nodejs)

The event loop bundles a small Node-compatible runtime on top of goja:

### Timers

```ts
setTimeout(() => runtime.log("after 50ms"), 50);
setInterval(() => runtime.log("tick"), 100);
setImmediate(() => runtime.log("next tick"));
clearTimeout(t); clearInterval(i); clearImmediate(im);
```

The event loop holds the script alive while *any* timer is pending. If
your script ends with only `setTimeout` callbacks scheduled, the run
will wait for them to fire before returning.

### `console`

```ts
console.log("info");
console.error("oops");
console.warn("careful");
console.info("fyi");
console.debug("trace");
```

Disable with `Options.DisableConsole = true` if you want a clean global
namespace.

### `require`

CommonJS require, with TS-aware extension fallback added by sercon (see
[section 11](#11-module-resolution)):

```ts
const helpers = require("./helpers");
helpers.doThing();
```

Both `require("./foo")` and `import { x } from "./foo"` resolve to the
same module instance per Run; esbuild rewrites `import` to
`require` at transpile time.

## 9. TypeScript support

TS is transpiled by esbuild in-process. There is no `tsc`, no
`tsconfig.json`, no type-checking — types are erased and the resulting
JS is what executes. Practical implications:

- All TS syntax esbuild supports is allowed: type annotations, `interface`,
  `type`, generics, enums (compiled to objects), namespaces, decorators
  (legacy & TC39 stage 3).
- `import type { X } from "y"` is stripped entirely (no runtime require).
- TS errors are *not* surfaced as build errors — only esbuild parse
  errors are. Run a separate `tsc --noEmit` in CI if you want type
  enforcement.
- `tsconfig.json` is **not** consulted.

`.tsx` and `.jsx` are supported via esbuild's `LoaderTSX` / `LoaderJSX`,
including for required modules resolved by the engine's extension
fallback. JSX has no React runtime in scope by default — either set
the factory at the file level with an `@jsx` pragma:

```tsx
/** @jsx h */
function h(tag: string, props: any, ...children: any[]) {
  return { tag, props: props ?? {}, children };
}
export const Box = (label: string) => <div className="box">{label}</div>;
```

…or provide a `React.createElement`-compatible binding from Go and
rely on esbuild's default pragma. ESM `export default <value>` also
works through the entry-script rewriter's `__esModule ? .default : m`
unwrap, so `import answer from "./mod"` resolves to the default
export.

## 10. Top-level `await`

Top-level `await` works in entry scripts:

```ts
const r = await net.http.get("https://example.com");
runtime.log(r.status);
```

How it works under the hood: esbuild rejects top-level await with the
`cjs` output format, so the engine transpiles the *entry* script with
`format=esm`, then rewrites `import` statements to `require()` calls and
wraps the rest of the body in:

```js
;(async () => { /* your transpiled body */ })().then(__resolve, __reject);
```

`__resolve` and `__reject` are bindings the engine sets on the runtime
to capture the final settlement. *Required-module* code (anything loaded
through `require`) is transpiled with `format=cjs` and does **not**
support top-level await — wrap in `(async () => …)()` yourself if you
need it inside a module.

## 11. Module resolution

The engine plugs a custom source loader into `goja_nodejs/require` that
adds:

1. Direct path: if the requested file exists on disk, use it. `.ts` /
   `.tsx` files are transpiled on the fly.
2. **Extension fallback** when the request has no extension: try `.ts`,
   `.tsx`, `.js`, `.cjs`, `.mjs`, `.json` in that order.
3. **`.js` → `.ts` swap**: if a path ending in `.js` / `.cjs` / `.mjs`
   is requested but the literal file doesn't exist, the same path
   ending in `.ts` (or `.tsx`) is tried. Handles `package.json` `main`
   fields that point at compiled output where only the TypeScript
   source is on disk.
4. **`package.json` `source` preference**: when reading a
   `package.json`, if a `source` field is present and points at an
   existing `.ts`/`.tsx` file, `main` is rewritten to that path before
   the registry consumes it. Matches the convention used by parcel,
   microbundle, and similar bundlers to mark the TS source of truth.
5. **Directory index**: if the resolved path is a directory, try
   `index.ts`, `index.tsx`, `index.js`.

The base directory follows Node's CommonJS rules — relative imports
resolve against the requiring module's directory, courtesy of
goja_nodejs.

```ts
// All resolve to ./helpers/assert.ts:
import { check } from "./helpers/assert";
import { check } from "./helpers/assert.ts";
const { check } = require("./helpers/assert");
const { check } = require("./helpers/assert.js");
```

JSON imports work out of the box via either the ESM-style default
import (esbuild rewrites it to a require) or a direct `require`:

```ts
import data from "./data.json";
const r = require("./data.json");
```

`node_modules` lookup *works* via the upstream registry, but the source
loader doesn't currently transpile through that path — keep TS helpers
under `ScriptRoot`.

## 12. Timeouts and cancellation

Two mechanisms interrupt a running script:

- `Options.Timeout` — wall-clock limit per `Run`. Exceeding it returns
  `scriptengine.ErrScriptTimeout`.
- `ctx` passed to `Run`/`RunFile` — cancelling the context returns
  `ctx.Err()` (typically `context.Canceled` or
  `context.DeadlineExceeded`).

Both call `vm.Interrupt(...)` and `loop.Terminate()` from a watcher
goroutine. This works for **sync** JS — including `while(true){}` —
because the goja VM checks the interrupt flag between bytecode steps. A
host Go binding that's blocked in cgo or a long syscall isn't
interruptible mid-call; if you write such a binding, plumb the engine
`ctx` through to it yourself.

```go
eng := scriptengine.New(scriptengine.Options{Timeout: 200 * time.Millisecond})
_, err := eng.Run(ctx, "loop.ts", `while (true) {}`)
// err is scriptengine.ErrScriptTimeout, returned ~200-300ms after Run.
```

## 13. Error semantics

- A Go binding returning `(T, error)` automatically throws as a JS
  exception when `err != nil`. The exception's `.message` is `err.Error()`.

  ```ts
  try { mayFail(); } catch (e) { runtime.log(String(e)); }
  ```

- A Go binding that `panic`s with a `*goja.Object` (e.g.
  `vm.NewGoError(...)`) does the same.

- A script throwing a JS error out of the top level surfaces from
  `Run` as a Go `error` whose message is the JS error's `.message`.

- Timeout errors satisfy `errors.Is(err, scriptengine.ErrScriptTimeout)`.
  Cancellation errors satisfy `errors.Is(err, context.Canceled)` or
  `context.DeadlineExceeded`.

## 14. Type generation (.d.ts)

`Engine.WriteTypes(w)` walks the registered bindings and emits a
TypeScript declaration file. The mapping table (abbreviated):

| Go type | TS type |
|---|---|
| `string` | `string` |
| any numeric / `float64` | `number` |
| `bool` | `boolean` |
| `[]T` | `T[]`; `[]byte` → `Uint8Array` |
| `map[string]T` | `Record<string, T>` |
| `*T` | `T` (transparent unwrap) |
| `error` (single return) | omitted (collapses to `void`) |
| `(T, error)` (tuple return) | `T` |
| `interface{}` / `any` | `unknown` |
| `goja.FunctionCall` parameter | folded into `(...args: unknown[])` |
| `goja.Value` return | `unknown` |
| `scriptengine.AsyncBinding` value (from `PromisifyAsync[T]`) | `Promise<T>` |
| self-referential types | `unknown` (cycle detection bails after depth 4) |

```bash
sercon --emit-dts sercon.d.ts
```

In your editor, place the resulting `.d.ts` next to your scripts (or
reference it via `/// <reference path="..." />`) for autocomplete.

### JSDoc comments on declarations

The emitter pulls JSDoc strings from the engine's doc map (set via
`Engine.SetDocs` / `Engine.SetMemberDocs`) and renders them above
each declaration. Single-line docs collapse to `/** … */`; multi-line
docs expand to a standard `* `-prefixed block. Bindings without a
doc entry emit no JSDoc — the emitter never inserts empty
placeholders. Docs are looked up by the same dotted path the
binding was registered under, so namespace members get hover text
too:

```ts
// AUTO-GENERATED by scriptengine.Engine.WriteTypes — do not edit by hand.

/** Hashing primitives. */
declare const crypto: {
  hash: {
    /** SHA-256 hex digest of a UTF-8 input. */
    sha256(...args: unknown[]): string;
    /** BLAKE3 hex digest (32-byte output, lukechampine.com/blake3). */
    blake3(...args: unknown[]): string;
    // …
  };
  // …
};
```

The CLI's reserved-global surface ships with a curated doc map; see
`cmd/sercon/docs.go` for the source of truth and add new
entries there when adding new bindings (the lockstep rule keeps
`--examples` / MANUAL / `sercon.d.ts` in sync, and the doc map is now
the seventh artifact in that chain).

## 15. Limitations and gotchas

- **No native ES modules at runtime.** All `import` is rewritten to
  `require` by esbuild. Side-effect-only imports run the module body
  exactly once per Run.
- **No top-level await inside required modules.** Wrap with
  `(async () => … )()` inside the module if you need it.
- **No `tsconfig.json` honoured.** Module resolution and type checking
  are independent from any project-level TS config.
- **Per-Run runtime.** Side effects on `globalThis` do not persist
  between calls to `Run` on the same `Engine`. The require *compile*
  cache is shared; module *exports* are not.
- **Multi-line ESM imports may stress the entry rewriter.** The current
  rewriter is line-based and handles the common forms; especially gnarly
  inputs (comments inside the spec list, very unusual whitespace) fall
  back to executing as-is, which may throw.
- **`RegisterConstructor` is d.ts-only today.** At runtime it behaves
  like `Register`. True `new`-able constructor semantics are on the
  roadmap.
- **HTTP bindings are real network calls.** `net.http.*` uses
  `net/http` with a 5s timeout. They are not mockable from JS.

See [OUT-OF-SCOPE.md](./OUT-OF-SCOPE.md) for the active backlog of
deferred ideas.

---

*This manual covers sercon v0.25.0. Whenever you add, remove, or change a <!-- x-release-please-version -->
flag, a binding, or the script API, update this file alongside the help
screen (`--help`), the examples walkthrough (`--examples`), and the
`CHANGELOG.md`.*
