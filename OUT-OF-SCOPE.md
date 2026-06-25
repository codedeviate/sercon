# Out of scope / backlog

Outstanding ideas, follow-ups, and known gaps that are **not implemented**.
Each entry names the **reason** it's parked, so it's easy to re-promote when
the situation changes. Promotion from this list to a spec / plan / commit is
the only way these become "real" work — keep entries terse until then.

**Shipped work is not tracked here.** Once an item lands it moves out of this
file: the thematic capability history lives in [`HISTORY.md`](./HISTORY.md),
and per-version detail in [`CHANGELOG.md`](./CHANGELOG.md).

Library picks honour the project constraints: **pure Go, no cgo; stdlib
first; trustworthy and maintained; no heavy frameworks** — which is itself the
reason most items below stay parked (the only viable library isn't pure Go,
drags in heavy dependencies, or doesn't exist yet).

**Entry rule:** every item must have a viable path that satisfies those
constraints — a pure-Go implementation (now or plausibly later) **or** an
optional feature-detected external-CLI fallback (see *External-CLI fallbacks*
below). A capability whose *only* path is cgo linked into our binary can never
satisfy no-cgo, so it doesn't belong here — don't add it (or remove it if it
slipped in).

## Encoding / decoding / barcodes

- **PDF417 decoder.** v0.5.4 shipped what is now `codec.barcode.decode`
  over gozxing, which doesn't cover PDF417 (the encoder still works via
  boombuler). A pure-Go PDF417 reader would close the symmetry.
  **Reason:** no maintained pure-Go PDF417 reader exists — the realistic
  path is porting ZXing's Java PDF417 reader, which is a from-scratch
  effort with no demand signal. Re-promote if a library appears or
  someone actually needs PDF417 round-tripping.

## Databases

The native pure-Go SQL engines have all shipped (`db.sqlite`, `db.postgres`,
`db.mysql`, `db.mssql`, `db.clickhouse`, `db.oracle`, plus the non-SQL
`db.redis` / `db.memcached` / `db.ldap` / `db.dict` — see `HISTORY.md`).
Adding another `database/sql` engine is now a known, mechanical step (driver
import + DSN builder onto the shared handle in `cmd/sercon/db_sql.go`), so new
engines are promoted on demand rather than tracked here. The one parked below
has a concrete reason to wait.

- **Snowflake.** Fits the same shared-handle pattern, and its driver
  `github.com/snowflakedb/gosnowflake` is pure Go — but it drags in large
  AWS/Azure/GCS cloud SDKs, conflicting with the "no heavy frameworks"
  preference (not cgo). **Reason:** dependency weight, no demand signal.
  Re-promote if someone needs Snowflake and accepts the transitive-dependency
  cost — or reach it via a `snowsql` external-CLI fallback, which sidesteps
  the dep weight entirely.

(**ODBC connectivity** was removed: the only Go option,
`github.com/alexbrainman/odbc`, links unixODBC via **cgo** with no pure-Go
implementation and no clean CLI fallback — so it can never satisfy no-cgo. The
native pure-Go drivers already cover the engines people actually ask for.)

## Networking — servers

The server foundation (`LoopCallable` + `Engine.HoldRun`) and the HTTP/HTTPS,
SMTP, raw TCP/UDP, and raw ICMP listeners have all shipped (see `HISTORY.md`).
The remaining application-protocol servers are parked:

- **HTTP/3.** stdlib's HTTP server auto-upgrades HTTP/1.1 to HTTP/2
  over TLS; H2 ships with the HTTPS listener for free. H3 would need
  `quic-go` and explicit handling. **Reason:** new dependency without a
  current demand signal. Re-promote when there's a clear request.
- **Let's Encrypt autocert.** `golang.org/x/crypto/acme/autocert`
  brings stateful disk caching, renewal coordination, and a
  side-effecting registration step — useful for production
  deployments but heavy for an embeddable script engine.
  **Reason:** sercon is CLI-first and the cert is best owned by
  the supervisor (caddy, traefik, nginx in front). Defer until
  clear demand.
- **Server-side IMAP, FTP.** Planned as separate sub-spec cycles
  built on the `LoopCallable` + `HoldRun` foundation. Each remaining
  protocol gets its own brainstorm → plan → ship cycle. Promote
  once the spec lands.
- **Server-side POP3.** Same foundation as the above, but
  **Reason:** no mature pure-Go POP3 server library exists today.
  Re-promote when one appears or when there's enough demand to
  justify writing one in tree.

## Packet capture

Sniffing shipped in v0.24.0 as `net.capture` (live capture, pcap/pcapng
read+write, interface enumeration, layer decode — all pure-Go; see
`HISTORY.md`). The parked pieces:

- **Windows live capture.** The native route (Npcap) needs **cgo**, which is
  out. File read/write + decode already work on Windows; only live sniffing is
  stubbed. **Viable path:** an external-CLI fallback — shell out to
  `dumpcap` / `tshark` (Wireshark) and read the captured pcap with the
  existing pure-Go reader. **Reason parked:** waits on the External-CLI
  fallbacks mechanism; Linux/macOS live capture covers the common case today.
- **Kernel-level BPF filtering.** A post-decode tcpdump-*syntax* `filter`
  shipped in v0.25.0 (a pure-Go predicate evaluated in userspace after
  decode). What remains is true **kernel-level** drop: compiling the
  expression to a cBPF program (`x/net/bpf` can assemble instructions, but
  no pure-Go *expression→BPF compiler* exists — we'd write one) and
  attaching it at the socket. **Reason:** Linux could attach via
  `afpacket.SetBPF` / `SO_ATTACH_FILTER`, but macOS `bsdbpf` exposes no
  filter-attach API (would need forking it or raw `BIOCSETF`), so it's
  Linux-mostly + a from-scratch compiler. Re-promote if high-pps kernel
  drop is actually needed.
- **Deeper / exotic decode.** ARP, VLAN (802.1Q), DNS, and enriched TCP
  (window/checksum/options) now map to structured fields (see `HISTORY.md` /
  `CHANGELOG.md`). Remaining protocols (DHCPv4/v6, SCTP, GRE, deeper TLS/HTTP)
  still surface as `bytes` — extend `decodePacket` on demand following the same
  additive pattern.

## External-CLI fallbacks

**Settled pattern.** External-CLI bindings are **optional, feature-detected
fallbacks** for capabilities that have no trustworthy pure-Go path. The
established charter (codified via `services.pdf`):

- Namespaced under a capability-named `services.*` global (e.g. `services.pdf`,
  `services.typst`).
- Sync `available` boolean (over `exec.LookPath`) + `backend` string for
  introspection; per-binary `tools` map when multiple binaries are involved.
- **No shell.** All execs go through the shared `runTool` helper: no shell
  interpretation, `--` separator before path args to prevent flag injection,
  per-call timeout, and an output-cap to prevent memory overruns.
- **Each new tool requires explicit maintainer authorization** — listing a
  candidate here is not a green light. The static pure-Go binary stays fully
  functional without any of these; they only enrich behaviour when the tool
  is installed.

**Already wired (authorized):**
- **Clipboard tools** (`pbcopy`/`pbpaste`, `xclip`/`xsel`/`wl-clipboard`,
  `clip`/PowerShell `Get-Clipboard`) — back `runtime.clipboard` (text +
  PNG image), feature-detected.
- **poppler-utils** (`pdftoppm`/`pdftotext`/`pdftohtml`/`pdfinfo`) — backs
  `services.pdf.*` (render/extract PDFs: `info`, `toImage`, `toText`,
  `toHtml`). Shipped in v0.69.0; PDF render/extraction is now fully handled
  by this namespace.
- **typst** CLI — backs `services.typst.*`, refactored onto the shared
  `runTool` helper in the same release.

**Remaining candidates** (follow the `services.pdf` pattern when authorized):
- **LaTeX** — `pdflatex` / `xelatex` / `tectonic` for `.tex` → PDF.
- **ImageMagick / GraphicsMagick** — `magick` / `convert` for image
  transforms beyond the pure-Go image stack.
- **Ghostscript** (`gs`), **pandoc** (document conversion), **ffmpeg**
  (media) — same opt-in, feature-detected model.

**Residual clipboard items (still parked):** `runtime.clipboard` covers text
and PNG image I/O (macOS `pngpaste`/`osascript`, Linux `wl-paste`/`xclip`,
Windows PowerShell). Remaining: non-PNG formats (JPEG/TIFF), RTF/HTML/file-list
clipboard, clipboard watching. Re-promote on demand.

- **`image` v2 (parked).** The v1 `image` global (shipped — decode/encode
  PNG/JPEG/GIF/TIFF/BMP/WebP, SVG rasterize-in, and a chainable raster `Image`
  handle) intentionally leaves a second wave parked:
  - **Animated GIF / APNG** — decode/encode all frames (v1 is first-frame only).
  - **EXIF auto-orient pixel transform** — rotate the raster pixel data according to the Orientation tag and strip the tag on save; the EXIF read/write API itself shipped in v0.70.0.
  - **Custom convolution / morphology / arbitrary filters** — user-supplied
    kernels, erode/dilate, edge detect beyond the fixed `sharpen`/`blur`.
  - **SVG output** — v1 is rasterize-*in* only; emitting SVG is a different
    (vector) pipeline.
  - **ICC colour profiles** — profile-aware decode/encode + conversion.
  **Reason parked:** v1 covers the common reconnaissance/troubleshooting needs
  (resize/crop/convert/inspect); these add scope (extra deps, larger surface)
  better promoted on demand.

- **`services.typst` v2 (parked).** The v1 binding (shipped —
  `available`/`version`/`fonts`/`compile`/`query`, inline `source` or `.typ`
  `input` → PDF bytes or PDF/PNG/SVG to an `output` path) intentionally leaves a
  second wave parked:
  - **`typst watch`** — incremental recompile on file change; long-lived, so it
    needs `HoldRun` bookkeeping like the server listeners.
  - **Per-page PNG/SVG bytes return** — v1 only returns bytes for single-file
    PDF; multi-page PNG/SVG must go to an `output` path (`{p}` template). A
    bytes-array return for the page set is the parked piece.
  - **Package-cache / offline control** — `--package-cache-path`,
    `--package-path`, network-disabled compiles for reproducible/offline builds.
  - **PDF tagging / metadata** — accessibility tags, document metadata, PDF/A.
  **Reason parked:** v1 covers compile/query/fonts/version — the common
  typesetting needs; these add scope (process ownership, multi-output plumbing)
  better promoted on demand.

## Security & resilience testing

For **authorized testing of your own systems** — load, stress, and
resilience checks (how a service holds up under connection floods,
slow-loris-style held connections, malformed input, traffic bursts). The
low-level primitives already exist: `net.tcp` / `net.udp` / `net.icmp`
clients, the `net.raw` raw-IPv4 engine (v0.34.0), `net.probe.*`,
`net.capture`, the `server.*` listeners, and the engine's async concurrency —
so a script can already generate concurrent load by hand. What's missing is a
**higher-level harness**: controlled request/connection-rate generation
against a target you operate, ramp / soak / burst profiles, latency and
error-rate reporting, and resilience assertions.

**Scope guardrail:** explicitly for defensive, authorized self-testing (your
own infrastructure, staging, CI, or a lab). **Not** attack tooling — no mass
targeting, no amplification/reflection helpers, no detection evasion. Bake
the boundary into the spec (e.g. document the authorized-use intent; consider
a required target-allowlist or explicit confirmation for high-volume modes).

**Shipped:** the **HTTP** slice of this harness — `net.load.http(opts)`,
a worker-pool HTTP load/resilience self-test returning latency percentiles +
error rate, with the dual-use guardrail baked in (public targets refused
without `confirm:true`; loopback/private always allowed; concurrency hard-capped
at 1000; plain HTTP client loop — no raw packets / spoofing / amplification).

**Still parked:** TCP/UDP connection-flood and slow-loris-style held-connection
generators (more weapon-shaped), and ramp / soak / burst **stage** profiles
(compose multiple `net.load.http` calls for now). **Reason still parked:**
dual-use; each wants the same careful spec treatment the HTTP slice got.

## Examples & advanced scripts

The initial **`examples/scripts/advanced/`** set shipped — 12 in-depth,
end-to-end scripts (advanced HTTP/HTTPS/SMTP/TCP servers, a load/resilience
self-test, sqlite ETL, a crypto pipeline, codec interop, packet analysis, a
host-recon report, a complete WebDriver login flow, and a TUI dashboard). See
`examples/README.md` → *Advanced examples*.

What's left here is **growth on demand**, not a standing task: add more
advanced scripts when a real workflow is worth demonstrating (e.g. a Selenium
flow exercising more of the Phase 2 surface, a multi-DB join/migration, a
grid/remote WebDriver example). New ones self-skip on missing prerequisites
and follow the same wiring (`make demo` / CI offline subset / README). No
reason parked — just author + verify when the need arises.

## Tracked code follow-ups

Not features — small known debts noted during implementation, kept here until
addressed.

- **Verify the Windows `runtime.clipboard` paths.** The Windows text
  (`clip` / PowerShell `Get-Clipboard`) and image (`Set-Clipboard` /
  `Get-Clipboard -Format Image` via temp PNG) paths are implemented to the
  documented contracts but have **not been executed** — macOS and Linux (X11 +
  Wayland) are verified end-to-end, Windows is not. Windows isn't a focus right
  now; re-promote when a Windows host/CI runner is available to round-trip text
  + PNG (watch for PowerShell `-STA` and the temp-file marshalling).
