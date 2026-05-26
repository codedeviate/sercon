package main

// apiDocs returns the doc strings consumed by Engine.SetMemberDocs to
// decorate the emitted api.d.ts with JSDoc blocks. Keys are dotted
// member paths under the `api` namespace ("log", "http.get",
// "exec.shell"). Docs are intentionally terse — one or two sentences
// each — so editor hover stays readable. The full prose lives in
// MANUAL.md; this map is the at-a-glance summary.
//
// When adding or changing a binding, update the corresponding entry
// here as part of the same change. Missing entries are silently
// tolerated by the emitter (no JSDoc block is rendered).
func apiDocs() map[string]string {
	return map[string]string{
		// Core
		"log":          "Stringify each argument and print one space-separated line to stdout. The script-side equivalent of console.log without buffering.",
		"assert.equal": "Throw when actual != expected (strict equality on primitives, deep equality on objects). Optional msg appears in the error.",
		"assert.ok":    "Throw when cond is falsy. Optional msg appears in the error.",

		// HTTP (built-in)
		"http.get":  "Perform an HTTP GET with a 5-second default timeout. Returns { status, body }.",
		"http.post": "Perform an HTTP POST with a 5-second default timeout. Returns { status, body }.",

		// Time
		"time.nowMs":  "Wall-clock milliseconds since the Unix epoch.",
		"time.sleep":  "Resolve after `ms` milliseconds. Cancellable via the engine timeout.",
		"time.format": "Format a unix-ms timestamp through strftime tokens. Optional IANA tz (e.g. 'Europe/Stockholm'); default is the host's local zone.",

		// Env
		"env.get": "Read an environment variable. Returns undefined when unset (not empty string).",

		// Hash (api.hash.* — see hash.ts example for the full set)
		"hash.md5":      "MD5 hex digest of a UTF-8 input. Avoid for security purposes — exposed for compatibility with legacy fingerprints.",
		"hash.sha1":     "SHA-1 hex digest of a UTF-8 input. Avoid for security purposes.",
		"hash.sha256":   "SHA-256 hex digest of a UTF-8 input.",
		"hash.sha384":   "SHA-384 hex digest of a UTF-8 input.",
		"hash.sha512":   "SHA-512 hex digest of a UTF-8 input.",
		"hash.sha3_256": "SHA-3 256-bit hex digest. The underscore in the name matches recon's binding.",
		"hash.sha3_512": "SHA-3 512-bit hex digest.",
		"hash.blake3":   "BLAKE3 hex digest (32-byte output, lukechampine.com/blake3).",
		"hash.crc32":    "CRC-32 (IEEE polynomial), zero-padded to 8 hex chars.",

		// String utilities
		"str.trim":              "Strip whitespace (or any char in the optional mask string) from both ends.",
		"str.ltrim":             "Like trim, left side only.",
		"str.rtrim":             "Like trim, right side only.",
		"str.reverse":           "Rune-aware reversal — `reverse('café')` is `'éfac'`.",
		"str.stripHtml":         "Remove HTML tags and decode common entities.",
		"str.nl2br":             "Replace newlines with <br> (or <br/> when xhtml=true).",
		"str.br2nl":             "Inverse of nl2br: <br>, <br/>, <br /> → '\\n'.",
		"str.base64Encode":      "Standard base64 (with padding).",
		"str.base64Decode":      "Standard base64; URL-safe input is accepted via auto-detect.",
		"str.urlEncode":         "Form-encoding ('+' for space). For path segments use encodeURIComponent (provided by goja).",
		"str.urlDecode":         "Inverse of urlEncode.",
		"str.htmlEntityDecode":  "Decode named and numeric HTML entities to their UTF-8 equivalents.",
		"str.pad":               "Pad to `len` with `padChar` (default ' '). `side` is 'right' (default), 'left', or 'both'.",
		"str.lpad":              "Shortcut for pad(side: 'left').",
		"str.rpad":              "Shortcut for pad(side: 'right').",
		"str.sprintf":           "Go's fmt verbs (%s, %d, %x, %.2f, %v, %t, %q, …) — not PHP's.",
		"str.printf":            "sprintf + write to stdout.",
		"str.normalizeNewlines": "Canonicalise any mix of \\r\\n, \\r, \\n to the requested style ('lf' | 'crlf' | 'cr').",

		// Path
		"path.dirname":  "Directory portion of a path. POSIX-style; trailing slashes are stripped.",
		"path.basename": "Final segment of a path; optional suffix is stripped if it matches.",

		// Net probes (network-dependent)
		"net.tcp":   "Dial a TCP target and report latency + resolved IP. Default timeout 5s.",
		"net.dns":   "Look up A / AAAA / MX / TXT / CNAME / NS records. Default: all five.",
		"net.tls":   "Open a TLS connection (InsecureSkipVerify; for probing only) and return the cert chain summary.",
		"net.ntp":   "Query an NTPv4 server (UDP 123) and report offset, RTT, stratum, root delay / dispersion.",
		"net.whois": "Two-hop WHOIS via the IANA referral, returning the parsed record plus the raw response text.",

		// Email auth probes
		"email.spf":    "Query TXT(<domain>) for SPF, return record + parsed mechanisms + all-policy.",
		"email.dmarc":  "Query TXT(_dmarc.<domain>) and parse policy / pct / rua / ruf tags.",
		"email.mtaSts": "Probe MTA-STS: TXT(_mta-sts.<domain>) plus the fetched policy file.",
		"email.tlsRpt": "Probe TLS-RPT: TXT(_smtp._tls.<domain>) and parse rua.",
		"email.bimi":   "Probe BIMI: TXT(<selector>._bimi.<domain>); selector defaults to 'default'.",
		"email.all":    "Run all five email probes in parallel — five-way handshake aggregate.",

		// Compression
		"compression.algos":      "Available compression algorithm names (gzip / deflate / zlib / bzip2 / zstd / brotli / lz4 / xz / snappy).",
		"compression.compress":   "Compress data with the named algorithm. Returns Uint8Array.",
		"compression.decompress": "Decompress data previously produced by compress (same algorithm name required).",

		// Barcode
		"barcode.formats": "Available barcode formats (qr / datamatrix / aztec / pdf417 / code128 / code39 / codabar / ean13 / ean8 / upca).",
		"barcode.encode":  "Render data into a PNG of the chosen format. opts.width / opts.height default to 200x200.",

		// Text / charset
		"text.detect": "Detect the most-likely charset of a byte sequence (saintfish/chardet). Returns top guess + candidates.",
		"text.decode": "Decode bytes in a named charset to a UTF-8 string.",
		"text.encode": "Encode a UTF-8 string to bytes in the named charset.",

		// Check digits
		"checkdigit.algos":    "Supported algorithms (luhn / isbn10 / isbn13 / ean13 / ean8 / upca).",
		"checkdigit.validate": "Return whether the input passes the named algorithm's check digit.",
		"checkdigit.compute":  "Compute the missing trailing check digit for a partial input.",
		"checkdigit.inspect":  "Diagnostic combining validate + compute: { valid, given, computed, … }.",

		// Archive
		"archive.create":  "Create a zip / tar / tar.gz at destPath from a list of paths. Format inferred from extension.",
		"archive.extract": "Extract a zip / tar / tar.gz to destDir. opts.overwrite controls O_EXCL behaviour.",

		// Diff
		"diff.compare": "Unified-diff two text inputs. opts: context (default 3), fromFile / toFile (default 'a' / 'b'). Binary inputs return { binary: true } with an empty diff.",

		// JQ
		"jq.query":    "Run a jq filter over data and return the first emitted value (or null).",
		"jq.queryAll": "Run a jq filter and drain the iterator into an array.",

		// Exec
		"exec.shell": "Run a subprocess. String cmd → /bin/sh -c (or `cmd /C` on Windows); array cmd → argv. Non-zero exits resolve; spawn failures and timeouts throw.",
		"exec.http":  "HTTP via recon (preferred) or curl (fallback). 4xx / 5xx resolve as status; transport errors and timeouts throw. opts.backend = 'auto' | 'recon' | 'curl'.",

		// Git
		"git.branch":   "Current branch (empty when HEAD is detached) plus the list of local branches.",
		"git.isClean":  "True iff `git status --porcelain` is empty.",
		"git.revParse": "Full 40-char SHA for the given rev. Invalid refs throw.",
		"git.status":   "Parsed `git status --porcelain` entries: { path, indexStatus, workingStatus }.",
		"git.add":      "Stage one path (string) or several (string[]).",
		"git.commit":   "Create a commit; returns the post-commit HEAD SHA. opts.allowEmpty toggles --allow-empty.",
		"git.log":      "Recent commits as { sha, shortSha, author, email, timestamp, subject }. opts.limit / opts.revRange.",
		"git.diffStat": "Aggregate { files, insertions, deletions } from `git diff --shortstat`. Default revRange HEAD~1..HEAD.",
		"git.runText":  "Escape hatch: run any `git <args>`, get { stdout, stderr, exitCode } — exitCode is data, not a throw.",

		// GitHub CLI
		"gh.authStatus": "Probe gh's auth state. Missing gh / unauthenticated resolve with { authenticated: false, … } — only context cancellation throws.",
		"gh.prList":     "List pull requests on the cwd's repo (or opts.cwd). Defaults: open state, limit 30. Filters: state / limit / author.",
		"gh.repoView":   "Repo metadata. With no arg uses cwd's repo; pass 'owner/name' for any repo gh can see. owner + defaultBranch are pre-flattened.",
	}
}
