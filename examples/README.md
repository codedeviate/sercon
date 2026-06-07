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
| `smoke.ts` | `log` and `assert.*` |
| `async.ts` | `http.get`, `time.sleep`, top-level `await`, `import` from a sibling helper |
| `argv.ts` | `runtime.argv` — the per-script argument vector (program / script / user args). |
| `console.ts` | `console.*` — browser/Node-compat shim (log/info/debug → stdout, warn/error → stderr). |
| `tui-update.ts` | `tui.*` — multi-pane TUI with two parallel subprocesses streaming output into separate panes. Manual run only (takes over the terminal); not in `make demo`. |
| `hash.ts` | `hash.*` — all nine algorithms |
| `strings.ts` | `str.*` — trim/pad/reverse/strip/encode/sprintf |
| `path-and-time.ts` | `path.*` and `time.format` (strftime tokens, IANA zones) |
| `default-export.ts` | ESM `export default` interop via the entry rewriter |
| `export-default.ts` | `export default` — the entry's default export becomes Run's value, which the CLI prints as JSON to stdout. |
| `tsx-demo.ts` | `.tsx` module loading with a local `@jsx h` factory |
| `json-import.ts` | `import data from "./data.json"` and `require("./data.json")` |
| `pkg-resolution.ts` | `package.json` `source` field preferred over `main` |
| `net-probe.ts` | `net.tcp/dns/tls/ntp/whois` — hits the real network (not in CI). |
| `traceroute.ts` | `net.probe.traceroute` — trace the path to a host (icmp/udp/tcp); needs root, so the demo handles the privilege rejection and exits 0. |
| `raw.ts` | `net.raw` raw packet engine: craft a SYN and read the reply (degrades cleanly without root). |
| `email-auth.ts` | `email.spf/dmarc/mtaSts/tlsRpt/bimi/all` — hits real DNS (not in CI). |
| `compression.ts` | `compression.compress/decompress` round-trip across all 9 algos. |
| `barcode.ts` | `barcode.encode` over all 10 supported symbologies. |
| `charset.ts` | `text.detect/encode/decode` — 5-charset round-trip + Latin-1 detection. |
| `checkdigit.ts` | `checkdigit.validate/compute/inspect` over Luhn, ISBN-10/13, EAN-13/8, UPC-A. |
| `dump-codec.ts` | `codec.php.*` / `codec.perl.*` — round-trip a value through PHP serialize / var_export / var_dump and Perl Data::Dumper. |
| `codec-xml.ts` | `codec.xml.encode` / `decode` — value ↔ XML (@-attributes, #text, arrays as repeated siblings). Round-trips an object and self-checks. |
| `archive.ts` | `archive.create/extract` round-trip over zip / tar / tar.gz. |
| `diff.ts` | `diff.compare` — unified diff of two text inputs. |
| `jq.ts` | `jq.query/queryAll` — jq filters over JS data structures. |
| `exec-shell.ts` | `exec.shell` — subprocess runner; POSIX-only (uses `/bin/echo` / `/usr/bin/tr` / `sleep`). |
| `exec-stream.ts` | `services.exec.stream` — run a subprocess and stream its stdout/stderr to a callback line by line; resolves `{ exitCode, success, durationMs }` on exit. Runs a finite `echo` command and self-checks. |
| `exec-http.ts` | `exec.http` — recon-with-curl-fallback HTTP client; hits httpbin.org (not in CI). |
| `http-request.ts` | `http.request` — full HTTP client (headers/body/timeout/retry/auth/redirect) over net/http; hits httpbin.org (not in CI). |
| `browser.ts` | `browser.*` — stateful HTTP session (cookie jar + replayed headers); hits httpbin.org (not in CI). |
| `git.ts` | `git.*` — branch / isClean / revParse / status / add / commit / log / diffStat / runText; uses a throwaway temp repo so the host checkout is untouched. |
| `gh.ts` | `gh.*` — authStatus / prList / repoView; gracefully degrades when gh is missing or unauthenticated (not in CI for that reason). |
| `preg.ts` | `preg.*` — PHP-style `/pattern/flags` syntax over Go's RE2: match / matchAll / replace; demonstrates supported i/m/s flags and the clean error for unsupported flags. |
| `preg2.ts` | `preg2.*` — PCRE engine (dlclark/regexp2): lookahead, lookbehind, backreferences, the `x` flag. Same shape as `preg` but no linear-time guarantee. |
| `jwt.ts` | `jwt.*` — sign / view / validate (HMAC only: HS256/HS384/HS512); demonstrates the resolve-on-failure contract for bad signature / expired / audience mismatch. |
| `encrypt.ts` | `encrypt.*` — age X25519 keygen / encrypt / decrypt. Single + multi-recipient round-trip, public/private cross-check, binary payloads. |
| `sqlite.ts` | `sqlite.*` — in-memory SQLite handle: schema, parameterised insert/query/queryValue, mutations with rowsAffected, BLOB round-trip, close. |
| `redis.ts` | `redis.*` — RESP client (do/ping/close); gracefully degrades without a server. |
| `memcached.ts` | `memcached.*` — get/set/delete; gracefully degrades without a server. |
| `ldap.ts` | `ldap.*` — anonymous bind + rootDSE / search; gracefully degrades, hits a public test LDAP (not in CI). |
| `dict.ts` | `dict.*` — RFC 2229 define / match; gracefully degrades, hits dict.org (not in CI). |
| `ai.ts` | `ai.*` — providers() + send() over claude/codex/copilot/gemini; gracefully degrades without a provider. |
| `agent-browser-core.ts` | `services.agentBrowser.*` — launch/open/fill/get/isVisible/snapshot/close against a `data:` URL; self-skips when the agent-browser CLI is not on PATH. |
| `agent-browser-capture.ts` | `services.agentBrowser` Phase 2 — `setDefaultOptions`/`defaultOptions`, `set.viewport`, `screenshot` (bytes + file), `pdf` (bytes), and the one-shot `eval` shortcut; self-skips when the CLI is absent. |
| `server-http.ts` | `server.http.listen` — minimal HTTP server with routing and middleware (logger); self-tests routes via `net.http.get` then closes. |
| `server-static.ts` | `server.http.static` — mount a directory tree at a URL prefix; self-tests via `net.http.get` then closes. |
| `server-ws.ts` | `res.upgradeWebSocket` — upgrade an HTTP request to a WebSocket; iterates frames via `for await`, echoes text frames back; self-tests the handshake via `net.probe.wss` then closes. |
| `server-smtp.ts` | SMTP server (`server.smtp.listen`) + outbound sender (`net.email.send`) round-trip; binds a port, sends a message to itself, captures it in `onData`, asserts subject + body. |
| `server-tcp.ts` | Raw TCP server (`server.tcp.listen`) + `net.tcp.connect` client echo round-trip; binds an ephemeral port, echoes bytes from the connection handler, asserts the echo matches, then closes. Fully offline. |
| `server-icmp.ts` | `server.icmp.listen` — raw ICMP listener with reply(); needs root, so the demo handles the privilege rejection and exits 0. |
| `capture-file.ts` | `net.capture` — list interfaces, then a `toFile`/`openFile` pcap round-trip on a hand-built UDP frame; asserts the decoded `udp.dstPort`. Fully offline (live `net.capture.open` is privileged and Linux/macOS-only, shown in comments). |
| `net-sockets.ts` | `net.tcp.connect` / `net.udp.open` / `net.icmp.open` — long-lived client sockets with the push/callback read model (`onData`/`onMessage`, `onClose`, `onError`, `close`). Runs an offline UDP loopback round-trip; TCP + ICMP shown in comments. |
| `tui-keys.ts` | `tui.*` — autoscroll (panes follow the tail), `{ autoscroll: false }` per-pane opt-out, `{ mouse: true }` root option, `tui.onKey` persistent callback, `tui.waitKey` awaitable keypress. Falls back to prefixed lines in non-TTY. |
| `pty-color.ts` | `services.exec.shell` `{ pty: true }` — run a command under a pseudo-terminal so TTY-gated color output reaches the pane; contrasts with the same command without `pty` (monochrome). Unix only; falls back to pipes on Windows. **Run separately in a real terminal** — headless the PTY pane renders blank (CRLF), so the color contrast only shows on a TTY. |
| `hang.ts` | Timeout demo; intentionally non-zero exit. Run separately. |

Helpers under `helpers/` are sibling-imported by the above; they aren't
runnable on their own.

## Adding a new binding

Bindings are wired in `cmd/sercon/main.go` inside `registerSurface`. The
surface is a set of top-level globals (`runtime`, `crypto`, `text`, `codec`,
`fs`, `net`, `db`, `server`, `services`, `tui`), each registered via
`RegisterNamespaceFactory` so every `Run` gets its own VM + event loop in
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

## Regenerating `sercon.d.ts`

After changing bindings, emit a fresh declaration file:

```
go run ./cmd/sercon -emit-dts examples/scripts/sercon.d.ts
```

Editors that pick up sibling `.d.ts` files (VS Code with the TS plugin, for
example) will then show types and signatures for the top-level globals
(`fs`, `http`, `services`, `codec`, `runtime`, `tui`, …) used inside
the example scripts.
