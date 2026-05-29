package main

// Doc maps consumed by registerSurface via Engine.SetMemberDocs to
// decorate the emitted d.ts with JSDoc blocks. Each function
// returns docs for one top-level global; keys are member paths
// relative to that global (e.g. "log", "assert.equal", "hash.sha256").
//
// When adding or changing a binding, update the matching entry here
// as part of the same change. Missing entries are silently tolerated
// by the emitter (no JSDoc block is rendered).

func runtimeDocs() map[string]string {
	return map[string]string{
		"log":          "Stringify each argument and print one space-separated line to stdout. The script-side equivalent of console.log without buffering.",
		"assert.equal": "Throw when actual != expected (strict equality on primitives, deep equality on objects). Optional msg appears in the error.",
		"assert.ok":    "Throw when cond is falsy. Optional msg appears in the error.",
		"time.nowMs":   "Wall-clock milliseconds since the Unix epoch.",
		"time.sleep":   "Resolve after `ms` milliseconds. Cancellable via the engine timeout.",
		"time.format":  "Format a unix-ms timestamp through strftime tokens. Optional IANA tz (e.g. 'Europe/Stockholm'); default is the host's local zone.",
		"env.get":      "Read an environment variable. Returns undefined when unset (not empty string).",
		"argv":         "Per-script argument vector: [programName, scriptPath, ...userArgs]. argv[0] is the program name (sercon), argv[1] is the running script path, and any args after `--` on the command line start at argv[2].",
	}
}

func cryptoDocs() map[string]string {
	return map[string]string{
		"hash.md5":      "MD5 hex digest of a UTF-8 input. Avoid for security purposes — exposed for compatibility with legacy fingerprints.",
		"hash.sha1":     "SHA-1 hex digest of a UTF-8 input. Avoid for security purposes.",
		"hash.sha256":   "SHA-256 hex digest of a UTF-8 input.",
		"hash.sha384":   "SHA-384 hex digest of a UTF-8 input.",
		"hash.sha512":   "SHA-512 hex digest of a UTF-8 input.",
		"hash.sha3_256": "SHA-3 256-bit hex digest. The underscore in the name matches recon's binding.",
		"hash.sha3_512": "SHA-3 512-bit hex digest.",
		"hash.blake3":   "BLAKE3 hex digest (32-byte output, lukechampine.com/blake3).",
		"hash.crc32":    "CRC-32 (IEEE polynomial), zero-padded to 8 hex chars.",
		"jwt.sign":      "Sign a claims object. secret is raw bytes for HS*; PEM-encoded private key for RS*/PS*/ES*/EdDSA; or a JWK JSON object (kty picks the key type) for any algorithm. opts.algorithm defaults to HS256.",
		"jwt.view":      "Decode header + payload WITHOUT verifying the signature. Useful for inspection / debugging auth flows. Malformed input throws.",
		"jwt.validate":  "Verify signature + standard claims (exp/nbf/iat) + optional aud/iss. secret accepts raw bytes / PEM public key / JWK. Set opts.algorithm for the algo-confusion guard. Resolves { valid:true, claims } or { valid:false, reason }.",
		"encrypt.keygen":        "Generate a fresh age X25519 keypair. Returns { publicKey: 'age1...', privateKey: 'AGE-SECRET-KEY-1...' }.",
		"encrypt.keygenPgp":     "Generate a PGP keypair (RSA 2048). opts.name / opts.email populate the user ID. Returns armored { publicKey, privateKey } blocks. encrypt/decrypt auto-route to PGP when they see these.",
		"encrypt.encrypt":       "Seal data to recipients. age public keys (age1...) → age backend (opts.armored for ASCII); PGP public-key blocks → PGP backend (always armored). Auto-dispatched on key format. Multi-recipient: any listed identity decrypts.",
		"encrypt.decrypt":       "Open a payload with one of the supplied identities. Routes to age or PGP based on the identity / ciphertext format. age: binary or armored auto-detected. Wrong identity throws.",
		"encrypt.rekey":         "Re-encrypt for a new recipient set without exposing plaintext to JS. Output format defaults to match the input; opts.armored forces. Internal decrypt+encrypt loop.",
		"encrypt.detectBackend": "Classify a recipient / identity string. Returns { backend: 'age'|'pgp'|'unknown', kind?: 'public'|'private' }. Pure prefix matching; no parsing or I/O.",
	}
}

func textDocs() map[string]string {
	return map[string]string{
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
		"charset.detect":        "Detect the most-likely charset of a byte sequence (saintfish/chardet). Returns top guess + candidates.",
		"charset.decode":        "Decode bytes in a named charset to a UTF-8 string.",
		"charset.encode":        "Encode a UTF-8 string to bytes in the named charset.",
		"preg.match":            "First hit of /pattern/flags against subject, or null. Returns { match, groups, index }; optional groups that didn't match surface as empty strings.",
		"preg.matchAll":         "Every hit of /pattern/flags against subject, as an array of { match, groups, index } objects.",
		"preg.replace":          "Substitute every match of /pattern/flags in subject. Replacement uses Go's $1 / ${1} backref syntax — PHP's \\1 form is NOT translated.",
		"preg2.match":           "First hit of /pattern/flags via regexp2 (PCRE). Supports lookahead/lookbehind/backreferences. Same { match, groups, index } shape as preg. No linear-time guarantee.",
		"preg2.matchAll":        "Every hit of /pattern/flags via regexp2 (PCRE), as an array of { match, groups, index }.",
		"preg2.replace":         "Substitute every match of /pattern/flags via regexp2. Replacement uses .NET $1 / ${1} syntax. Backtracking engine — keep a timeout around untrusted input.",
		"jq.query":              "Run a jq filter over data and return the first emitted value (or null).",
		"jq.queryAll":           "Run a jq filter and drain the iterator into an array.",
		"diff.compare":          "Unified-diff two text inputs. opts: context (default 3), fromFile / toFile (default 'a' / 'b'). Binary inputs return { binary: true } with an empty diff.",
	}
}

func codecDocs() map[string]string {
	return map[string]string{
		"compression.algos":      "Available compression algorithm names (gzip / deflate / zlib / bzip2 / zstd / brotli / lz4 / xz / snappy).",
		"compression.compress":   "Compress data with the named algorithm. Returns Uint8Array.",
		"compression.decompress": "Decompress data previously produced by compress (same algorithm name required).",
		"barcode.formats":          "Available encode formats (qr / datamatrix / aztec / pdf417 / code128 / code39 / codabar / ean13 / ean8 / upca).",
		"barcode.decodableFormats": "Available decode formats (qr / datamatrix / aztec / code128 / code39 / code93 / codabar / ean13 / ean8 / upca / upce / itf). PDF417 is encode-only.",
		"barcode.encode":           "Render data into a PNG of the chosen format. opts.width / opts.height default to 256x256 (2D) or 400x120 (1D). opts.quietZone (true or px count) pads a white margin — required for EAN/UPC to decode.",
		"barcode.decode":           "Decode a PNG/JPEG/WebP image to { format, text } via gozxing. Optional format hint skips the auto-detect walk. EAN/UPC need a quiet zone in the input.",
		"checkdigit.algos":         "Supported algorithms (luhn / isbn10 / isbn13 / ean13 / ean8 / upca).",
		"checkdigit.validate":      "Return whether the input passes the named algorithm's check digit.",
		"checkdigit.compute":       "Compute the missing trailing check digit for a partial input.",
		"checkdigit.inspect":       "Diagnostic combining validate + compute: { valid, given, computed, … }.",
	}
}

func fsDocs() map[string]string {
	return map[string]string{
		"path.dirname":    "Directory portion of a path. POSIX-style; trailing slashes are stripped.",
		"path.basename":   "Final segment of a path; optional suffix is stripped if it matches.",
		"archive.create":  "Create a zip / tar / tar.gz at destPath from a list of paths. Format inferred from extension.",
		"archive.extract": "Extract a zip / tar / tar.gz to destDir. opts.overwrite controls O_EXCL behaviour.",
	}
}

func netDocs() map[string]string {
	return map[string]string{
		"http.get":      "Perform an HTTP GET with a 5-second default timeout. Returns { status, body }.",
		"http.post":     "Perform an HTTP POST with a 5-second default timeout. Returns { status, body }.",
		"http.request":  "Full HTTP client: method, url, opts {headers, body, timeout, retry, follow, username, password}. Returns {status, ok, headers, body, url}. 4xx/5xx dont throw; retry covers transport errors + 5xx.",
		"probe.tcp":     "Dial a TCP target and report latency + resolved IP. Default timeout 5s.",
		"probe.dns":     "Look up A / AAAA / MX / TXT / CNAME / NS records. Default: all five.",
		"probe.tls":     "Open a TLS connection (InsecureSkipVerify; for probing only) and return the cert chain summary.",
		"probe.ntp":     "Query an NTPv4 server (UDP 123) and report offset, RTT, stratum, root delay / dispersion.",
		"probe.whois":   "Two-hop WHOIS via the IANA referral, returning the parsed record plus the raw response text.",
		"probe.ping":    "Reachability probe. mode tcp (default; dials host:port) or icmp (needs raw-socket privileges). Returns { sent, received, lossPercent, minMs, avgMs, maxMs }. Unreachable = received 0, no throw.",
		"probe.smtp":    "SMTP capability probe (no mail sent). EHLO + parse extensions. Returns { banner, ehloDomain, extensions, starttls, authMechanisms, sizeLimit }. Connection failures throw.",
		"probe.wss":     "WebSocket handshake probe. Opens ws://wss:// connection, optional ping/pong RTT. Returns { connected, subprotocol, status, handshakeMs, pingMs }. Failed handshake throws.",
		"netstatus.check": "Run DNS / TCP / TLS / HTTP against one host concurrently. Returns { reachable, dns, tcp, tls, http } — each sub-probe ok+error; reachable = dns.ok AND tcp.ok. Sub-failures are data, not throws.",
		"email.spf":     "Query TXT(<domain>) for SPF, return record + parsed mechanisms + all-policy.",
		"email.dmarc":   "Query TXT(_dmarc.<domain>) and parse policy / pct / rua / ruf tags.",
		"email.mtaSts":  "Probe MTA-STS: TXT(_mta-sts.<domain>) plus the fetched policy file.",
		"email.tlsRpt":  "Probe TLS-RPT: TXT(_smtp._tls.<domain>) and parse rua.",
		"email.bimi":    "Probe BIMI: TXT(<selector>._bimi.<domain>); selector defaults to 'default'.",
		"email.all":     "Run all five email probes in parallel — five-way handshake aggregate.",
		"browser.open":  "Open a stateful HTTP session: { setUserAgent, setHeader, get, post, cookies }. Cookie jar + default headers persist across requests (like a browser).",
	}
}

func dbDocs() map[string]string {
	return map[string]string{
		"sqlite.open":    "Open a SQLite database (':memory:' or a file path; created if absent). Resolves to a handle { exec, query, queryValue, close }. Connection is Ping-ed before resolving.",
		"redis.open":     "Connect to Redis (redis://...). Returns { do, ping, close }. do(cmd, ...args) runs any RESP command; missing key -> null. Pings on open to surface bad addresses.",
		"memcached.open": "Connect to memcached (host:port). Returns { get, set, delete }. get -> string or null (miss); delete -> bool (existed). set(key, value, expirySeconds?).",
		"ldap.open":      "Dial LDAP (ldap://host:port), anonymous bind (or opts.bindDN/password). Returns { rootDSE, search, close }. search(baseDN, filter, attrs?) -> entries; rootDSE -> server metadata.",
		"dict.define":    "RFC 2229 DICT word lookup. define(host, word, opts?) -> { word, found, definitions: [{ db, dbName, text }] }. found:false on no match (not an error).",
		"dict.match":     "RFC 2229 word match. match(host, word, opts?) -> { word, matches: [{ db, word }] }. opts.strategy (default prefix), opts.database, opts.port (default 2628).",
	}
}

func servicesDocs() map[string]string {
	return map[string]string{
		"exec.shell": "Run a subprocess. String cmd → /bin/sh -c (or `cmd /C` on Windows); array cmd → argv. Non-zero exits resolve; spawn failures and timeouts throw.",
		"exec.http":  "HTTP via recon (preferred) or curl (fallback). 4xx / 5xx resolve as status; transport errors and timeouts throw. opts.backend = 'auto' | 'recon' | 'curl'.",
		"git.branch":   "Current branch (empty when HEAD is detached) plus the list of local branches.",
		"git.isClean":  "True iff `git status --porcelain` is empty.",
		"git.revParse": "Full 40-char SHA for the given rev. Invalid refs throw.",
		"git.status":   "Parsed `git status --porcelain` entries: { path, indexStatus, workingStatus }.",
		"git.add":      "Stage one path (string) or several (string[]).",
		"git.commit":   "Create a commit; returns the post-commit HEAD SHA. opts.allowEmpty toggles --allow-empty.",
		"git.log":      "Recent commits as { sha, shortSha, author, email, timestamp, subject }. opts.limit / opts.revRange.",
		"git.diffStat": "Aggregate { files, insertions, deletions } from `git diff --shortstat`. Default revRange HEAD~1..HEAD.",
		"git.runText":  "Escape hatch: run any `git <args>`, get { stdout, stderr, exitCode } — exitCode is data, not a throw.",
		"gh.authStatus": "Probe gh's auth state. Missing gh / unauthenticated resolve with { authenticated: false, … } — only context cancellation throws.",
		"gh.prList":     "List pull requests on the cwd's repo (or opts.cwd). Defaults: open state, limit 30. Filters: state / limit / author.",
		"gh.repoView":   "Repo metadata. With no arg uses cwd's repo; pass 'owner/name' for any repo gh can see. owner + defaultBranch are pre-flattened.",
		"ai.providers":  "Which of claude / codex / copilot / gemini are on PATH, in preference order.",
		"ai.send":       "Run a one-shot prompt through a provider. opts { prompt (required), provider?, system?, context?, timeout? }. Returns { provider, output, exitCode }. Non-zero exit is data; no provider throws.",
	}
}

func tuiDocs() map[string]string {
	return map[string]string{
		"layout": "Declare the pane layout for this Run. Tree nodes: { name, title?, weight? } (leaf), { rows: [...], weight? } (vertical split), { cols: [...], weight? } (horizontal split). Throws on duplicate names, empty rows/cols, unknown keys, or under --watch.",
		"pane":   "Return a Pane handle for a declared pane. Throws if the name wasn't in the layout. Handle methods: write(text), writeln(text), clear(), title(text). services.exec.shell({pane}) streams subprocess I/O into a pane.",
	}
}

func serverDocs() map[string]string {
	return map[string]string{
		"http.listen":  "Bind an HTTP listener: server.http.listen({port, host?, routes, use?}) → handle with .address, .close(), .stopped Promise. routes is a map of stdlib http.ServeMux patterns ('GET /users/{id}') to handlers (req, res) => res.json({...}) or {use: [...], handler: fn} for per-route middleware.",
		"https.listen": "Like server.http.listen plus required cert/key (file paths OR inline PEM strings). No autocert; no self-signed magic.",
	}
}
