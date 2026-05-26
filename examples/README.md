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
| `charset.ts` | `api.text.detect/encode/decode` — 5-charset round-trip + Latin-1 detection. |
| `checkdigit.ts` | `api.checkdigit.validate/compute/inspect` over Luhn, ISBN-10/13, EAN-13/8, UPC-A. |
| `archive.ts` | `api.archive.create/extract` round-trip over zip / tar / tar.gz. |
| `diff.ts` | `api.diff.compare` — unified diff of two text inputs. |
| `jq.ts` | `api.jq.query/queryAll` — jq filters over JS data structures. |
| `exec-shell.ts` | `api.exec.shell` — subprocess runner; POSIX-only (uses `/bin/echo` / `/usr/bin/tr` / `sleep`). |
| `exec-http.ts` | `api.exec.http` — recon-with-curl-fallback HTTP client; hits httpbin.org (not in CI). |
| `http-request.ts` | `api.http.request` — full HTTP client (headers/body/timeout/retry/auth/redirect) over net/http; hits httpbin.org (not in CI). |
| `browser.ts` | `api.browser.*` — stateful HTTP session (cookie jar + replayed headers); hits httpbin.org (not in CI). |
| `git.ts` | `api.git.*` — branch / isClean / revParse / status / add / commit / log / diffStat / runText; uses a throwaway temp repo so the host checkout is untouched. |
| `gh.ts` | `api.gh.*` — authStatus / prList / repoView; gracefully degrades when gh is missing or unauthenticated (not in CI for that reason). |
| `preg.ts` | `api.preg.*` — PHP-style `/pattern/flags` syntax over Go's RE2: match / matchAll / replace; demonstrates supported i/m/s flags and the clean error for unsupported flags. |
| `preg2.ts` | `api.preg2.*` — PCRE engine (dlclark/regexp2): lookahead, lookbehind, backreferences, the `x` flag. Same shape as `api.preg` but no linear-time guarantee. |
| `jwt.ts` | `api.jwt.*` — sign / view / validate (HMAC only: HS256/HS384/HS512); demonstrates the resolve-on-failure contract for bad signature / expired / audience mismatch. |
| `encrypt.ts` | `api.encrypt.*` — age X25519 keygen / encrypt / decrypt. Single + multi-recipient round-trip, public/private cross-check, binary payloads. |
| `sqlite.ts` | `api.sqlite.*` — in-memory SQLite handle: schema, parameterised insert/query/queryValue, mutations with rowsAffected, BLOB round-trip, close. |
| `redis.ts` | `api.redis.*` — RESP client (do/ping/close); gracefully degrades without a server. |
| `memcached.ts` | `api.memcached.*` — get/set/delete; gracefully degrades without a server. |
| `ldap.ts` | `api.ldap.*` — anonymous bind + rootDSE / search; gracefully degrades, hits a public test LDAP (not in CI). |
| `dict.ts` | `api.dict.*` — RFC 2229 define / match; gracefully degrades, hits dict.org (not in CI). |
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
