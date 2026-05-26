# Changelog

All notable changes to this project are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
See [CLAUDE.md](./CLAUDE.md) for the project's commit-message conventions.

## [Unreleased]

Nothing yet.

## [0.5.2] — 2026-05-26

Third Moderate cut. **`api.jwt.*`** — sign / view / validate over
HMAC-signed JWTs. New dependency:
`github.com/golang-jwt/jwt/v5 v5.3.1` (pure Go, de facto standard).
Asymmetric algorithms (RSA / ECDSA / EdDSA) are deliberately split
off into a follow-up cut so the JS-side key-shape design can be
done in isolation.

### Added

- `api.jwt.sign(claims, secret, opts?)` produces a
  compact-serialisation signed JWT. `opts.algorithm` defaults to
  `"HS256"`; `"HS384"` and `"HS512"` are also supported. Claims
  pass straight through to jwt-go's `MapClaims`, so RFC 7519
  reserved claims (`exp`, `nbf`, `iat`, `aud`, `iss`, `sub`,
  `jti`) work alongside arbitrary application claims. Missing
  reserved claims aren't synthesised.
- `api.jwt.view(token)` decodes header + payload **without
  verifying the signature** and returns `{ header, payload,
  signature }`. Useful for debugging auth flows or surfacing
  `aud` / `iss` to the user before deciding to trust the token.
  Both `RawURLEncoding` (no padding) and the padded `URLEncoding`
  form are accepted on input; the returned `signature` is the
  raw base64url segment as it appeared in the token.
- `api.jwt.validate(token, secret, opts?)` verifies the signature
  + standard claims and resolves with
  `{ valid: true, claims }` or `{ valid: false, reason }`. The
  resolve-don't-throw contract is the key design point: scripts
  branch on `valid` for bad signature / expired / nbf / aud
  mismatch / iss mismatch. Only structural input errors (wrong
  segment count, invalid base64, invalid JSON, empty secret /
  empty token) throw — those aren't validation failures and a
  script pattern-matching on `valid: false` shouldn't
  accidentally accept a garbage string.
- `opts.audience` / `opts.issuer` propagate into jwt-go's
  `WithAudience` / `WithIssuer` parser options when set; absent,
  jwt-go skips those checks.
- Unsupported algorithm names — including `RS256`, `ES256`,
  `EdDSA`, and the special `"none"` value — throw at `sign` /
  `validate` time with a named-algorithm error. Silent fallback
  to a weaker algorithm than requested would be a security
  footgun.
- `TestJwt*` (19 sub-tests covering: round-trip per algorithm
  HS256/HS384/HS512, view-without-secret, bad-signature
  resolve-false, expired-token resolve-false, audience and issuer
  mismatch resolve-false, four asymmetric-algo rejections, four
  malformed-input throws, empty-secret throws on both sign and
  validate).
- `examples/scripts/jwt.ts` walks through every form; pure stdlib
  so it runs in the CI offline subset.
- `--examples` step 30 covers the binding; MANUAL section 5 gains
  the `api.jwt` block plus a paragraph per op.
- `cmd/sercon/api_docs.go` grows three entries so the emitted
  `api.d.ts` carries hover docs.

### Changed

- `OUT-OF-SCOPE.md`'s JWT entry rewritten: HMAC support is shipped;
  the asymmetric matrix (RSA / ECDSA / EdDSA) is now an explicit
  follow-up item with notes on the JS-side key-shape design.

## [0.5.1] — 2026-05-26

Second Moderate cut. **`api.preg.*`** — PHP-style `/pattern/flags`
regex syntax on top of Go's stdlib `regexp` (RE2). The "preg"
naming is a deliberate homage; the semantics are RE2's. No new
dependencies.

### Added

- `api.preg.match(pattern, subject)` — first hit as
  `{ match, groups, index }`, or `null` when nothing matches.
  Optional groups that didn't match surface as empty strings (not
  `undefined`) so the result shape stays stable across calls.
- `api.preg.matchAll(pattern, subject)` — same shape, drained
  into an array.
- `api.preg.replace(pattern, replacement, subject)` — substitutes
  via Go's `$1` / `${1}` backref syntax. PHP's `\1` form is **not**
  translated — `\\` escapes are already legitimate in the
  replacement and aliasing them would surprise users coming from
  Go's `regexp.ReplaceAllString`.
- Supported flags: `i` / `m` / `s`, merged into Go's `(?ims)`
  inline-flag prefix. Unsupported PHP flags (`u`, `U`, `x`) and
  unknown flags throw with a clear, named-flag error rather than
  silently dropping. The `u` error explicitly notes that RE2 is
  UTF-8 by default so the flag is unnecessary.
- `TestPreg*` (15 sub-tests covering null-on-no-match, first-match
  groups + index, matchAll length, replace backrefs, all three
  supported flags, four unsupported / unknown flag forms, three
  malformed-pattern forms, optional-group empty-string).
- `examples/scripts/preg.ts` walks through every form above plus
  the unsupported-flag error path. Included in `make demo` and the
  CI offline subset (the binding is pure stdlib so it always
  works).
- `--examples` step 29 covers the binding; MANUAL section 5 gains
  the `api.preg` block plus a paragraph on the RE2-vs-PCRE
  semantics difference and the flag rules.
- `cmd/sercon/api_docs.go` grows three entries so the emitted
  `api.d.ts` carries hover docs for `match` / `matchAll` /
  `replace`.

### Changed

- `OUT-OF-SCOPE.md`'s entry for `preg_match` / `preg_replace` is
  rewritten as a forward-looking note about a possible
  `regexp2`-backed PCRE binding — RE2 covers most uses but
  scripts that genuinely need lookahead / lookbehind / pattern
  backrefs would justify a sibling `api.preg2.*` namespace.

## [0.5.0] — 2026-05-26

First Moderate cut: **JSDoc support in the `.d.ts` emitter.** Editor
hover for `api.*` bindings is now populated, and library embedders
have a first-class way to attach docs without writing struct tags or
sibling registration data.

Bumped to 0.5.0 by convention (Moderate bucket → 0.5.x); the
release-please manifest follows along. No new runtime dependencies.

### Added

- `Engine.SetDocs(path, doc)` and `Engine.SetMemberDocs(namespace, docs)`
  attach JSDoc strings to registered bindings. `path` is the dotted
  lookup key — a bare name for top-level bindings (`"log"`,
  `"http"`), or `"namespace.member"` for namespace members
  (`"http.get"`, `"exec.shell"`). The doc map lives on the engine
  rather than the registration, so the same `SetDocs` call works
  regardless of which of the five `Register…` variants was used.
- Multi-line docs (split on `\n`) expand to a standard
  `* `-prefixed JSDoc block, preserving blank lines as bare `*`
  lines. Single-line docs collapse to `/** … */`. Bindings without
  a doc entry emit no JSDoc block at all (no empty `/** */`
  placeholders).
- Calling `SetDocs` with an empty string removes any previously
  set doc for that path; that's also the documented way to undo a
  doc entry.
- `TestWriteTypes_Docs*` (5 sub-tests in `engine_test.go`):
  single-line top-level, multi-line expansion with blank-line
  handling, namespace member docs, absent-doc-no-block, empty-string
  removes.
- `cmd/sercon/api_docs.go` ships a curated doc map for every
  binding under `api.*`. The CLI calls `Engine.SetDocs("api", …)`
  and `Engine.SetMemberDocs("api", apiDocs())` so the emitted
  `examples/scripts/api.d.ts` now grows readable editor hover for
  the example surface.
- MANUAL.md gains a `SetDocs` / `SetMemberDocs` subsection in §3
  (library API) and a JSDoc-on-declarations subsection in §13
  (type generation).

### Changed

- `writeDTS` now accepts an `(regs, docs)` pair; `Engine.WriteTypes`
  passes the engine's doc map through.
- `writeMemberObject` carries a path prefix so nested sub-namespaces
  look up docs under the right dotted key.
- The lockstep checklist in CLAUDE.md grows a seventh artifact:
  `cmd/sercon/api_docs.go` must be updated when adding or changing
  a binding.
- `make version-check` and `make release-prep` now include
  `.release-please-manifest.json` as a fourth version marker so
  it stays in sync with the Go const and the two MANUAL.md
  strings.
- `OUT-OF-SCOPE.md`'s Moderate / `.d.ts` generator subsection is
  removed; this was its only entry.

## [0.4.21] — 2026-05-26

Repo / tooling cut — no new bindings, no script-API surface change.
Wires `release-please` as the primary release driver, retiring the
manual "edit CHANGELOG, `git tag`, push" flow. Closes the Easy
bucket of `OUT-OF-SCOPE.md` (every entry shipped across v0.3.0 –
v0.4.21).

### Added

- `.github/workflows/release-please.yml` runs `googleapis/release-please-action@v4`
  on every master push. The action maintains a "chore: release
  X.Y.Z" PR built from Conventional-Commits subjects since the last
  tag. Merging the PR bumps version markers, updates
  `CHANGELOG-AUTO.md`, and pushes a `vX.Y.Z` tag. `skip-github-release: true`
  is set so the existing tag-triggered release workflow keeps
  ownership of binary publishing.
- `release-please-config.json` — `release-type: simple`,
  `changelog-path: CHANGELOG-AUTO.md` (so the hand-curated
  CHANGELOG.md is never overwritten), and `extra-files` pointing at
  `pkg/scriptengine/version.go` and `MANUAL.md` so all three
  version markers are rewritten as one unit. `changelog-sections`
  surfaces `feat` / `fix` / `perf` / `refactor` / `docs` / `build` /
  `ci` and hides `chore`.
- `.release-please-manifest.json` pinned at `0.4.21` so
  release-please knows where to start counting from.
- `x-release-please-version` end-of-line marker comments on each
  versioned line: the `const Version` in `pkg/scriptengine/version.go`
  and the two MANUAL.md version strings (cover block + footer).
  The generic file updater finds these markers and rewrites just the
  version digits on the line, leaving everything else intact.

### Changed

- `.github/workflows/release.yml` is now documented as fired from
  either path — release-please merge or manual `git tag` — and the
  goreleaser job description acknowledges both entry points.
- `make release-prep VERSION=x.y.z` is reframed in the Makefile
  comment as a manual fallback for ad-hoc local bumps. The sed
  patterns were tightened with capture groups so they preserve the
  trailing marker comments. The `version-check` target's sed regex
  was anchored to end-of-line so the captured value isn't polluted
  by the marker comment.
- `CLAUDE.md`'s Versioning, CI, and release-flow sections now
  describe release-please as the primary driver.
- `OUT-OF-SCOPE.md`'s Easy bucket no longer has a Repo / tooling
  subsection — the only remaining entry shipped here.

## [0.4.20] — 2026-05-26

Final slice of **Easy / External tool integrations**: a wrapper
around the GitHub `gh` CLI. With this cut, every entry in the Easy
bucket has shipped. Still no new dependencies — `os/exec` and
`encoding/json`.

### Added

- `api.gh.authStatus()` → `Promise<{ authenticated, user, raw }>` —
  status probe. Runs `gh api user --jq .login` (machine-friendly)
  rather than `gh auth status` (multi-line human report). Missing
  gh and unauthenticated sessions resolve as data (`authenticated:
  false`), not throws — a status probe scripts can branch on.
  Context cancellation still throws.
- `api.gh.prList(opts?)` → `Promise<Array<{ number, title, state,
  author, headRefName, baseRefName, url, createdAt, updatedAt }>>`.
  Lists pull requests on the repo identified by `opts.cwd` (or the
  engine's working directory). Defaults: open state, limit 30.
  Filters: `state`, `limit`, `author`. `gh`'s `author: { login, …}`
  wrapper is flattened to a bare login string.
- `api.gh.repoView(repo?, opts?)` → `Promise<{ name, owner,
  description, url, defaultBranch, visibility }>`. With no arg,
  uses the cwd's repo; pass `"owner/name"` for any repo `gh` has
  access to. Convenience flattenings: `owner` is a login string
  (not `{login, …}`), `defaultBranch` is the branch name (not
  `defaultBranchRef.name`). Empty repos resolve with
  `defaultBranch: ""` rather than `undefined`.
- `parsePRListJSON` and `parseRepoViewJSON` are pulled out as
  testable helpers so the JSON-flattening logic can be exercised
  without spawning `gh`.
- `TestParsePRListJSON_*` / `TestParseRepoViewJSON_*` (5 sub-tests):
  author-wrapper flattening, null-author preserved, owner +
  defaultBranchRef flattening, null defaultBranchRef yields
  `defaultBranch: ""`, malformed-JSON error.
- `TestGhAuthStatus_NoGhResolvesFalse` pins the no-throw-on-missing
  contract via `t.Setenv("PATH", "/nonexistent")`.
- `TestGhAuthStatus_AgreesWithGhAuthStatus` is a skip-when-missing
  integration probe that cross-checks our boolean against `gh auth
  status`'s real exit code on the host.
- `examples/scripts/gh.ts` gracefully degrades when `gh` is missing
  or unauthenticated so it can be part of `make demo` everywhere.
  Excluded from CI (needs network + a logged-in account).
- `--examples` step 28 covers the binding; MANUAL section 5 gains
  the `api.gh` block plus three bullets of prose.
- `examples/scripts/api.d.ts` regenerated.

### Changed

- `OUT-OF-SCOPE.md`'s Easy bucket no longer has an External tool
  integrations subsection — every entry shipped across v0.4.17 –
  v0.4.20.

## [0.4.19] — 2026-05-26

Third slice of **Easy / External tool integrations**: a wrapper
around the host `git` CLI. `api.gh` is split off into the next cut
(v0.4.20) — given the size of the git surface, packing both
together would have made one over-large release. No new
dependencies — `os/exec` and a single `regexp` for diffStat are all
this needs.

### Added

- `api.git.branch(opts?)` → `Promise<{ current, detached, all }>` —
  current branch (empty on detached HEAD), detached flag derived
  from git's own exit code on `symbolic-ref --short HEAD`, and the
  list of local branches.
- `api.git.isClean(opts?)` → `Promise<boolean>` — convenience
  boolean over `git status --porcelain` emptiness.
- `api.git.revParse(rev, opts?)` → `Promise<string>` — full 40-char
  SHA. Invalid refs throw with git's stderr message in the chain.
- `api.git.status(opts?)` → `Promise<Array<{ path, indexStatus,
  workingStatus }>>` — parsed porcelain v1 output. The status
  fields are the raw single-char codes (`M`, `A`, `D`, `R`, `?`, …)
  so scripts can dispatch without re-parsing.
- `api.git.add(paths, opts?)` — stages one path or a list. `--` is
  inserted between `add` and the paths so leading-dash values work.
- `api.git.commit(message, opts?)` → `Promise<{ sha }>` — empty
  message throws before spawning. `allowEmpty:true` toggles
  `--allow-empty`. Returns the post-commit HEAD SHA.
- `api.git.log(opts?)` → `Promise<Array<{ sha, shortSha, author,
  email, timestamp, subject }>>` — `limit` (default 50) most-recent
  commits in `revRange` (default `HEAD`). Tab-separated format
  string keeps parsing one-line-per-commit.
- `api.git.diffStat(opts?)` → `Promise<{ files, insertions,
  deletions }>` — aggregates `git diff --shortstat`. Default range
  is `HEAD~1..HEAD`. Pure-add or pure-delete diffs return zero on
  the missing side instead of throwing.
- `api.git.runText(args, opts?)` → `Promise<{ stdout, stderr,
  exitCode }>` — escape hatch for invocations the typed bindings
  don't cover. Non-zero exits surface as data (not a throw); spawn
  failures and context cancellation still throw.
- All bindings accept `opts.cwd` so a single engine can work across
  multiple checkouts.
- `TestGit*` (10 sub-tests): fresh repo, dirty-tree status,
  revParse + invalid ref, add + commit round-trip, empty-message
  guard, log ordering and fields, diffStat counters, runText
  non-zero exit, runText input validation, detached-HEAD reporting.
- `examples/scripts/git.ts` builds a throwaway temp repo, exercises
  the read-only ops, demonstrates runText, then cleans up after
  itself. Included in `make demo` and the CI offline subset (git is
  on PATH everywhere we care about).
- `--examples` step 27 covers the binding; MANUAL section 5 gains
  the `api.git` block plus a paragraph per op.
- `examples/scripts/api.d.ts` regenerated.

### Changed

- `optInt` (previously private to barcode) now accepts plain Go
  `int` and `nil` opts maps in addition to `int64` / `float64`.
  Mirrors the `optMillis` change from v0.4.18 and lets test
  harnesses hand in maps without `int64()` casts.
- `OUT-OF-SCOPE.md`'s Easy / External tool integrations list drops
  the `git(repo_path)` entry. `gh()` remains as the v0.4.20 cut.

## [0.4.18] — 2026-05-26

Second slice of **Easy / External tool integrations**: HTTP via
`recon` (preferred) with `curl` as a fallback. Still no new
dependencies — `os/exec` and the system binaries do the work.

### Added

- `api.exec.http(method, url, opts?)` → `Promise<{ status, headers,
  body, durationMs, backend }>` — curl-compatible HTTP client.
- `opts.headers` is a `Record<string, string>` of request headers;
  `opts.body` is piped to the backend as `--data-binary @<tempfile>`
  so CR / LF survive verbatim; `opts.timeout` defaults to 30 000 ms;
  `opts.follow` toggles `-L`; `opts.insecure` toggles `-k`.
- `opts.backend` picks the binary explicitly: `"auto"` (default —
  recon first, curl as a fallback), `"recon"`, or `"curl"`. The
  result tells you which backend ran via `backend`.
- Response headers are dumped to a temp file via `-D <path>` and
  parsed back, rather than relying on `-i`'s body stream. Recon's
  `-i` is verbose-debug style (`< Header: value`), so a unified
  parser across the two backends needed an out-of-band channel.
- Redirect chains (with `follow: true`) skip past intermediate 3xx
  blocks and report the final response.
- HTTP 4xx / 5xx do not throw — they're a normal HTTP outcome and
  callers branch on `status`. Process-start failures, transport
  errors, and context deadline / cancel throw.
- `TestExecHTTP_*` (8 sub-tests): baseline GET, POST with body +
  headers, 4xx-doesn't-throw, transport error, timeout throws,
  forced curl backend, forced recon backend, input validation.
- `TestParseHeaderFile_RedirectChain` /
  `TestParseStatusCode_ReconQuirkyStatusLine` /
  `TestParseStatusCode_NoCode` pin the parser against the quirks
  observed in the real backends (recon prints
  `HTTP/HTTP/2.0 200 OK`, curl prints `HTTP/2 200`).
- `examples/scripts/exec-http.ts` walking through every form above;
  hits httpbin.org so it's in `make demo` but not in CI (same
  policy as `net-probe.ts` and `email-auth.ts`).
- `--examples` step 26 covers the binding; MANUAL section 5 gains
  the `api.exec.http` block plus a paragraph on the throw-vs-resolve
  contract and the backend-selection rules.
- `examples/scripts/api.d.ts` regenerated.

### Changed

- `optMillis` now accepts a plain Go `int` in addition to `int64`
  and `float64`. JS-side integers go through goja as `int64`, but
  Go-level test harnesses that build option maps directly sometimes
  hand in `int`. Tolerating both removes a silent fall-back-to-
  default footgun.
- `OUT-OF-SCOPE.md`'s Easy / External tool integrations list drops
  the recon-with-curl-fallback HTTP entry (now shipped); `git` and
  `gh` wrappers remain as the v0.4.19 cut.

## [0.4.17] — 2026-05-26

First slice of **Easy / External tool integrations**: a generic
subprocess runner, the foundation for the next two cuts (HTTP via
recon-with-curl-fallback in 0.4.18, git/gh wrappers in 0.4.19). No
new dependencies — `os/exec` and `context` from the standard library.

### Added

- `api.exec.shell(cmd, opts?)` → `Promise<{ stdout, stderr,
  exitCode, success, durationMs }>` — generic subprocess runner.
- String `cmd` is passed verbatim to `/bin/sh -c` (or `cmd /C` on
  Windows) so pipes, redirects, and globs work as a user would
  type them. Array `cmd` is treated as `argv` directly with no
  shell involvement.
- `opts.cwd` sets the working directory; `opts.env` is merged on
  top of the parent process environment (not a replacement);
  `opts.timeout` defaults to 30 000 ms; `opts.stdin` is piped to
  the child.
- Non-zero exits resolve with `success: false` and the real
  `exitCode` — they do not throw, since "ran the linter, expected
  to be told it failed" is a routine outcome.
- Process-start failures (binary not on `PATH`, permission denied)
  and context deadline / cancellation throw — those aren't normal
  subprocess outcomes.
- `TestExecShell_*` (8 sub-tests): string cmd via shell, argv mode,
  non-zero exit not thrown, stdin pipe, env override, cwd
  respected, timeout throws, input validation.
- `examples/scripts/exec-shell.ts` walking through every form
  above; included in `make demo` and the offline CI step.
- `--examples` step 25 covers the binding; MANUAL section 5 gains
  the `api.exec.shell` block plus a paragraph on the
  throw-vs-resolve contract.
- `examples/scripts/api.d.ts` regenerated.

### Changed

- `OUT-OF-SCOPE.md`'s Easy / External tool integrations list drops
  the `shell(cmd, opts?)` entry (now shipped); the `recon-with-curl`
  HTTP entry and the `git` / `gh` wrappers remain as the next two
  cuts.

### Fixed

- `api.archive.extract(path, dest, opts)` now respects
  `opts.overwrite`. The previous implementation read opts via the
  2-arg `optsAsMap` helper but the binding takes three positional
  args, so the helper saw `destDir` (a string) as the opts arg and
  silently fell back to `overwrite: false`. Re-extracting into a
  populated destination tripped `O_EXCL` even when callers had
  explicitly opted in. Pinned with a new
  `TestArchiveExtract_OverwriteOptThroughBinding` that drives the
  binding through its real `FunctionCall` shape (the existing
  `TestArchive_OverwriteFlag` exercised the internal `extractZip`
  helper directly and so missed the marshalling layer).

## [0.4.16] — 2026-05-26

Sole cut on **Easy / JSON / querying**. One new pure-Go dep
(`itchyny/gojq`) plus a small normalisation pass to bridge goja's
integer types onto gojq's runtime expectations.

### Added

- `api.jq.query(data, filter)` → `Promise<unknown>` — runs the
  filter and returns the first emitted value (or `null` when the
  filter emits nothing).
- `api.jq.queryAll(data, filter)` → `Promise<unknown[]>` — drains
  the iterator and returns every emitted value.
- Filter syntax is full jq via
  [`github.com/itchyny/gojq`](https://github.com/itchyny/gojq) —
  field access, `.[]` explode, `select`, `map`, `add`, `group_by`,
  optional access via `?`, anything jq itself supports.
- Data round-trips through goja's `.Export()` as `map[string]any` /
  `[]any` trees, which gojq accepts directly.
- `TestJq_RunJqQuery` (5 sub-tests): scalar field, first element,
  exploded names, `select`-filtered admins, computed sum via
  `[.users[].age] | add`. Pins both the first-result and
  full-iterator counts.
- `TestJq_ParseError`: syntax errors surface a clear Go error
  mentioning "parse".
- `TestJq_RuntimeError`: in-iterator type errors (gojq emits them
  as in-band values) get type-asserted out and surfaced as Go
  errors / JS throws.
- `TestJq_OptionalMissing`: `.does.not.exist?` returns one
  result equal to `nil`, which goja converts to JS `null`.
- `examples/scripts/jq.ts` walks scalar / explode / select / sum /
  group_by / optional access patterns, plus a try/catch around a
  parse-error filter.
- `--examples` step 23 added; existing email step shifts to 24.
  `exampleCount` is now 24.

### Changed

- `MANUAL.md` § Built-in `api` declares the `jq` shape; new prose
  block covers the data-shape expectation, the int64 normalisation
  detail, and the error-handling contract.

### Fixed

- Without explicit normalisation gojq panics with
  `invalid type: int64` on any arithmetic against
  goja-exported integers (every JS-side integer becomes an
  `int64` after `Export`). `normaliseForJq` walks the input tree
  and coerces all sized integers to plain `int` and `float32` to
  `float64` before handing it to gojq. The reproducer would be as
  simple as `.users[].age` on a JSON-ish input — fixed inline.

### Dependencies

- New direct (pure Go):
  `github.com/itchyny/gojq v0.12.19`,
  transitive `github.com/itchyny/timefmt-go v0.1.8` (used by
  gojq's `strftime` / `strptime` filters).

## [0.4.15] — 2026-05-26

Sole cut on **Easy / Data comparison** — unified-diff helper plus a
forced bump of the minimum Go version. One new pure-Go dep.

### Added

- `api.diff.compare(a, b, opts?)`:
  - Inputs are strings (UTF-8) or any byte sequence (`ArrayBuffer`,
    `Uint8Array`).
  - Returns `{ identical, binary, added, removed, diff, format }`.
  - `identical` short-circuits with an empty `diff`. `binary`
    (NUL byte in the first 8 KB) likewise — a unified diff of
    binary content isn't useful. Otherwise `diff` is the unified
    diff text and `added` / `removed` are body-only `+` / `-` line
    counts (file headers excluded, mirroring `git diff --shortstat`).
  - `opts`: `context` (default 3), `fromFile` (default "a"),
    `toFile` (default "b").
- Backed by `github.com/pmezard/go-difflib/difflib` for the unified
  diff (preferred over `sergi/go-diff` because that's char-level;
  pmezard produces unified diffs directly).
- `TestDiff_CountAddedRemoved` — pins the `+++ b` / `--- a` header-
  exclusion behaviour that lets `git diff --shortstat`-style counts
  come out right.
- `TestDiff_LooksBinary` — 5 fixtures including a NUL inside the
  8 KB sample window, a NUL past the sample window (should still
  be treated as text), UTF-8 prose, and empty input.
- `examples/scripts/diff.ts` walks through a non-trivial line-edit
  diff (with custom `fromFile` / `toFile`) plus the identical /
  binary short-circuits.
- `--examples` step 22 added; existing email step shifts to 23.
  `exampleCount` is now 23.

### Changed

- **`go.mod` `go` directive bumped from 1.22 to 1.25**, and the
  README badge / CI matrix updated to match. Several deps
  (`golang.org/x/text` v0.37, `golang.org/x/crypto` v0.52,
  `klauspost/compress` v1.18.6, `beevik/ntp` v1.5.0,
  `likexian/whois` v1.15.7) now require Go ≥ 1.24, and x/text
  specifically requires 1.25. The alternative was pinning all of
  those down — sacrificing security and bug fixes — so the floor
  moves instead.
- `MANUAL.md` § Built-in `api` declares the `diff` shape and
  documents the identical / binary short-circuits, the
  `+`/`-`-counting rule, and the `opts.{context,fromFile,toFile}`
  defaults.

### Fixed

- The shared `optsAsMap` helper reads the JS arg at position 1
  (typical `func(target, opts)` shape). `diff.compare(a, b, opts)`
  has its opts at position 2, so the helper would have returned
  nil. `diffCompare` reads the position-2 opts inline — the
  helper stays in place for the rest of the namespaces that use
  the position-1 convention.

### Dependencies

- New direct (pure Go): `github.com/pmezard/go-difflib v1.0.0`.

## [0.4.14] — 2026-05-26

Sole cut on **Easy / Archives & document handling**. Stdlib-only:
`archive/zip` + `archive/tar` + `compress/gzip`. No new go.mod deps.

### Added

- `api.archive.create(destPath, sources)`:
  - Format inferred from `destPath` extension: `.zip`, `.tar`,
    `.tar.gz` (also `.tgz`).
  - Sources are either bare strings (basename becomes the
    in-archive name) or `{path, name?}` objects.
  - Directories are walked recursively; the directory's basename
    becomes the archive subdir and descendants land relative to
    it. All in-archive paths use forward slashes so output is
    cross-platform.
  - Returns `{ path, format, entries, bytes }`.
- `api.archive.extract(archivePath, destDir, opts?)`:
  - Format inferred from `archivePath`'s extension.
  - `opts.overwrite` defaults to `false`; collisions on existing
    files cause an `os.ErrExist`-shaped failure unless explicitly
    overwritten.
  - Returns `{ path, format, dest, entries }`.
- **zip-slip / tar-slip protection** via `safeJoin`. Refuses any
  archive entry whose name is absolute (`/…` or `\…`), contains a
  `..` segment, or whose resolved target falls outside destDir.
  No silent sanitisation — malicious archives surface with a
  clear error rather than getting rewritten into legal-looking
  entries.
- `TestArchive_RoundTrip` (3 sub-tests): create + extract round-
  trip for zip / tar / tar.gz against a small fixture tree.
- `TestArchive_SafeJoinRejectsEscape`: classic `../`, embedded
  `sub/../../`, and absolute-path entries all rejected; legitimate
  nested paths still accepted.
- `TestArchive_ZipSlipRejected`: builds a hand-crafted tar with a
  `../escape.txt` entry; extractTar refuses it.
- `TestArchive_OverwriteFlag`: pre-existing destination file is
  preserved when `overwrite:false`, clobbered with `overwrite:true`.
- `examples/scripts/archive.ts` round-trips `README.md`,
  `CHANGELOG.md`, `LICENSE` through all three formats and extracts
  into sibling dirs. Verified live: zip 19 KB / tar 49 KB /
  tar.gz 18.6 KB; 3 entries extracted from each.
- `--examples` step 21 added; existing email step shifts to 22.
  `exampleCount` is now 22.

### Changed

- `MANUAL.md` § Built-in `api` declares the `archive` shape; new
  prose block covers the source-format conventions
  (string vs `{path, name}` entries), the directory-recursion
  rule, the `overwrite` opt's collision semantics, and the
  zip-slip / tar-slip rejection policy.

## [0.4.13] — 2026-05-26

Third and final cut on **Easy / Encoding / decoding / barcodes** —
the check-digit helpers. Pure stdlib math, no new deps. After this
release Easy / Encoding is empty (the scanner mentioned in v0.4.11's
plan lives in Moderate, not Easy).

### Added

- `api.checkdigit.*`:
  - `algos()` → `string[]` (`luhn`, `isbn10`, `isbn13`, `ean13`,
    `ean8`, `upca`).
  - `validate(algo, input)` → `boolean`. Non-digit characters,
    wrong-length input, and unknown algorithm names all return
    `false` — scripts can probe candidates without wrapping in
    try/catch.
  - `compute(algo, partial)` → `string`. Takes the input *without*
    its check digit and returns just that digit (or `"X"` for
    ISBN-10 position 10).
  - `inspect(algo, input)` → `{ algo, input, valid, given,
    computed }`. Union of validate + compute, useful for
    "tell me everything" diagnostics in interactive tools.
- Synchronous (no Promise) because the algorithms are local math
  with no I/O — sub-microsecond per call.
- Six algorithms implemented inline:
  - **Luhn** (right-to-left doubling, mod-10).
  - **ISBN-10** (weighted sum mod-11; position 10 encoded as `X`).
  - **ISBN-13** / **EAN-13** (weights `1,3,1,3,…`, mod-10). Aliased
    because the math is identical.
  - **EAN-8** / **UPC-A** (weights `3,1,3,1,…`, mod-10).
- `TestCheckdigit_KnownVectors` — every algorithm validates a
  well-known good vector, rejects a single-digit-flipped variant,
  and reconstructs the original check digit from the partial.
  Includes both numeric and `X`-suffix ISBN-10 cases.
- `TestCheckdigit_UnknownAlgorithm` — clean error paths.
- `TestCheckdigit_BadInput` (18 sub-cases) — empty, non-digit,
  and wrong-length inputs all surface false rather than panicking.
- `TestCheckdigit_InspectShape` — pins the inspect-return key set.
- `examples/scripts/checkdigit.ts` — validates + computes + inspects
  across all six algorithms. Verified live: every known vector
  validates and every partial reconstructs to the original check
  digit (including ISBN-10 `X`).
- `--examples` step 20 added; existing email step shifts to 21.
  `exampleCount` is now 21.

### Changed

- `MANUAL.md` § Built-in `api` declares the `checkdigit` shape and
  the per-algorithm input-length / weighting table. Notes the
  validate→`boolean`, compute→`string`, inspect→`object`
  signatures and the sync-vs-Promise distinction.

## [0.4.12] — 2026-05-26

Second of three cuts on **Easy / Encoding / decoding / barcodes** —
the charset side. v0.4.11 covered barcode encoders; v0.4.13 will
add check-digit helpers and a scanner. Two new pure-Go deps.

### Added

- `api.text.detect(data)`: charset detection via
  [`saintfish/chardet`](https://github.com/saintfish/chardet).
  Returns `{ charset, confidence, language?, candidates: [...] }`
  where `confidence` is the 0–100 scale chardet publishes.
- `api.text.encode(text, charset)`: UTF-8 string → bytes in target
  charset. Lossy conversion fails rather than silently dropping
  characters with no representation; callers wanting lossy
  behaviour pre-process the input.
- `api.text.decode(data, charset)`: bytes-in-charset → UTF-8 string.
  Mirror of `encode`; combined they round-trip every charset
  htmlindex knows.
- Charset names follow the WHATWG Encoding Living Standard aliases
  via `golang.org/x/text/encoding/htmlindex.Get` — UTF-8,
  ISO-8859-1, Windows-1252, Shift_JIS, GBK, GB18030, Big5,
  EUC-JP, EUC-KR, etc., plus every documented alias. Unknown
  names throw a clear error.
- `TestText_RoundTrip` (5 sub-tests): encode → decode round-trip
  across UTF-8 / Latin-1 / Windows-1252 / Shift_JIS / GBK.
- `TestText_UnknownCharset`: clean error from `htmlindex.Get` for
  bogus names.
- `TestText_DetectLatin1NotUTF8`: chardet must NOT classify
  Latin-1 bytes containing `0xE9` (è / é) as UTF-8.
- `examples/scripts/charset.ts` — round-trips five representative
  charsets and runs detect against a long Latin-1 sample.
  Verified live: chardet picks Windows-1252 at 78% confidence
  with French as the language hint.
- `--examples` step 19 added; the email step shifts to 20.
  `exampleCount` is now 20.

### Changed

- `MANUAL.md` § Built-in `api` declares the `text` shape; new
  prose block calls out the WHATWG-alias charset names, the
  encoder's strict (non-lossy) behaviour, and the
  candidate-list shape that `detect` returns.

### Dependencies

- New direct (pure Go):
  `github.com/saintfish/chardet v0.0.0-20230101081208-5e3ef4b5456d`,
  `golang.org/x/text v0.37.0` (promoted from indirect — the
  encoding family is needed at top level).

## [0.4.11] — 2026-05-26

First of three cuts on **Easy / Encoding / decoding / barcodes** — the
encoder side, covering ten barcode symbologies under one entry point.
Charset detection / round-tripping lands in v0.4.12; check-digit
helpers and a scanner (decode-from-image) in v0.4.13.

### Added

- `api.barcode.*`:
  - `encode(format, data, opts?)` → `Promise<Uint8Array>` (PNG bytes).
    Ten supported formats: `qr`, `datamatrix`, `aztec`, `pdf417` (2D);
    `code128`, `code39`, `codabar` (linear text); `ean13`, `ean8`,
    `upca` (retail).
  - `formats()` → string list so scripts can iterate.
  - Default size: 256×256 for 2D codes, 400×120 for linear / retail.
    Override via `opts.width` / `opts.height`.
- Format-appropriate encoder defaults: QR uses medium error
  correction + auto mode; Code 39 includes a Mod-43 checksum;
  PDF417 uses security level 5; Aztec runs at 33% ECC with
  auto-selected layers.
- `TestBarcode_EncodeAllFormats` — every supported format produces
  a valid PNG (signature + header + size check) against an
  appropriate payload. Sized assertions confirm the default-sizing
  logic picks 256×256 vs 400×120 correctly.
- `TestBarcode_UnknownFormat` — unknown format names surface a
  clean error mentioning the supported list.
- `examples/scripts/barcode.ts` iterates `formats()`, encodes a
  format-appropriate sample for each, and verifies the PNG
  signature. Also demos a custom-sized QR.
- `--examples` step 18 ("Barcodes (api.barcode.*)"); the existing
  email step shifts to 19. `exampleCount` is 19.

### Changed

- `MANUAL.md` § Built-in `api` declares the `barcode` shape with
  the full format union; new prose block covers the format
  categories (2D / linear / retail) and the encoder-specific
  defaults.
- `MANUAL.md` correction: the compression section claimed the
  return type was `ArrayBuffer`, but goja actually surfaces a Go
  `[]byte` return as `Uint8Array` (you read `.length`, not
  `.byteLength`). Both the type declaration and the prose now say
  `Uint8Array`.
- `examples/scripts/compression.ts` already used `new Uint8Array(...)`
  so the script was correct; only the doc was off.
- `OUT-OF-SCOPE / Easy / Encoding / decoding / barcodes` lost the
  three encoder bullets; charset detect / decode / encode and
  check-digit remain.

### Dependencies

- New direct (pure Go):
  `github.com/boombuler/barcode v1.1.0`.

## [0.4.10] — 2026-05-26

Sole cut on **Easy / Hashing & compression** — the compression piece.
Hashing already shipped in v0.3.1; this rounds out the sub-section.

### Added

- `api.compression.*`: a uniform compress / decompress surface over
  **nine pure-Go algorithms**:
  - **stdlib**: `gzip`, `deflate`, `zlib`, `bzip2` (read only — see
    below).
  - **third-party (pure Go)**: `bzip2` write via `dsnet/compress/bzip2`,
    `zstd` via `klauspost/compress`, `brotli` via `andybalholm/brotli`,
    `lz4` via `pierrec/lz4/v4`, `xz` via `ulikunitz/xz`, `snappy`
    via `golang/snappy`.
  - `api.compression.compress(algo, data)` / `decompress(algo, data)`
    accept `string` (UTF-8) or `ArrayBuffer` / `Uint8Array` input
    and return `ArrayBuffer`. `api.compression.algos()` returns the
    supported list so scripts can iterate without hard-coding.
- `compressBytes` / `decompressBytes` route by algorithm in a flat
  switch — each branch handles writer creation, write, and Close in
  the order each compressor's framing demands.
- `TestCompression_RoundTrip` (9 sub-tests) — every algorithm
  round-trips a ~960 B payload byte-for-byte.
- `TestCompression_UnknownAlgorithm` — unknown algo names surface a
  clean error rather than silently returning empty bytes.
- `examples/scripts/compression.ts` iterates `algos()` and reports
  compressed size + ratio for each. Verified live:
  brotli=5.4%, deflate=6.4%, zlib=7.1%, zstd=7.8%, gzip=8.4%,
  snappy/lz4=10.0%, xz=13.3%, bzip2=17.7% on the demo corpus.
- `--examples` step 17 added; existing email step shifts to 18.
  `exampleCount` is now 18.

### Changed

- `MANUAL.md` § Built-in `api` declares the `compression` shape with
  the full algo-set typed in the union, plus a prose section
  cross-referencing the upstream lib for each algorithm.
- `Makefile`'s `DEMO_SCRIPTS` and `examples/README.md` table absorb
  `compression.ts`. CI workflow's offline subset includes it too —
  compression doesn't touch the network.
- `OUT-OF-SCOPE` / Easy / Hashing & compression section is empty
  (hashing landed in v0.3.1; compression closes the slot now).

### Dependencies

- New direct (all pure Go):
  `github.com/klauspost/compress v1.18.6`,
  `github.com/andybalholm/brotli v1.2.1`,
  `github.com/pierrec/lz4/v4 v4.1.26`,
  `github.com/ulikunitz/xz v0.5.15`,
  `github.com/golang/snappy v1.0.0`,
  `github.com/dsnet/compress v0.0.1`.

## [0.4.9] — 2026-05-26

Second cut on **Easy / Email authentication**, completing the
sub-section. Adds MTA-STS, TLS-RPT, BIMI, and a parallel
`email.all` aggregator on top of v0.4.8's SPF + DMARC. Still
stdlib-only.

### Added

- `api.email.mtaSts(domain)` — looks up `TXT(_mta-sts.<domain>)`,
  parses the `v=STSv1; id=…` marker, then fetches and parses the
  RFC 8461 policy file at
  `https://mta-sts.<domain>/.well-known/mta-sts.txt`. Returns
  `{ present, record, txt: { v, id }, policy?: { version, mode,
  mx[], maxAge }, policyError? }`. Policy-fetch failures (TLS error,
  4xx, timeout) don't fail the binding — the TXT view is still
  returned and the fetch error surfaces as a string under
  `policyError`.
- `api.email.tlsRpt(domain)` — `TXT(_smtp._tls.<domain>)` lookup,
  parses `v=TLSRPTv1; rua=…` into a flat tag map and surfaces
  `rua` separately.
- `api.email.bimi(domain, opts?)` — looks up
  `<selector>._bimi.<domain>`, selector defaulting to `default`
  (override via `opts.selector`). Surfaces the logo URL `l` and
  VMC URL `a`.
- `api.email.all(domain)` — runs every probe in parallel via
  goroutines and returns an aggregate object keyed by probe name.
  Per-probe failures surface under `<probe>.error` so a partial
  result is still useful (e.g. SPF + DMARC found, MTA-STS policy
  fetch timed out).
- `parseMTASTSPolicy` — RFC 8461 line-based `key: value` parser.
  Repeated `mx:` lines aggregate into a slice; `max_age` coerces
  to an int when numeric.
- `TestParseMTASTSPolicy` — three table cases covering the
  canonical form, CRLF + comments + leading whitespace tolerance,
  and the non-numeric `max_age` fallback.
- `TestEmailNamespace_HandlesMissing` extended to cover all five
  individual probes plus the `email.all` aggregator.

### Changed

- Renamed `parseDMARCTags` to `parseTagMap` (the format is shared
  with BIMI / TLS-RPT / the MTA-STS TXT marker). Kept the old name
  as a thin alias so the existing test continues to compile.
- `examples/scripts/email-auth.ts` now demos all five probes and
  the aggregator against `google.com`.
- `--examples` step 17 absorbs the new bindings.
- `MANUAL.md` § Built-in `api` declares the three new shapes + the
  aggregator return type; a new prose block covers each probe's
  semantics and the MTA-STS policy-error fallback behaviour.
- `OUT-OF-SCOPE` / Easy / Email authentication section is now
  empty.

## [0.4.8] — 2026-05-26

First of two cuts on **Easy / Email authentication**. This one covers
SPF + DMARC — the two records with mature semantics and small, easy
parsing. MTA-STS / TLS-RPT / BIMI + the aggregate `email.all` land
in v0.4.9. All stdlib; no new `go.mod` deps.

### Added

- `api.email.spf(domain)`: looks up `TXT(domain)`, returns the first
  record starting with `v=spf1`. Mechanisms are tokenised, and the
  trailing `all` qualifier is summarised under `allPolicy` as one of
  `"pass" | "fail" | "softfail" | "neutral" | ""`. Missing records
  surface as `{ present: false }` rather than throwing; DNS
  operational errors throw as usual.
- `api.email.dmarc(domain)`: looks up `TXT(_dmarc.<domain>)`, parses
  the `v=DMARC1; key=val; …` form into a flat tag map (keys
  case-folded; values keep internal whitespace + commas so `rua`
  lists survive). Common tags are surfaced separately on the result:
  `policy`, `subdomain`, `percent`, `rua`, `ruf`. Same
  presence-false convention as SPF.
- `examples/scripts/email-auth.ts` against `google.com`. Network-
  dependent, so it joins `net-probe.ts` outside the CI offline
  subset.
- `--examples` step 17 ("Email authentication"); `exampleCount` is
  now 17.
- Tests:
  - `TestParseDMARCTags` (5 table cases) pins the tag-format
    behaviour offline: case-folded keys, whitespace tolerance,
    `rua` comma preservation, empty / malformed parts dropped.
  - `TestEmailNamespace_HandlesMissing` end-to-end: a bogus
    `.invalid` domain resolves to `{ present: false }` on both
    bindings, exercising the NXDOMAIN → presence-false path.

### Changed

- `MANUAL.md` § Built-in `api` declares the `email` shape; a new
  prose block calls out the presence-check convention, the SPF
  `allPolicy` summary, and the DMARC parser's case-folding and
  whitespace rules. A trailing sentence promises the remaining three
  protocols + aggregator for v0.4.9.
- `OUT-OF-SCOPE` Easy / Email authentication trimmed: SPF + DMARC
  drop out, leaving the (renamed) "MTA-STS / TLS-RPT / BIMI +
  aggregator" entry for the next cut.

## [0.4.7] — 2026-05-26

Second cut on **Easy / Protocol probes & connectivity** — the two
third-party-lib items that didn't fit v0.4.6's stdlib-only slice.
After this release, Easy / Protocol probes is empty.

### Added

- `api.net.ntp(host, opts?)`: NTPv4 clock query via
  [`github.com/beevik/ntp`](https://github.com/beevik/ntp). Returns
  `{ serverTime, offsetMs, rttMs, stratum, referenceTime,
  rootDelayMs, rootDispersionMs }`. Optional `{ timeout?, port? }`
  with defaults of 5 s and UDP 123.
- `api.net.whois(domain, opts?)`: two-hop WHOIS lookup (IANA →
  registrar's whois server) via
  [`github.com/likexian/whois`](https://github.com/likexian/whois)
  with parsing through
  [`github.com/likexian/whois-parser`](https://github.com/likexian/whois-parser).
  Returns `{ raw, domain?, registrar? }`; `raw` is always populated,
  parsed views are best-effort. `opts.timeout` defaults to 10 s.
- `TestNetNTP_UnreachableHostSurfacesError` — confirms the binding
  surfaces a host-side error (rather than crashing) when pointed at
  a closed UDP port. Offline; runs under `-short`.
- `TestNetWHOIS_InvalidDomainDoesNotPanic` — smoke test for the
  whois wrapper. Skipped under `testing.Short()` because there's no
  reasonable way to stand up a fake whois server locally; the test
  is mostly useful for confirming the dependency wires through.
- `examples/scripts/net-probe.ts` extended with `ntp` against
  `pool.ntp.org` and `whois` against `example.com`.
- `--examples` step 16 absorbs the two new probes; the note line
  now also documents the NTP default port.

### Changed

- `MANUAL.md` § Built-in `api` declares the two new probe return
  shapes, with prose calling out beevik/ntp's per-probe options and
  the likexian/whois library's context-less timeout plumbing.

### Dependencies

- New direct: `github.com/beevik/ntp v1.5.0`,
  `github.com/likexian/whois v1.15.7`,
  `github.com/likexian/whois-parser v1.24.21`. All pure Go.
- Transitive: `github.com/likexian/gokit v0.25.16`.

## [0.4.6] — 2026-05-26

First of two cuts on **Easy / Protocol probes & connectivity**. This
one covers the stdlib-only items — `tcp`, `dns`, `tls` — so no new
`go.mod` entries. `ntp` and `whois` (which need third-party libs)
land in v0.4.7.

### Added

- `api.net.tcp(target, opts?)`: TCP connect probe. Resolves the host,
  dials, and reports `{ host, port, ip, latencyMs }`. `target` is
  `"host:port"` or `"host"` (with `opts.port` overriding the default
  `"80"`). Optional `{ timeout: ms }`, default 5s. Backed by
  `net.Dialer.DialContext`.
- `api.net.dns(host, opts?)`: DNS lookups via `net.Resolver`. Returns
  `{ a, aaaa, mx, txt, cname, ns }` with empty record sets omitted so
  scripts can probe membership. `opts.types` scopes the query (case-
  insensitive subset of `"a" | "aaaa" | "mx" | "txt" | "cname" | "ns"`).
- `api.net.tls(target, opts?)`: leaf-certificate inspection. Returns
  `{ cn, issuer, notBefore, notAfter, daysRemaining, dnsNames,
  serialNumber, fingerprintSha256 }`. Dials with `InsecureSkipVerify`
  so even expired / hostname-mismatched certs come back inspectable.
  Default port is `443`. Hosts wanting validity verification should
  re-run `crypto/x509.Verify` themselves.
- `cmd/sercon/api_net_test.go`: four local-fixture tests covering
  TCP against a localhost listener, DNS against `localhost`, DNS
  types-filter behaviour, and TLS against a self-signed ECDSA server.
  No external network access — all four pass offline.
- `examples/scripts/net-probe.ts` demonstrates the three probes
  against `example.com`. Excluded from the CI offline subset; runs
  via local `make demo`.
- `--examples` walkthrough gains step 16 ("Protocol probes
  (api.net.*)"); `exampleCount` is now 16.

### Changed

- `MANUAL.md` § Built-in `api` declares the new `net` shape, plus a
  prose block calling out the timeout default, `target` parsing
  rules, the `InsecureSkipVerify` policy on `tls`, and the
  empty-set-omission behaviour on `dns`.
- `examples/README.md` table and `Makefile`'s `DEMO_SCRIPTS` gain
  `net-probe.ts`. CI workflow's "smoke examples" step explicitly
  enumerates the offline subset and notes the network-dependent ones
  in a comment.

## [0.4.5] — 2026-05-26

Fifth slice of the **Easy** bucket — the **Repo / tooling**
sub-section. CI workflow, cross-platform release pipeline, and a
local make target that handles the boring parts of cutting a
release. No code changes; everything lands as new YAML and
Makefile targets.

### Added

- **`.github/workflows/ci.yml`** — runs on every push to `master` and
  every PR. Matrix: Go 1.22 + latest stable, on `ubuntu-latest` and
  `macos-latest`. Each job: `go build` (slim flags), `go vet`,
  `go test ./...`, plus the offline subset of `examples/scripts/*`
  (excludes `async.ts`, which hits example.com). A separate `lint`
  job runs `golangci-lint@v2.12.2` (pinned to match `make lint`'s
  fallback).
- **`.github/workflows/release.yml`** — triggers on `v*.*.*` tag
  push. Runs `goreleaser release --clean` with the repo's
  `.goreleaser.yml`. Uses `GITHUB_TOKEN` only — no PAT needed.
- **`.goreleaser.yml`** — cross-compiles darwin-{amd64,arm64} /
  linux-{amd64,arm64} / windows-amd64. Mirrors `make release`'s
  `-trimpath` + `-s -w` flags. Each archive bundles LICENSE, README,
  CHANGELOG, MANUAL.md, and MANUAL.pdf. Changelog block on the
  release page is sourced from Conventional Commits with the
  noisy types (`docs:`, `chore:`, `test:`, `style:`) filtered out.
- **`make release-prep VERSION=x.y.z`** — bumps the three version
  markers (`pkg/scriptengine/version.go`, MANUAL cover, MANUAL
  footer), runs `version-check`, and prints the next-step checklist.
  The CHANGELOG move stays manual because that's the part that
  needs editorial judgement.
- **`make version-check`** — standalone sanity check that the three
  version markers agree. Run by `release-prep`; useful by itself
  after editing one of them by hand.

### Changed

- `CLAUDE.md`'s common-commands block lists the two new make targets.
- `CLAUDE.md` gains a "CI and release flow" section describing what
  each workflow does and which tags will or won't trigger
  `goreleaser` (tags pre-`v0.4.5` won't, because the YAML wasn't in
  those commits — by design).

### Notes

- Tags `v0.2.4`–`v0.4.4` are still local-only and will keep their
  source-only releases when eventually pushed. From `v0.4.5` onward,
  pushing the tag triggers `goreleaser` and the release gains
  prebuilt darwin / linux / windows archives plus `checksums.txt`.

## [0.4.4] — 2026-05-26

Fourth slice of the **Easy** bucket — the **CLI** sub-section. Stdin
script support, a distinct exit code per failure type, and a `-v` flag
that actually reveals something useful. No new external dependencies.

### Added

- **Stdin script support.** A `-` positional argument reads an entry
  script from `os.Stdin`. PASS / FAIL lines and trace output use
  `<stdin>` as the label. Arguments are still processed in order, so
  `sercon prelude.ts -` runs the prelude and then stdin.
- **`Options.Verbose io.Writer`** on `pkg/scriptengine`. When non-nil,
  receives engine traces prefixed `[sercon] `: the rewritten
  entry-script JS (the form goja actually runs, post-ESM-to-CJS
  rewrite + async IIFE) and each module-resolution event. The CLI's
  `-v` flag plugs in `os.Stderr`.
- **`scriptengine.ErrTranspile`** sentinel. Both entry-script and
  required-module transpile failures now wrap it via `fmt.Errorf("%w:
  ...", ErrTranspile, ...)`, so hosts can use `errors.Is` to
  distinguish "the script never ran" from a runtime throw.
- **Distinct CLI exit codes**, the documented matrix is:
  - `0` — all scripts passed.
  - `1` — CLI usage error.
  - `2` — at least one transpile error (`errors.Is(err, ErrTranspile)`).
  - `3` — at least one timeout / context cancel
    (`errors.Is(err, ErrScriptTimeout | context.Canceled | …)`).
  - `4` — at least one JS throw (anything else).
  - Highest applicable code wins across a multi-script invocation.
- `TestRun_TranspileErrorSentinel` — pins the `errors.Is` contract.
- `TestRun_VerboseWriterEmitsTraces` — sanity-checks that the engine
  emits both transpile-entry and require-resolved lines.

### Changed

- `cmd/sercon/main.go` is now structured around an exit-code return
  rather than the old `error`-and-`os.Exit(1)` shape; `main()` is a
  one-liner `os.Exit(run(os.Args[1:]))`.
- `--help` gains an `ARGUMENTS` block (positional / stdin semantics)
  and an expanded `EXIT STATUS` block listing all five codes plus the
  "highest wins" rule. Synopsis grows a `sercon [flags] -` line; an
  extra example shows the shell-pipeline form.
- `MANUAL.md` § 4 (CLI) absorbs the new flag semantics, the stdin
  usage block, the exit-code table, and a paragraph explaining what
  `-v` reveals.

## [0.4.3] — 2026-05-26

Third slice of the **Easy** bucket — the **`.d.ts` generator**
sub-section. Two real behavioural fixes plus a new tracked artifact
(`examples/scripts/api.d.ts`).

### Added

- **`AsyncBinding` carrier type** in `pkg/scriptengine`. The struct
  pairs the raw goja-callback `func(goja.FunctionCall) goja.Value`
  with a `TSReturnType` string computed from the generic parameter
  of `PromisifyAsync[T]`. The d.ts emitter reads this to emit
  `Promise<T>` for async bindings; the engine unwraps the struct to
  its `.Func` at registration time so goja's host-callback
  special-case still fires.
- **`make types`** target — regenerates `examples/scripts/api.d.ts`
  from the current CLI binding surface. The output is now tracked in
  git so editor autocomplete and reviewers see the public api shape
  without running the binary.
- **`examples/scripts/api.d.ts`** — checked-in artifact, regenerable
  via `make types`. Lockstep rule in `CLAUDE.md` extends to seven
  artifacts and now points at this file.
- `TestWriteTypes_StructMethodReceiver` and
  `TestWriteTypes_AsyncBindingPromise` in `engine_test.go` covering
  both fixes end-to-end.

### Changed

- **`PromisifyAsync[T]` return type** is now `AsyncBinding` rather
  than the bare `func(goja.FunctionCall) goja.Value`. Existing CLI
  bindings (`api.http.*`, `api.time.sleep`) work unchanged — the
  engine recursively unwraps any `AsyncBinding` it finds in
  `map[string]any` namespace bodies before handing values to
  `vm.Set`.
- **`structShape` reflects methods from the original (possibly
  pointer) type** instead of unwrapping to `Elem` first. Go exposes
  pointer-receiver methods on `*T`'s method set, not on `T`'s, so
  the old code dropped them silently for `Register("counter", &c)`
  style registrations. Field iteration still uses the underlying
  struct.
- **`funcSig` now takes an `isMethod` flag**, used by callers that
  pass `reflect.Method.Type` (whose `In(0)` is the receiver). The
  flag is `true` only at the two callers reflecting methods —
  `structShape` and `writeConstructorDecl`. Everywhere else it
  stays `false`. Receivers no longer leak into the printed
  parameter list.
- **`writeValueDecl` resolves `RegisterFactory` factories at d.ts
  time** by invoking them with `(nil, nil)`, matching what
  `writeNamespaceDecl` already did. A panic falls back to a TODO
  comment + `unknown`. Lets factory-built `AsyncBinding`
  registrations surface as `Promise<T>` instead of the previous
  TODO.

### Removed

- The unused `Promised[T any] func(...)` marker type. Its job is
  done by `AsyncBinding` via a struct field, which dodges goja's
  exact-type check for `func(goja.FunctionCall) goja.Value`
  callbacks (named func types fall through that check and end up
  on the reflect path, which can't pack JS args into a
  `FunctionCall`).

## [0.4.2] — 2026-05-26

Second slice of the **Easy** bucket — the **Require / module loading**
sub-section. Two real behavioural additions to the resolver plus a
JSON-import regression test. No new external dependencies (the rewrite
uses `encoding/json` from stdlib).

### Added

- **`.js` → `.ts` swap** in `Engine.resolveRequirePath`. When a request
  ends in `.js` / `.cjs` / `.mjs` and the literal file is absent, the
  resolver now tries the same path with `.ts` / `.tsx`. Handles
  `package.json` `main` fields that point at compiled output where
  only the TypeScript source is on disk.
- **`package.json` `source` preference** via a new
  `Engine.maybeRewritePackageJSON`. When the source loader is asked
  for a `package.json` and that file has a `source` field pointing at
  an existing `.ts`/`.tsx` file, the JSON is rewritten on the fly so
  `main` points at the TS source. Matches the convention used by
  parcel/microbundle/etc.
- Three new tests:
  - `TestRun_PackageJsonMainTSFallback` — `main: "lib/index.js"`
    plus an on-disk `lib/index.ts` resolves correctly.
  - `TestRun_PackageJsonSourcePreferred` — `source` wins over `main`
    even when `main`'s target exists, proving the rewrite isn't
    just a fallback.
  - `TestRun_JSONImport` — `import data from "./data.json"` and
    `require("./data.json")` both yield the parsed JSON object.
- Two new runnable examples + supporting fixtures:
  - `examples/scripts/json-import.ts` (+ `helpers/data.json`).
  - `examples/scripts/pkg-resolution.ts` (+ `helpers/pkg/` tree
    containing `package.json`, `src/lib.ts`, and a decoy
    `dist/index.js`).

### Fixed

- `Engine.resolveRequirePath` now returns
  `require.ModuleFileDoesNotExistError` instead of a private
  "module not found" wrapper. Without this, goja_nodejs's `loadAsFile`
  short-circuited the fallback chain on the very first miss — meaning
  `loadAsDirectory` (and therefore `package.json` resolution) never
  ran. This was a latent bug; the new package-json tests surfaced it.

### Changed

- `Makefile`'s `DEMO_SCRIPTS` list gains the two new examples.
- `examples/README.md` script table extended accordingly.
- `MANUAL.md` § 10 (Module resolution) documents the new
  `.js` → `.ts` swap and `package.json` `source` preference, plus an
  explicit JSON-import snippet.

## [0.4.1] — 2026-05-26

Example coverage for everything that landed in 0.3.x – 0.4.0. Pure
docs/examples patch; no behaviour change.

### Added

- `examples/scripts/hash.ts` — every `api.hash.*` algorithm + a known
  SHA-256 vector check.
- `examples/scripts/strings.ts` — `api.str.*` tour (trim with mask,
  pad, reverse, stripHtml, base64, url, html-entity, sprintf,
  normalizeNewlines).
- `examples/scripts/path-and-time.ts` — `api.path.*` and
  `api.time.format` with both an IANA zone and a local-zone variant.
- `examples/scripts/default-export.ts` (+ `helpers/answer.ts`) — proves
  the entry rewriter's `__esModule ? .default : module` unwrap.
- `examples/scripts/tsx-demo.ts` (+ `helpers/el.tsx`) — TSX module
  loading with an `@jsx h` pragma so JSX rewrites to a local factory.
- `make demo` Makefile target — runs every success-path example as a
  single command. Excludes `hang.ts` (intentional timeout demo).
  `DEMO_SCRIPTS` variable enumerates the list so future additions only
  need a one-line update.

### Changed

- `examples/README.md` gains a table of bundled scripts and what each
  demonstrates.
- `MANUAL.md` § Quickstart points at `make demo` and lists the demo
  inventory.
- `CLAUDE.md` "Keeping docs in lockstep" gains a sixth artifact:
  `examples/scripts/`. Any change to a user-visible binding, flag, or
  script-facing behaviour now must add or update the relevant example
  there and pass `make demo`. The rule explicitly carves out
  library-only changes (`WithScriptRoot`, `Engine.Reset()`) which
  don't need example growth.

## [0.4.0] — 2026-05-26

Opening cut of the **Easy** backlog bucket — the **Transpile / entry
rewriter** sub-section. No code changes to the engine; this is
test-and-doc coverage for paths that already worked but were never
proven end-to-end. The MINOR bump reflects the bucket transition, not
new behaviour.

### Added

- `TestRun_ESMDefaultExport` — proves `import answer from "./mod"`
  resolves to the value of `export default …` via the entry-script
  rewriter's `__esModule ? .default : module` unwrap.
- `TestRun_ESMDefaultAndNamed` — proves a mixed
  `import def, { named } from "./mod"` statement assigns both names
  correctly (uses `reImportDefAndN`).
- `TestRun_TSXModuleEndToEnd` — proves a `.tsx` helper module is
  resolved by extension fallback, transpiled by esbuild's `LoaderTSX`,
  and executes through goja. Uses an `@jsx h` pragma so JSX rewrites
  to a local factory rather than `React.createElement`.

### Changed

- `MANUAL.md` § 8 (TypeScript support) replaces the "wired but not yet
  exercised" caveat about `.tsx` / `.jsx` with a concrete usage block
  showing the `@jsx h` pragma pattern and the `export default`
  interop.

Final slice of the **Trivial** backlog bucket — the **Repo / tooling**
sub-section. Adds the linter config and shakes out the findings the
codebase had collected while we focused on features. After this cut
the Trivial bucket is empty; the next minor (v0.4.0) starts on the
**Easy** bucket.

### Added

- `.golangci.yml` — golangci-lint v2 config. Uses the `standard`
  default linter set (errcheck, govet, ineffassign, staticcheck,
  unused) with two small adjustments: `errcheck.check-type-assertions`
  is on, and the `govet.inline` analyzer is disabled because its
  suggestions to inline calls into x/crypto wrappers aren't actionable
  without bumping our Go minimum.
- `make lint` target. Uses the system `golangci-lint` if installed,
  otherwise falls back to a one-shot `go run` of the pinned version
  (`GOLANGCI_VERSION=v2.12.2`) so contributors don't need to install
  anything globally.

### Fixed

A clean lint pass surfaced 19 issues across the codebase. Highlights:

- `cmd/sercon/api_hash.go`: BLAKE3 now uses `blake3.Sum256` (one-shot)
  rather than instantiating a `Hasher` and discarding `Write`'s
  return value.
- `pkg/scriptengine/bindings.go`: explicit `_ =` discard on
  `resolve` / `reject` returns from `vm.NewPromise()`. The errors are
  only ever non-nil if the promise has already settled, which is
  impossible in our usage — making the discard explicit documents
  that.
- `pkg/scriptengine/dts.go`: `reflect.Ptr` → `reflect.Pointer` (the
  canonical name; `Ptr` is the deprecated alias).
- `cmd/sercon/help.go`: removed an unused `(*styler).section` helper
  and a dead-store assignment in the TS highlighter.
- `pkg/scriptengine/timeout.go`: deleted the unused `withInterrupt`
  helper. The file now holds only the `ErrScriptTimeout` sentinel,
  matching the actual architecture (the live cancellation watcher is
  inline in `engine.go`). `CLAUDE.md`'s description of the file is
  updated to match.

### Changed

- `CLAUDE.md` lists `make lint` in the common-commands block and the
  description of `pkg/scriptengine/timeout.go` no longer mentions the
  deleted helper.

Third slice of the **Trivial** backlog bucket — the
**String utilities & formatting** sub-section. All stdlib, no new
dependencies.

### Added

- `api.str.*` (eighteen members): `trim` / `ltrim` / `rtrim` (PHP-style
  custom-mask trimming), `reverse` (rune-aware), `stripHtml`, `nl2br`
  (with optional XHTML mode), `br2nl`, `base64Encode` / `base64Decode`,
  `urlEncode` / `urlDecode` (form-style), `htmlEntityDecode`, `pad` /
  `lpad` / `rpad` (recon-shaped, defaults to a single-space pad on the
  right), `sprintf` / `printf` (Go fmt verbs; printf writes to stdout),
  `normalizeNewlines` ("lf" / "crlf" / "cr").
- `api.path.*`: `dirname`, `basename(p, suffix?)` — POSIX semantics
  (forward slashes).
- `api.time.format(unixMs, fmt, tz?)`: small strftime renderer over a
  curated token set (`%Y %y %m %d %H %M %S %F %T %j %A %a %B %b %z %Z
  %%`). Day/month names are English; unknown `%X` tokens are emitted
  verbatim. Takes Unix milliseconds for symmetry with `api.time.nowMs`.
- Three new test files driving each surface through real Engine + Run:
  `api_str_test.go`, plus a Go-level table test for `strftime`.
- `--examples` walkthrough adds steps 14 ("String utilities") and 15
  ("Paths and time formatting"). `exampleCount` is now 15.

### Changed

- `MANUAL.md` declares the new `str` / `path` shapes and extends the
  `time` shape with `format`. A new prose block calls out the
  recon-style vs JS-native semantic differences (trim mask, urlEncode
  form-style, sprintf verbs, pad sides, strftime token table).

## [0.3.1] — 2026-05-26

Second slice of the **Trivial** backlog bucket — the
**Hashing & compression** sub-section. (Compression is rated Easy
rather than Trivial in OUT-OF-SCOPE.md, so this cut covers hashing
only.)

### Added

- `api.hash.*` script binding: nine algorithms returning lowercase hex
  digests of the UTF-8 input.
  - `md5`, `sha1`, `sha256`, `sha384`, `sha512` (stdlib `crypto/*`)
  - `sha3_256`, `sha3_512` (`golang.org/x/crypto/sha3`)
  - `blake3` — 32-byte output (`lukechampine.com/blake3`,
    pure-Go BLAKE3)
  - `crc32` — IEEE polynomial, zero-padded to 8 hex chars (stdlib
    `hash/crc32`)
- `cmd/sercon/api_hash_test.go`: per-algorithm vectors for the empty
  string and the canonical "abc" input.
- `--examples` walkthrough gains a "Hashing (api.hash.*)" section
  (`exampleCount` is now 13).

### Changed

- `MANUAL.md` declares the new `api.hash` shape in its built-in `api`
  declaration block and explains the UTF-8 / lowercase-hex / crc32-IEEE
  semantics.

### Dependencies

- New direct: `lukechampine.com/blake3 v1.4.1`,
  `golang.org/x/crypto v0.52.0` (pulls in `klauspost/cpuid/v2` for
  blake3's CPU feature detection — still pure Go).

## [0.3.0] — 2026-05-26

First minor focusing on the **Trivial** backlog bucket, starting with
the **Engine** sub-section. This release is two pure-API-surface
additions; no library dependencies were added.

### Added

- `RunOption` + `WithScriptRoot(dir)`: pass per-Run overrides to
  `Engine.Run` / `Engine.RunFile` without rebuilding the engine. The
  override applies only to the current call and rewrites the entry
  script's effective base directory for `require` / `import`
  resolution.
- `Engine.Reset()`: clear every registered binding. Lets a long-lived
  Engine be reused across unrelated script batches that each want a
  clean global namespace. Not safe to call concurrently with Run.

### Changed

- `Engine.Run` / `Engine.RunFile` are now variadic in their option
  list; existing two-/three-arg callers compile unchanged.

## [0.2.4] — 2026-05-26

### Fixed

- `make manual` no longer passes `--toc`. recon's auto-injected TOC
  was being placed above the cover-page `<div>`, pushing the cover
  to page 2. `MANUAL.md` ships its own curated `## Table of contents`
  section, which stays in flow and renders in the correct order.

### Changed

- `OUT-OF-SCOPE.md` gains a top-level **Deferred** bucket alongside
  Trivial / Easy / Moderate / Hard. Items land there with a stated
  reason rather than an effort estimate, ready to re-promote when the
  reason resolves. First occupant: `pdf_export_page` (no trustworthy
  pure-Go PDF renderer).

## [0.2.3] — 2026-05-25

### Added

- `MANUAL.md` now opens with an HTML `<div class="cover">` cover page
  (title, subtitle, version, date, repo & licence) styled by recon's
  bundled CSS, matching the
  [`codedeviate/recon`](https://github.com/codedeviate/recon) manual.

### Changed

- `make manual` now passes `--unsafe-html` (so the cover-page `<div>`
  renders) and `--page-break-on-h1` (so every top-level section starts
  on a fresh PDF page). Updated `--doc-*` metadata to "sercon User
  Manual" with keywords.

## [0.2.2] — 2026-05-25

### Added

- `LICENSE` — MIT, copyright 2026 Thomas Bjork.
- Badge row at the top of `README.md`: GitHub repo, license, Go version,
  latest release, pkg.go.dev. Style mirrors the
  [`codedeviate/webrunner`](https://github.com/codedeviate/webrunner)
  badges, with the `crates.io` slot replaced by `pkg.go.dev`. A
  Homebrew tap badge is staged in an HTML comment and will be enabled
  once `sercon` lands in `codedeviate/homebrew-cli`.

### Changed

- `go.mod`: lowered the `go` directive from `1.26.3` to `1.22` to
  match the README's "Go 1.22+" claim and the project's original
  compatibility target. `go mod tidy` also split direct/indirect
  requires into separate blocks.

## [0.2.1] — 2026-05-25

### Added

- `Makefile` with `build`, `release`, `manual`, `test`, `vet`, `clean`
  targets. `make release` builds with `-trimpath -ldflags='-s -w'` for
  a roughly 30% smaller binary (~23 MB → ~16 MB on darwin/arm64).
- `MANUAL.pdf` — PDF rendering of `MANUAL.md` produced by
  `make manual` (uses `recon --md-to-pdf`). Regenerated whenever
  `MANUAL.md` changes.

### Changed

- `CLAUDE.md` lockstep section now lists `MANUAL.pdf` and points at
  `make manual`; the common-commands section uses `make` targets.

## [0.2.0] — 2026-05-25

### Added

- `--help` / `-h`: in-depth, colourised help screen (sections, flag table,
  examples, exit codes, see-also). Colour is auto-disabled when stdout is
  not a TTY and on `NO_COLOR=1`; force on with `FORCE_COLOR=1`.
- `--examples`: colourised walkthrough of every script feature
  (logging, assertions, http, time, env, imports, require, promises,
  error handling, timeouts, goja built-ins, eventloop additions).
- `--version`: prints the engine version (`scriptengine.Version`) plus
  goja / goja_nodejs / esbuild versions read from `runtime/debug`
  build info.
- `MANUAL.md`: long-form reference covering the library API, CLI, the
  built-in `api` surface, goja built-ins, eventloop additions, top-level
  await mechanics, module resolution, error semantics, and limitations.
- `pkg/scriptengine.Version` constant, bumped in lockstep with the git
  tag.

### Changed

- `CLAUDE.md` now documents the lockstep update rule for `MANUAL.md`,
  `--help`, `--examples`, and `CHANGELOG.md`.

## [0.1.0] — 2026-05-25

Initial cut of the embeddable TypeScript script engine. The project went
through an intra-day rename from `tsrun` to `sercon` before being
tagged; the final names below are the ones that ship in 0.1.0.

### Added

- `pkg/scriptengine` library:
  - `Engine`, `Options` (with `DisableConsole`, `Timeout`, `ScriptRoot`), `New`.
  - `Register`, `RegisterNamespace`, `RegisterConstructor` for static
    bindings; `RegisterFactory` / `RegisterNamespaceFactory` for bindings
    that need the per-Run `*goja.Runtime` and `*eventloop.EventLoop`.
  - `Run` / `RunFile` with a fresh runtime per call and a shared
    `require.Registry` compile cache.
  - `PromisifyAsync[T]` helper that schedules resolution back onto the
    event loop and parks a `SetTimeout` sentinel so `loop.Run` waits for
    the async tail.
  - `Promised[T]` marker for d.ts return-type annotation.
  - `WriteTypes` — `.d.ts` emitter with cycle protection
    (visited-set + depth cap) and special cases for `goja.FunctionCall`
    and `goja.Value`.
  - Two-mode esbuild transpile: CJS for required modules, ESM-then-rewrite
    for the entry script to support top-level `await`.
  - TS-aware custom `require` source loader with Node-style extension
    fallback (`.ts`, `.tsx`, `.js`, `.cjs`, `.mjs`, `.json` and
    `index.*` for directories).
  - Interrupt-based timeout (`ErrScriptTimeout`) and `context.Context`
    cancellation, both routed through one watcher goroutine that calls
    `vm.Interrupt` plus `loop.Terminate`.
- `cmd/sercon` CLI with `-timeout`, `-root`, `-emit-dts`, `-v` flags and
  the example `api` namespace (`api.log`, `api.assert.*`, `api.http.*`,
  `api.time.*`, `api.env.get`).
- Example scripts: `smoke.ts`, `async.ts`, `helpers/assert.ts`,
  `helpers/fixtures.ts`, `hang.ts` (timeout demo).
- Test suite (11 cases) covering the 10 spec-required scenarios plus a
  bare-specifier require-resolution sanity check. Golden `.d.ts` fixture
  refreshable via `go test -update`.
- READMEs at the repo root and under `examples/`.
- `CLAUDE.md` describing non-obvious architecture and the project's
  semver + Conventional Commits conventions.

[Unreleased]: about:blank
[0.1.0]: about:blank
