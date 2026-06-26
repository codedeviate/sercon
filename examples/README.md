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
| `deadline.ts` | `runtime.setDeadline`/`clearDeadline`/`getDeadline` — adjust or remove the script's own running timeout at runtime. |
| `secrets.ts` | `runtime.secrets` set/get/delete round-trip in the OS keystore (prefix-namespaced); self-skips when no backend. |
| `clipboard.ts` | `runtime.clipboard` — host system clipboard text round-trip (write then read); self-skips when no clipboard backend is on PATH. |
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
| `valkey.ts` | `valkey.*` — RESP client for Valkey (the Redis fork); same surface as redis, accepts `valkey://` URLs; gracefully degrades without a server. |
| `memcached.ts` | `memcached.*` — get/set/delete; gracefully degrades without a server. |
| `ldap.ts` | `ldap.*` — anonymous bind + rootDSE / search; gracefully degrades, hits a public test LDAP (not in CI). |
| `dict.ts` | `dict.*` — RFC 2229 define / match; gracefully degrades, hits dict.org (not in CI). |
| `ai.ts` | `ai.*` — providers() + send() over claude/codex/copilot/gemini; gracefully degrades without a provider. |
| `agent-browser-core.ts` | `services.agentBrowser.*` — launch/open/fill/get/isVisible/snapshot/close against a `data:` URL; self-skips when the agent-browser CLI is not on PATH. |
| `agent-browser-capture.ts` | `services.agentBrowser` Phase 2 — `setDefaultOptions`/`defaultOptions`, `set.viewport`, `screenshot` (bytes + file), `pdf` (bytes), and the one-shot `eval` shortcut; self-skips when the CLI is absent. |
| `agent-browser-state.ts` | `services.agentBrowser` Phase 3 — `network.requests`, `cookies.get`/`set`, `storage.local.set`/`get`, `tabs.new`/`list`/`close`, `diff.snapshot`; uses a `data:` URL; self-skips when the agent-browser CLI is not on PATH. |
| `agent-browser-advanced.ts` | `services.agentBrowser` Phase 4 — `vitals()`, `cmd(command, ...args)` escape hatch, `batch(cmds)`, and `auth.list()` vault listing; uses a `data:` URL; self-skips when the agent-browser CLI is not on PATH. |
| `agent-browser-frames.ts` | `services.agentBrowser` `frame(target)` — switch into an iframe by CSS selector (single level; resolves against the main document) and read an element inside it, then `frame("main")` back; self-skips without the CLI. |
| `webdriver.ts` | `services.webdriver.*` — W3C WebDriver client v1: `available` gate, `connect` (headless Chrome), `get`/`title`/`find`/`text`/`sendKeys`/`getAttribute`/`executeScript`/`screenshot`/`quit`; uses a `data:` URL; self-skips when no chromedriver/geckodriver is on PATH. |
| `webdriver-advanced.ts` | `services.webdriver` Phase 2: windows/tabs (`newWindow`/`switchToWindow`/`closeWindow`), frames (`switchToFrame` by index or element handle, `switchToParentFrame`, `switchToDefaultContent`), alerts (`alertText`/`acceptAlert`, with `unhandledPromptBehavior: "ignore"`), window rect (`setWindowRect`/`getWindowRect`), action chains (`hover`/`dragAndDrop`), and `executeScript` returning element handles; uses `data:` URLs; self-skips when no chromedriver/geckodriver is on PATH. |
| `webdriver-frames.ts` | `services.webdriver` nested-frame addressing — `frameChain([...])` reaches a deeply nested iframe in one call and the selector form of `switchToFrame` scopes queries to a frame; nested same-origin `data:` iframes (same W3C path as cross-origin); self-skips without a driver. |
| `webdriver-wait-click.ts` | `services.webdriver` `clickWhenReady` + `waitFor({enabled})` — reliably wait for an async-rendered/enabled button inside a cross-origin iframe, then trusted-click it; self-skips without a driver. |
| `webdriver-cdp-click.ts` | `services.webdriver` `cdpClick` + raw `cdp()` — a trusted click on a button inside a nested cross-origin iframe (the Klarna "Pay order" case), via CDP `DOM.getContentQuads` + `Input.dispatchMouseEvent`. Chrome-only. |
| `webdriver-cdp-oopif.ts` | `services.webdriver` `cdpClick` across a true out-of-process iframe (cross-*site*), plus `targets()`/`attach()` — routes input over a browser-level CDP connection (the Klarna "Pay order" case). Chrome-only. |
| `fs-report.ts` | `fs` file API (`writeText`/`writeBytes`/`readText`/`readBytes`/`mkdir`/`exists`/`remove`/`stat`) — builds an illustrated per-step screenshot report; captures real screenshots when a WebDriver is present, else records steps without images. |
| `typst.ts` | `services.typst` — compile inline Typst to PDF bytes + PNG file, version/fonts/query; self-skips without the typst CLI. |
| `pdf-extract.ts` | Render/extract PDFs via poppler (`services.pdf`): `info`, `toImage` (page 1 → PNG bytes), `toText`; self-skips without poppler. |
| `doctor.ts` | `services.doctor()` — report external tool requirements (installed/version/conflict) and assert required features (e.g. `["git"]`); `--doctor` is the CLI form. |
| `server-http.ts` | `server.http.listen` — minimal HTTP server with routing and middleware (logger); self-tests routes via `net.http.get` then closes. |
| `server-static.ts` | `server.http.static` — mount a directory tree at a URL prefix; self-tests via `net.http.get` then closes. |
| `server-ws.ts` | `res.upgradeWebSocket` — upgrade an HTTP request to a WebSocket; iterates frames via `for await`, echoes text frames back; self-tests the handshake via `net.probe.wss` then closes. |
| `server-sse.ts` | `res.sse` — one-way Server-Sent Events (`text/event-stream`) stream; sends a string event, two named JSON events with ids, then closes server-side; self-tests the accumulated frames via `net.http.request`. |
| `server-smtp.ts` | SMTP server (`server.smtp.listen`) + outbound sender (`net.email.send`) round-trip; binds a port, sends a message to itself, captures it in `onData`, asserts subject + body. |
| `server-tcp.ts` | Raw TCP server (`server.tcp.listen`) + `net.tcp.connect` client echo round-trip; binds an ephemeral port, echoes bytes from the connection handler, asserts the echo matches, then closes. Fully offline. |
| `server-icmp.ts` | `server.icmp.listen` — raw ICMP listener with reply(); needs root, so the demo handles the privilege rejection and exits 0. |
| `capture-file.ts` | `net.capture` — list interfaces, then a `toFile`/`openFile` pcap round-trip on a hand-built UDP frame; asserts the decoded `udp.dstPort`. Fully offline (live `net.capture.open` is privileged and Linux/macOS-only, shown in comments). |
| `net-sockets.ts` | `net.tcp.connect` / `net.udp.open` / `net.icmp.open` — long-lived client sockets with the push/callback read model (`onData`/`onMessage`, `onClose`, `onError`, `close`). Runs an offline UDP loopback round-trip; TCP + ICMP shown in comments. |
| `load.ts` | `net.load.http` — authorized HTTP load/resilience self-test against a loopback server; latency percentiles + error rate, asserts a clean run. |
| `image.ts` | `image` — decode/transform/encode: resize (aspect), grayscale, blur, crop, save/open round-trip, PNG + WebP output. |
| `image-anim.ts` | `image.decodeFrames` / `image.encodeFrames` — encode a 2-frame GIF and APNG from an embedded PNG, decode both back, assert frame counts and timing; also demonstrates non-animated PNG → single frame. Fully offline and self-contained. |
| `exif.ts` | `image.exif` — read/write/replace/clear EXIF metadata: builds a JPEG in-memory, round-trips EXIF through replace/write/clear, asserts each step. Fully offline and self-contained. |
| `tui-keys.ts` | `tui.*` — autoscroll (panes follow the tail), `{ autoscroll: false }` per-pane opt-out, `{ mouse: true }` root option, `tui.onKey` persistent callback, `tui.waitKey` awaitable keypress. Falls back to prefixed lines in non-TTY. |
| `pty-color.ts` | `services.exec.shell` `{ pty: true }` — run a command under a pseudo-terminal so TTY-gated color output reaches the pane; contrasts with the same command without `pty` (monochrome). Unix only; falls back to pipes on Windows. **Run separately in a real terminal** — headless the PTY pane renders blank (CRLF), so the color contrast only shows on a TTY. |
| `hang.ts` | Timeout demo; intentionally non-zero exit. Run separately. |
| `paymentproviders-kcov3.ts` | Bundled `paymentproviders` library — KCO v3 payment lifecycle (`getPayment`/`capturePayment`/`refundPayment`/`cancelPayment`) over a local mock; live check self-skips without `KCO_*` env. |
| `paymentproviders-nets.ts` | Bundled `paymentproviders` — `netsv1` (Nexi/Nets Checkout v1) create/get over a local mock; live check self-skips without `NETS_SECRET_KEY`. |
| `paymentproviders-svea.ts` | Bundled `paymentproviders` — `sveacheckout2` (Svea Checkout) create over a mock that verifies the SHA512 signature; live check self-skips without `SCO_*`. |
| `paymentproviders-qliro.ts` | Bundled `paymentproviders` — `qlirov2` (Qliro One) create over a mock checking the `Qliro` auth header; live check self-skips without `QLIRO_*`. |
| `paymentproviders-swedbankpay.ts` | Bundled `paymentproviders` — `swedbankpayv3`/`swedbankpayv2` (Bearer + HAL operations): create a payment order, then `capturePayment` resolves the `capture` operation href and POSTs to it. Live check self-skips without `SWEDBANKPAY_*`. |
| `web-html.ts` | `web.html` — lenient HTML parse with CSS (`find`/`findAll`) + XPath (`xpath`/`xpathAll`) and chainable nodes (`text`/`attr`/…). Offline always; live `web.html.load` self-skips without a network. |
| `web-feed.ts` | `web.feed` — RSS/Atom/JSON feeds normalized to one model with a `.raw` escape hatch. Offline always; live `web.feed.load` self-skips without a network. |
| `web-sitemap.ts` | `web.sitemap` — urlset/sitemapindex parse, gzip, and `{expand:true}` recursion. Offline always; live `web.sitemap.load` self-skips without a network. |

Helpers under `helpers/` are sibling-imported by the above; they aren't
runnable on their own.

## Advanced examples (`advanced/`)

In-depth, end-to-end scripts that compose multiple bindings into realistic
workflows. Self-contained ones run in `make demo`; ones needing a driver or
external service self-skip.

| Script | Demonstrates |
| --- | --- |
| `advanced/load-resilience.ts` | Resilience / load self-test — starts a loopback HTTP server, drives it at rising concurrency (4/16/32), reports latency p50/p95/max + error rate + throughput per level, and asserts the service stays healthy after the burst. Self-contained and offline; comments show how to repoint `TARGET` at a real endpoint you're authorized to test. |
| `advanced/http-api.ts` | Full HTTP API on `server.http.listen` — middleware chain (request logger + bearer-token auth → 401 + error-catcher), CRUD over an in-memory store, a `/health` route; self-tests every path (incl. 401/201/404) then closes. Self-contained. |
| `advanced/smtp-pipeline.ts` | `server.smtp.listen` receive → parse headers/body/attachment → `net.email.send` reply, full loopback round-trip with assertions. Self-contained. |
| `advanced/tcp-proxy.ts` | A TCP proxy: `server.tcp.listen` relays a client connection to an upstream echo server (also started in-script) via `net.tcp.connect`, both directions; asserts the round-trip. Self-contained. |
| `advanced/https-server.ts` | `server.https.listen` with the `cert: "self-signed"` shortcut (ephemeral in-process cert, no files); verifies the served cert via `net.probe.tls` (skip-verify). Self-contained. |
| `advanced/sqlite-etl.ts` | `db.sqlite` ETL — schema + bulk insert in a transaction + prepared statements + `GROUP BY` aggregate → export to JSON and `codec.xml`. Self-contained (in CI). |
| `advanced/crypto-pipeline.ts` | Secure-payload workflow — `hash.sha256` → `jwt` sign (HS256, hash claim) → `encrypt` (age) → base64 transport, then the full reverse + verify; plus wrong-key negative checks. Self-contained (in CI). |
| `advanced/codec-interop.ts` | Round-trips a value through `codec.php` / `codec.perl` / `codec.xml` / JSON + a `compression` pass, with a per-codec preservation table. Self-contained (in CI). |
| `advanced/packet-analysis.ts` | Hand-builds Ethernet/IP frames → `net.capture.toFile` → `openFile` → decode → per-protocol counts + top destination ports; asserts the tallies. Self-contained, offline. |
| `advanced/recon-host-report.ts` | Multi-binding host recon — `net.probe` dns/tcp/tls + HTTP headers → a structured report. Hits the real network; **self-skips cleanly when offline** (not in CI). Edit the target to a host you're authorized to probe. |
| `advanced/webdriver-login-flow.ts` | Complete WebDriver UI test — drive a login form (fill/submit/wait/assert) + screenshot; self-skips when no chromedriver/geckodriver is on PATH. |
| `advanced/webdriver-actions.ts` | Low-level W3C input via `performActions` — a pointer sequence (viewport coords) that clicks a fixed box, then a key sequence that types into a focused input; plus best-effort cookies. Complements the helper-based `webdriver-advanced.ts`. Self-skips without a driver. |
| `advanced/webdriver-grid.ts` | Drive a **remote** WebDriver / Selenium Grid endpoint via `connect({url, capabilities})`, gated on `SERCON_WEBDRIVER_GRID_URL` + `services.webdriver.probe`; self-skips cleanly when the env var is unset or the endpoint isn't ready. |
| `advanced/sqlite-migration.ts` | Idempotent versioned schema migration (a `schema_version` gate, v1 create + v2 `ALTER TABLE`/lookup) then a three-table JOIN (`orders ⋈ customers ⋈ regions`) with `GROUP BY` aggregation; asserts per-region totals and per-tier counts. Self-contained (in CI). |
| `advanced/sse-stream.ts` | Live-metrics stream over `res.sse` — pushes JSON `tick` events on a timer plus a named `alert`, stops the timer via `stream.closed`, and closes after N ticks; self-tests the accumulated `text/event-stream` via a buffered `net.http.request`. Self-contained, offline. |
| `advanced/tui-dashboard.ts` | Live multi-pane `tui` dashboard — streams a bounded subprocess into one pane + periodic status ticks in another; runs a fixed number of cycles then exits. **Manual run** (takes over a real terminal); falls back to prefixed lines in non-TTY; not in `make demo`. |

## DevShop flows (`sws6/`)

Reality-based `services.webdriver` flows against the **internal** dev storefront
`http://dev-shop.sws.local` — building blocks for internal UI test flows. Every
script self-skips when no WebDriver driver is on PATH **or** the host is
unreachable, so they're safe to run anywhere; they target an internal host and
are **not** in `make demo`/CI. `sws6/shop.ts` is a shared helper module (host,
persona, `connectShop()`, env-sourced secrets), not a runnable demo.

**Three things make these work:**

1. **Session cookie** — the shop withholds its `swssid` session cookie from the
   default `HeadlessChrome` user-agent, so `connectShop()` spoofs a normal
   desktop UA. Without it the basket never persists and login can't stick.
2. **Secrets in the environment** — login credentials and payment test data are
   read via `runtime.env.get`, never hard-coded. Copy `sws6/.env.example` to
   `sws6/.env` (gitignored), fill it in, and load it before running:
   `set -a; source examples/scripts/sws6/.env; set +a`. Scripts that need a
   value self-skip with a message when it's unset.
3. **A longer `-timeout`** — these drive a real headless browser (launch Chrome,
   load pages, fill forms, poll, quit), which overruns sercon's default 10s
   per-script timeout. Run them with `-timeout 30s` or they intermittently fail
   with `script timeout`:
   `./sercon -timeout 30s examples/scripts/sws6/login.ts`.

All the storefront flows (search, category, sort, login, add-to-cart,
view-cart) **work end-to-end and assert real results**. `checkout-payment`
reaches the payment provider's widget (e.g. the KCO iframe) and stops at a
**dry-run** — the final iframe/3DS/ngrok submission is best driven by the
Playwright-based `/devshop` skill (`/devshop buy … via <provider>`).

| Script | Demonstrates |
| --- | --- |
| `sws6/search.ts` | Search (`/en/search?q=watch`) → assert results → open the first product. |
| `sws6/browse-category.ts` | Browse a category (`/en/category/ladies`) → assert the product tiles → open the first. |
| `sws6/filter-sort.ts` | Change the category sort (`#sort-by-select`, server-side) → assert the listing reorders. |
| `sws6/login.ts` | Existing-customer login (`#existing-account-type-radio` → env email/password → `.login-action`); asserts the `/customer/logout` link appears. Needs `DEVSHOP_EMAIL`/`DEVSHOP_PASSWORD`. |
| `sws6/add-to-cart.ts` | Open a product, select all variants (`select.attribute-value-select`), click Buy, assert the header cart count reaches ≥1. |
| `sws6/view-cart.ts` | Add an item then open `/en/checkout`; assert the item is in the basket and read the order total. |
| `sws6/checkout-payment.ts` | Provider via `argv` (`kco`\|`sco`\|`nets`\|`qliro`, default `kco`): (optional login) → add → checkout → locate the provider UI → dry-run stop. Test data from env; full iframe/3DS payment → `/devshop`. |

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
