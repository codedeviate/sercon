package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"golang.org/x/term"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// styler emits ANSI escape codes when the destination is a TTY and the user
// has not opted out via NO_COLOR (see https://no-color.org/).
type styler struct {
	w       io.Writer
	enabled bool
}

func newStyler(w io.Writer) *styler {
	return &styler{w: w, enabled: shouldColor(w)}
}

func shouldColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

func (s *styler) wrap(code, text string) string {
	if !s.enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s *styler) bold(t string) string    { return s.wrap("1", t) }
func (s *styler) dim(t string) string     { return s.wrap("2", t) }
func (s *styler) cyan(t string) string    { return s.wrap("36", t) }
func (s *styler) yellow(t string) string  { return s.wrap("33", t) }
func (s *styler) green(t string) string   { return s.wrap("32", t) }
func (s *styler) magenta(t string) string { return s.wrap("35", t) }

// highlightTS does a minimal TypeScript-ish colourisation: keywords in
// magenta, strings in yellow, line comments dimmed. It's deliberately small
// and conservative — anything it can't recognise stays in the default colour.
func (s *styler) highlightTS(src string) string {
	if !s.enabled {
		return src
	}
	keywords := map[string]bool{
		"const": true, "let": true, "var": true, "function": true,
		"return": true, "import": true, "from": true, "export": true,
		"await": true, "async": true, "if": true, "else": true,
		"for": true, "while": true, "try": true, "catch": true,
		"throw": true, "new": true, "true": true, "false": true,
		"null": true, "undefined": true, "void": true, "typeof": true,
	}

	var out strings.Builder
	for _, line := range strings.Split(src, "\n") {
		i := 0
		for i < len(line) {
			c := line[i]
			// line comment
			if c == '/' && i+1 < len(line) && line[i+1] == '/' {
				out.WriteString(s.dim(line[i:]))
				break
			}
			// string literal: ", ', `
			if c == '"' || c == '\'' || c == '`' {
				end := findStringEnd(line, i)
				out.WriteString(s.yellow(line[i : end+1]))
				i = end + 1
				continue
			}
			// identifier / keyword
			if isIdentStart(c) {
				j := i + 1
				for j < len(line) && isIdentCont(line[j]) {
					j++
				}
				word := line[i:j]
				if keywords[word] {
					out.WriteString(s.magenta(word))
				} else {
					out.WriteString(word)
				}
				i = j
				continue
			}
			out.WriteByte(c)
			i++
		}
		out.WriteByte('\n')
	}
	return strings.TrimRight(out.String(), "\n")
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_' || c == '$'
}

func isIdentCont(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func findStringEnd(s string, start int) int {
	quote := s[start]
	for i := start + 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == quote {
			return i
		}
	}
	return len(s) - 1
}

// visibleLen counts runes (good enough for ASCII section titles and the few
// box-drawing chars we use).
func visibleLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// showVersion prints the engine version plus the upstream goja / esbuild
// versions read from the build info if available.
func showVersion(w io.Writer) {
	s := newStyler(w)
	fmt.Fprintf(w, "%s %s\n", s.bold(s.cyan("sercon")), s.green("v"+scriptengine.Version))
	fmt.Fprintf(w, "%s %s\n", s.dim("scriptengine library:"), "v"+scriptengine.Version)
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			switch dep.Path {
			case "github.com/dop251/goja",
				"github.com/dop251/goja_nodejs",
				"github.com/evanw/esbuild":
				fmt.Fprintf(w, "%s %s\n", s.dim(dep.Path+":"), dep.Version)
			}
		}
	}
}

// showHelp prints a colourised, structured help screen. The flag package's
// default usage is too terse for the binary; this replaces it.
func showHelp(w io.Writer) {
	s := newStyler(w)
	fmt.Fprintln(w, s.bold("NAME"))
	fmt.Fprintf(w, "    %s — embeddable TypeScript script engine, CLI front-end\n\n", s.cyan("sercon"))

	fmt.Fprintln(w, s.bold("SYNOPSIS"))
	fmt.Fprintf(w, "    %s [flags] <script.ts> [script.ts ...]\n", s.cyan("sercon"))
	fmt.Fprintf(w, "    %s [flags] -                # read entry script from stdin\n", s.cyan("sercon"))
	fmt.Fprintf(w, "    %s run [flags] <script.ts> [args...]   # one script; args → runtime.argv\n", s.cyan("sercon"))
	fmt.Fprintf(w, "    %s init [dir]                # drop sercon.d.ts + jsconfig for editor autocomplete\n", s.cyan("sercon"))
	fmt.Fprintf(w, "    %s --examples | --help | --version\n\n", s.cyan("sercon"))

	fmt.Fprintln(w, s.bold("DESCRIPTION"))
	fmt.Fprintln(w, "    Scripts get fifteen reserved top-level globals — runtime, crypto,")
	fmt.Fprintln(w, "    text, codec, fs, net, db, cloud, server, services, tui, image,")
	fmt.Fprintln(w, "    web, audio, mcp — each holding a related group of bindings:")
	fmt.Fprintln(w, "    runtime (logging, assertions, time, env, argv), crypto (hash/jwt/")
	fmt.Fprintln(w, "    encrypt), text (string/regex/charset/jq/diff), codec (compression/")
	fmt.Fprintln(w, "    barcode/checkdigit), fs (path/archive), net (http/probe/email send")
	fmt.Fprintln(w, "    + ...), db (sqlite/redis/...), cloud (aws/azure/gcp), server (http/")
	fmt.Fprintln(w, "    https listeners + WebSocket + smtp), services (exec/git/gh/ai), tui")
	fmt.Fprintln(w, "    (multi-pane UI), image (decode/encode/transform), web (fetch/html/")
	fmt.Fprintln(w, "    feed/sitemap), audio (decode/encode/transform), mcp (client/server).")
	fmt.Fprintln(w, "    Each script gets a fresh runtime; helpers are loaded via")
	fmt.Fprintln(w, "    require()/import. See MANUAL.md")
	fmt.Fprintln(w, "    §5 for the full reference and §6 for the server surface, or")
	fmt.Fprintln(w, "    --examples for a guided tour. The generated sercon.d.ts (see")
	fmt.Fprintln(w, "    -emit-dts) is the machine-readable spec. Run `sercon init` in a")
	fmt.Fprintln(w, "    script directory to drop sercon.d.ts + a jsconfig.json so any")
	fmt.Fprintln(w, "    TypeScript-aware editor gives autocomplete + hover with no setup.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    A `console` global (log/info/debug → stdout, warn/error → stderr)")
	fmt.Fprintln(w, "    is also provided so scripts pasted from a browser or Node run")
	fmt.Fprintln(w, "    unchanged; runtime.log is the native equivalent.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    Long-running scripts (HTTP servers, WebSocket handlers) keep the")
	fmt.Fprintln(w, "    engine alive while listeners are bound. Use `sercon serve")
	fmt.Fprintln(w, "    script.ts` to add production niceties: structured access log to")
	fmt.Fprintln(w, "    stderr, --shutdown-timeout (default 30s), --port-override, and a")
	fmt.Fprintln(w, "    `READY listening on tcp/…` line on stdout per listener.")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "    A script may begin with a `#!` shebang line (it's stripped before")
	fmt.Fprintln(w, "    transpile), so a .ts can be made directly executable. For one that")
	fmt.Fprintln(w, "    takes arguments, use the `run` subcommand via")
	fmt.Fprintln(w, "    `#!/usr/bin/env -S sercon run`: `run` executes a single script and")
	fmt.Fprintln(w, "    hands every token after it to the script as runtime.argv[2:].")
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, s.bold("FLAGS"))
	flagLine := func(name, args, desc string) {
		fmt.Fprintf(w, "    %s %s\n        %s\n",
			s.green(name), s.dim(args), desc)
	}
	flagLine("-timeout", "duration", "Per-script wall-clock limit (default 10s; 0 disables).")
	flagLine("-root", "dir", "Script root for require/import resolution (default: dir of the first script).")
	flagLine("-emit-dts", "path", "Write a .d.ts for the example bindings to `path` and exit.")
	flagLine("-emit-reference", "path", "Write the markdown binding reference to `path` and exit.")
	flagLine("-v, --verbose", "", "Print a PASS line per script on success (default is quiet — only failures print); also trace the rewritten entry-script JS and each module resolution to stderr, and print duration on failure.")
	flagLine("--silent", "", "Suppress the runner's PASS/FAIL status lines entirely. Script console output and the export-default result still print; failure is signalled only by the exit code.")
	flagLine("--help, -h", "", "Show this help and exit.")
	flagLine("--examples", "", "Show colourised script examples covering every feature; then exit.")
	flagLine("--no-pager", "", "Don't page --help / --examples through $PAGER (default: page when stdout is a terminal; falls back to `less`).")
	flagLine("--version", "", "Print the engine version (plus goja / esbuild versions) and exit.")
	flagLine("--doctor", "", "Check external tool requirements (git, gh, AI providers, agent-browser, chromedriver/geckodriver, typst, recon/curl, clipboard/image): installed?, version, purpose — and the chromedriver↔Chrome major-version match; then exit. Exit 0 normally (missing tools are optional), 5 on a detected compatibility conflict.")
	flagLine("--watch", "", "Re-run on every .ts / .tsx / .js / .jsx / .json / .d.ts change under the script root. Debounced (150 ms). Ctrl-C exits cleanly. .git / .vscode / node_modules / dotfiles ignored.")
	flagLine("--secrets-prefix", "P", "Namespace prefix for runtime.secrets keystore items (overrides $SERCON_SECRETS_PREFIX; default \"sercon/\").")
	flagLine("--env-file", "PATH", "Load KEY=VALUE pairs from a .env file into the environment before running (repeatable). Real env vars always win; later files override earlier. Replaces the `set -a; source .env` ritual.")
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, s.bold("ARGUMENTS"))
	fmt.Fprintln(w, "    Each positional argument is either a path to a `.ts`/`.tsx` file or")
	fmt.Fprintln(w, "    `-` to read an entry script from standard input. Arguments are run in")
	fmt.Fprintln(w, "    order; their results compose into the final exit code (highest wins).")
	fmt.Fprintln(w, "    Everything after a standalone `--` is passed to the scripts as")
	fmt.Fprintln(w, "    `runtime.argv` (Node/Bun layout: [program, script, ...args]); all")
	fmt.Fprintln(w, "    scripts in one invocation share that argument tail. (With `sercon")
	fmt.Fprintln(w, "    run <script>`, no `--` is needed — tokens after the script are the")
	fmt.Fprintln(w, "    args, which is what shebang scripts rely on.)")
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, s.bold("EXIT STATUS"))
	fmt.Fprintf(w, "    %s   all scripts passed.\n", s.green("0"))
	fmt.Fprintf(w, "    %s   CLI usage error (unknown flag, missing scripts, …).\n", s.yellow("1"))
	fmt.Fprintf(w, "    %s   at least one script failed to transpile (never ran).\n", s.yellow("2"))
	fmt.Fprintf(w, "    %s   at least one script timed out or was context-cancelled.\n", s.yellow("3"))
	fmt.Fprintf(w, "    %s   at least one script ran and threw a JS exception.\n", s.yellow("4"))
	fmt.Fprintln(w, "    When several scripts run, the highest applicable code wins.")
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, s.bold("EXAMPLES"))
	fmt.Fprintf(w, "    %s\n        Run the bundled smoke + async demos.\n",
		s.cyan("sercon examples/scripts/smoke.ts examples/scripts/async.ts"))
	fmt.Fprintf(w, "    %s\n        Show the rich, colourised feature walkthrough.\n",
		s.cyan("sercon --examples"))
	fmt.Fprintf(w, "    %s\n        Emit a declaration file for editor autocomplete.\n",
		s.cyan("sercon --emit-dts sercon.d.ts"))
	fmt.Fprintf(w, "    %s\n        One-liner from a shell pipeline (reads from stdin).\n",
		s.cyan(`echo 'runtime.log(1+2);' | sercon -`))
	fmt.Fprintf(w, "    %s\n        Pass arguments to a script via runtime.argv.\n",
		s.cyan("sercon script.ts -- --port 8080"))
	fmt.Fprintf(w, "    %s\n        Run one script with args (no `--`); shebang-friendly.\n",
		s.cyan("sercon run script.ts --port 8080"))
	fmt.Fprintf(w, "    %s\n        Long-running server with graceful shutdown + access log.\n",
		s.cyan("sercon serve server.ts"))
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, s.bold("SEE ALSO"))
	fmt.Fprintf(w, "    MANUAL.md   in-depth manual (library API, script API, goja built-ins)\n")
	fmt.Fprintf(w, "    README.md   project overview\n")
}

// showExamples walks every CLI-visible feature with a short narrative + a TS
// snippet. Snippets are highlighted with the minimal TS colouriser.
func showExamples(w io.Writer) {
	s := newStyler(w)
	header := func(n int, title string) {
		fmt.Fprintln(w, "")
		fmt.Fprintf(w, "%s %s\n", s.dim(fmt.Sprintf("[%d/%d]", n, exampleCount)), s.bold(s.cyan(title)))
		fmt.Fprintln(w, s.dim(strings.Repeat("─", visibleLen(title)+8)))
	}
	code := func(snippet string) {
		fmt.Fprintln(w, s.highlightTS(strings.TrimRight(snippet, "\n")))
	}
	note := func(msg string) {
		fmt.Fprintln(w, s.dim("  → "+msg))
	}

	fmt.Fprintln(w, s.bold(s.cyan("sercon")), s.dim("— script feature tour"))
	fmt.Fprintln(w, s.dim("Each example is a complete .ts file you can save and run with"))
	fmt.Fprintln(w, s.dim("`sercon path/to/file.ts`. Snippets use the fifteen reserved top-level globals."))

	header(1, "Logging")
	code(`runtime.log("hello", 1 + 2, { a: 1 });
// any arguments are coerced to strings and space-joined`)

	header(2, "Assertions")
	code(`runtime.assert.equal(1 + 1, 2);
runtime.assert.ok([1, 2, 3].length > 0, "non-empty");`)
	note("Failure throws and surfaces as a non-zero exit.")

	header(3, "HTTP — sync via top-level await")
	code(`const r = await net.http.get("https://example.com");
runtime.log(r.status, r.body.length);

const p = await net.http.post("https://httpbin.org/post", "hello");
runtime.log(p.status);`)
	note("net.http.get/post return Promise<{status:number, body:string}>.")

	header(4, "Time")
	code(`const start = runtime.time.nowMs();
await runtime.time.sleep(50);
runtime.log("waited", runtime.time.nowMs() - start, "ms");`)

	header(5, "Environment")
	code(`const home = runtime.env.get("HOME") ?? "(unset)";
runtime.log("home:", home);`)

	header(6, "Shared helpers via import")
	code(`// helpers/assert.ts
export function check(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}

// main.ts
import { check } from "./helpers/assert";
check(2 + 2 === 4, "math broke");`)
	note("import is rewritten to require() at transpile time.")

	header(7, "CommonJS require")
	code(`const { check } = require("./helpers/assert");
check(true, "ok");`)

	header(8, "Promises directly")
	code(`const winner = await Promise.race([
  runtime.time.sleep(20).then(() => "fast"),
  runtime.time.sleep(60).then(() => "slow"),
]);
runtime.log(winner); // → fast`)

	header(9, "Catching Go-side errors")
	code(`try {
  await net.http.get("http://this-host-does-not-resolve.invalid");
} catch (e) {
  runtime.log("caught:", String(e));
}`)
	note("Errors returned from Go bindings throw as JS exceptions.")

	header(10, "Timeouts")
	code(`// sercon -timeout 200ms loop.ts
while (true) {}   // interrupted, exit code 1`)
	note("Both wall-clock timeout and host ctx cancellation interrupt sync JS.")

	header(11, "Goja built-ins (cheatsheet)")
	code(`// All ES5.1 + most ES6 built-ins are available:
const m = new Map<string, number>([["a", 1]]);
const s = new Set([1, 2, 3]);
runtime.log(JSON.stringify([...m]), [...s].reduce((a, b) => a + b, 0));
runtime.log(Math.PI.toFixed(3), new Date().toISOString());`)
	note("See MANUAL.md → 'JavaScript runtime built-ins' for the full list.")

	header(12, "Console + setTimeout (from goja_nodejs)")
	code(`console.log("via console module");
setTimeout(() => console.log("tick"), 10);
await runtime.time.sleep(50);`)

	header(13, "Hashing (crypto.hash.*)")
	code(`runtime.log(crypto.hash.md5("abc"));      // 900150983cd24fb0d6963f7d28e17f72
runtime.log(crypto.hash.sha256("abc"));   // ba7816bf...
runtime.log(crypto.hash.sha3_512("abc")); // SHA-3
runtime.log(crypto.hash.blake3("abc"));   // BLAKE3
runtime.log(crypto.hash.crc32("abc"));    // 352441c2`)
	note("All algos take a UTF-8 string and return lowercase hex (crc32 is zero-padded to 8 chars).")

	header(14, "String utilities (text.str.*)")
	code(`runtime.log(text.str.trim("  hi  "));               // "hi"
runtime.log(text.str.trim("///x///", "/"));          // "x"
runtime.log(text.str.reverse("café"));               // "éfac" (rune-aware)
runtime.log(text.str.stripHtml("<b>bold</b>"));      // "bold"
runtime.log(text.str.base64Encode("hello"));         // "aGVsbG8="
runtime.log(text.str.urlEncode("a b/c"));            // "a+b%2Fc"
runtime.log(text.str.sprintf("%-6s %d", "name", 42)); // "name   42"
runtime.log(text.str.lpad("7", 4, "0"));             // "0007"`)
	note("All members accept JS strings. sprintf uses Go fmt verbs (%s/%d/%x/%.2f/...).")

	header(15, "Paths and time formatting (fs.path.* / runtime.time.format)")
	code(`runtime.log(fs.path.dirname("/a/b/c.txt"));        // "/a/b"
runtime.log(fs.path.basename("/a/b/c.txt", ".txt")); // "c"
runtime.log(runtime.time.format(runtime.time.nowMs(), "%F %T", "UTC"));
// strftime tokens supported: %Y %y %m %d %H %M %S %F %T %j %A %a %B %b %z %Z %%`)

	header(16, "Protocol probes (net.*)")
	code(`// All five return Promises and hit the real network.
const t = await net.probe.tcp("example.com:443");
runtime.log("ip:", t.ip, "latencyMs:", t.latencyMs);

const d = await net.probe.dns("example.com", { types: ["a", "mx"] });
runtime.log("ips:", d.a, "mx:", d.mx);

const c = await net.probe.tls("example.com");
runtime.log("cn:", c.cn, "daysRemaining:", c.daysRemaining);

const n = await net.probe.ntp("pool.ntp.org");
runtime.log("offsetMs:", n.offsetMs, "rttMs:", n.rttMs, "stratum:", n.stratum);

const w = await net.probe.whois("example.com");
runtime.log("registrar:", w.registrar?.name, "expires:", w.domain?.expirationDate);

// net.probe.traceroute — path to a host (needs root). protocol: icmp|udp|tcp.
const hops = await net.probe.traceroute("1.1.1.1", { maxHops: 15 });
for (const h of hops) runtime.log(h.ttl, h.address ?? "*", h.rttsMs);`)
	note("Optional { timeout: ms } on every probe. Default ports: tcp 80, tls 443, ntp 123.")

	header(17, "Compression (codec.compression.*)")
	code(`// Nine pure-Go algorithms with a uniform interface. Inputs are strings
// (UTF-8) or ArrayBuffer; outputs are ArrayBuffer.
runtime.log(codec.compression.algos().join(", "));

const c = await codec.compression.compress("zstd", "hello world");
runtime.log("zstd:", new Uint8Array(c).length, "bytes");

const back = await codec.compression.decompress("zstd", c);
runtime.log("round-trip ok:", Array.from(new Uint8Array(back)).map(b => String.fromCharCode(b)).join(""));`)
	note("All algos round-trip byte-for-byte. gzip / deflate / zlib / bzip2 use stdlib; the others are klauspost / brotli / lz4 / xz / snappy.")

	header(18, "Barcodes (codec.barcode.*)")
	code(`// 10 symbologies (QR, DataMatrix, Aztec, PDF417, Code128, Code39,
// Codabar, EAN-13, EAN-8, UPC-A) behind one encode() call. Output is
// a PNG payload (Uint8Array).
runtime.log(codec.barcode.formats().join(", "));

const qrPng = await codec.barcode.encode("qr", "hello", { width: 256, height: 256 });
runtime.log("QR PNG:", new Uint8Array(qrPng).length, "bytes");

const ean = await codec.barcode.encode("ean13", "5901234123457");
runtime.log("EAN-13 PNG:", new Uint8Array(ean).length, "bytes");`)
	note("Decoders / scanners ship in a later cut (Easy / Encoding part 3).")

	header(19, "Charset detection + conversion (text.charset.*)")
	code(`// Detect: feed bytes, get the top guess + a candidate list.
const sample = await text.charset.encode("café crème", "ISO-8859-1");
const det    = await text.charset.detect(sample);
runtime.log("guess:", det.charset, "@", det.confidence + "%", det.language);

// Round-trip a Japanese string through Shift_JIS.
const sjis = await text.charset.encode("こんにちは", "Shift_JIS");
runtime.log("sjis bytes:", new Uint8Array(sjis).length);
const back = await text.charset.decode(sjis, "Shift_JIS");
runtime.log("back to utf-8:", back);`)
	note("Charset names follow WHATWG aliases (UTF-8, ISO-8859-1, Windows-1252, Shift_JIS, GBK, …).")

	header(20, "Check digits (codec.checkdigit.*)")
	code(`runtime.log(codec.checkdigit.validate("luhn",   "4532015112830366")); // true
runtime.log(codec.checkdigit.validate("isbn13", "9780306406157"));      // true
runtime.log(codec.checkdigit.compute("luhn",  "453201511283036"));      // "6"
runtime.log(codec.checkdigit.compute("isbn10", "048665088"));           // "X"

const r = codec.checkdigit.inspect("luhn", "4532015112830366");
runtime.log(r.valid, r.given, r.computed); // true "6" "6"`)
	note("Supported algos: luhn, isbn10, isbn13, ean13, ean8, upca. Sync — no Promise.")

	header(21, "Archives (fs.archive.*)")
	code(`// Format is inferred from the destination's extension:
//   .zip / .tar / .tar.gz / .tgz
const out = await fs.archive.create("/tmp/demo.zip",
  ["README.md", { path: "src", name: "source-tree" }]);
runtime.log("wrote:", out.path, out.bytes, "bytes,", out.entries.length, "entries");

// Extract back. overwrite: false (default) errors on collisions.
const got = await fs.archive.extract("/tmp/demo.zip", "/tmp/demo-out",
  { overwrite: true });
runtime.log("extracted:", got.entries.length, "entries to", got.dest);`)
	note("Both bindings reject archive entries that try to escape the destination (zip-slip / tar-slip).")

	header(22, "Diff (text.diff.compare)")
	code(`const r = await text.diff.compare("one\ntwo\n", "one\ntwo-edited\nthree\n", {
  fromFile: "old.txt",
  toFile:   "new.txt",
});
runtime.log("added:", r.added, "removed:", r.removed);
runtime.log(r.diff);

// Identical inputs short-circuit; binary inputs (NUL byte) skip diffing.
runtime.log("same:", (await text.diff.compare("abc", "abc")).identical);`)
	note("Inputs are strings (UTF-8) or ArrayBuffer / Uint8Array. Binary inputs return binary:true with empty diff.")

	header(23, "JSON querying (text.jq.*)")
	code(`const data = {
  users: [{ name: "alice", admin: true }, { name: "bob", admin: false }],
};
runtime.log(await text.jq.query(data, ".users[0].name"));          // "alice"
runtime.log(await text.jq.queryAll(data, ".users[].name"));        // ["alice","bob"]
runtime.log(await text.jq.queryAll(data,
  ".users[] | select(.admin) | .name"));                       // ["alice"]`)
	note("Filters are full jq syntax via itchyny/gojq. Missing paths via `?` return null instead of throwing.")

	header(24, "Email authentication (net.email.*)")
	code(`// Five individual probes plus an aggregate. All return
// { present: boolean, ... } and resolve ` + "`present: false`" + ` for NXDOMAIN
// or missing record rather than throwing.
const spf    = await net.email.spf("google.com");
const dmarc  = await net.email.dmarc("google.com");
const mtaSts = await net.email.mtaSts("google.com");
const tlsRpt = await net.email.tlsRpt("google.com");
const bimi   = await net.email.bimi("google.com");

// Or all five in parallel:
const all = await net.email.all("google.com");
runtime.log("dmarc policy:", all.dmarc.policy);
runtime.log("mta-sts mode:", all.mtaSts.policy?.mode);`)
	note("BIMI accepts opts.selector (defaults to 'default').")

	header(25, "Subprocess execution (services.exec.shell)")
	code(`// String cmd → routed through /bin/sh -c (cmd /C on Windows) so pipes,
// redirects and globs work as typed at the prompt.
const piped = await services.exec.shell("echo hi | tr a-z A-Z");
runtime.log(piped.stdout.trim(), "exit:", piped.exitCode);

// Array cmd → argv, no shell expansion.
const literal = await services.exec.shell(["/bin/echo", "literal *"]);

// opts: cwd, env (merged), timeout (ms, default 30000), stdin.
const fed = await services.exec.shell(["/usr/bin/tr", "a-z", "A-Z"], {
  stdin: "hello\n",
  cwd: "/tmp",
  env: { GREETING: "hi" },
  timeout: 5000,
});

// services.exec.stream: same cmd/opts, but output streams to a callback
// line by line as it arrives (no buffering), resolving on exit.
const s = await services.exec.stream("echo one; echo two", (line, stream) => {
  runtime.log(stream, line);
});
runtime.log("stream exit", s.exitCode);`)
	note("Non-zero exits resolve with success:false; timeouts and spawn failures throw.")

	header(26, "HTTP via recon (with curl fallback) (services.exec.http)")
	code(`// Auto backend prefers recon; falls back to curl when recon is missing.
const r = await services.exec.http("GET", "https://httpbin.org/get");
runtime.log(r.status, r.backend, r.durationMs + "ms");

// Force a specific backend if you need one.
const c = await services.exec.http("GET", "https://httpbin.org/get",
  { backend: "curl" });

// POST with custom headers + body.
const p = await services.exec.http("POST", "https://httpbin.org/post", {
  headers: { "X-Sercon": "demo", "Content-Type": "application/json" },
  body: JSON.stringify({ hello: "world" }),
});
runtime.log("echo:", JSON.parse(p.body).data);`)
	note("4xx/5xx resolve as status; transport errors and timeouts throw. opts: timeout, follow, insecure, backend.")

	header(27, "Git CLI wrapper (services.git.*)")
	code(`// Read-only ops — branch / isClean / revParse / status / log /
// diffStat — plus add / commit and a runText escape hatch.
const b = await services.git.branch();              // current + all locals
const head = await services.git.revParse("HEAD");   // 40-char SHA
const clean = await services.git.isClean();         // porcelain check

// log fields: sha, shortSha, author, email, timestamp (unix s), subject.
const recent = await services.git.log({ limit: 3 });
recent.forEach(c => runtime.log(c.shortSha, c.subject));

// diffStat aggregates --shortstat output.
const stat = await services.git.diffStat({ revRange: "HEAD~1..HEAD" });
runtime.log("+" + stat.insertions, "-" + stat.deletions);

// Escape hatch: non-zero exits surface as data, not a throw.
const r = await services.git.runText(["config", "user.email"]);
runtime.log(r.stdout.trim());`)
	note("All bindings accept opts.cwd. add/commit are mutating — guard with isClean / revParse before chaining.")

	header(28, "GitHub CLI wrapper (services.gh.*)")
	code(`// authStatus is a probe — missing gh or unauthenticated session
// resolve as data, not a throw.
const auth = await services.gh.authStatus();
if (!auth.authenticated) { runtime.log("not authed:", auth.raw); }

// repoView with no arg uses cwd's repo; pass "owner/name" for any repo.
const repo = await services.gh.repoView("cli/cli");
runtime.log(repo.owner + "/" + repo.name, "default:", repo.defaultBranch);

// prList — gh's author.login is flattened to a plain string.
const prs = await services.gh.prList({ state: "open", limit: 5 });
prs.forEach(p => runtime.log("#" + p.number, p.title, "(" + p.author + ")"));`)
	note("authStatus never throws on missing-gh; prList/repoView throw with gh's stderr on real failures.")

	header(29, "PHP-style regex (text.preg.*)")
	code(`// /pattern/flags syntax over Go's RE2 — no lookaround / backrefs in
// patterns; use goja's native RegExp if you need those.
const m = text.preg.match("/(\\w+)\\s+(\\d+)/", "alice 30 bob 27");
if (m) runtime.log(m.match, m.groups, m.index);

// matchAll drains every hit, replace uses $1 / ${1} backrefs.
const xs = text.preg.matchAll("/(\\w+)=(\\w+)/", "k1=v1 k2=v2 k3=v3");
const out = text.preg.replace("/(\\w+)@(\\w+)/", "$2/$1", "alice@corp");

// Flags: i / m / s supported; u / U / x and unknown flags throw.
text.preg.match("/HELLO/i", "Hello world");
text.preg.matchAll("/^x/m", "x\\nx\\nx");`)
	note("RE2 is UTF-8 by default — the `u` flag is unnecessary and explicitly errors. Optional groups that didn't match surface as empty strings.")

	header(34, "PCRE regex (text.preg2.*)")
	code(`// Same shape as text.preg, but on dlclark/regexp2 — lookaround,
// backreferences, and the x flag all work (RE2 can't do these).
text.preg2.match("/foo(?=bar)/", "foobar");          // lookahead
text.preg2.match("/(?<=\\$)\\d+/", "$42");           // lookbehind
text.preg2.match("/(\\w+) \\1/", "the the");         // backreference
text.preg2.matchAll("/\\d+/", "a1 b22");             // [1, 22]
text.preg2.replace("/(\\w+)@(\\w+)/", "$2/$1", "alice@corp");`)
	note("No linear-time guarantee — regexp2 backtracks. Prefer text.preg (RE2) unless you need a PCRE feature; keep a timeout around untrusted input.")

	header(35, "HTTP — full client (net.http.request)")
	code(`// Beyond net.http.get/post: headers, body, timeout, retry, auth,
// redirect control. Returns { status, ok, headers, body, url }.
const r = await net.http.request("POST", "https://example/v1", {
  headers: { "Content-Type": "application/json" },
  body: JSON.stringify({ q: "search" }),
  timeout: 5000,
  retry: 2,                  // re-attempt on transport error / 5xx
  username: "user", password: "pass",   // basic auth
});
if (r.ok) runtime.log(r.body); else runtime.log("HTTP", r.status);`)
	fmt.Println("    // multipart upload: net.http.request(\"POST\", url, { multipart: [{ name, value }, { name, filename, content, type }] })")
	note("4xx/5xx are normal responses (r.ok = status in [200,400)); transport errors and timeouts throw. retry never re-attempts 4xx. follow:false surfaces 3xx directly. body also accepts Uint8Array/ArrayBuffer; body and multipart are mutually exclusive.")

	header(30, "JWT — sign / view / validate (crypto.jwt.*)")
	code(`// HMAC — opts.algorithm defaults to HS256.
const t = crypto.jwt.sign(
  { sub: "alice", exp: Math.floor(Date.now() / 1000) + 3600 },
  "shared-secret",
);
const { header, payload, signature } = crypto.jwt.view(t);   // decode w/o verify
const ok = crypto.jwt.validate(t, "shared-secret");
if (ok.valid) runtime.log(ok.claims.sub); else runtime.log("reject:", ok.reason);

// Asymmetric — secret is PEM-encoded (private for sign, public for validate).
const tok = crypto.jwt.sign(
  { sub: "bob" },
  privatePEM,
  { algorithm: "EdDSA" },     // RS256/RS384/RS512, PS256/PS384/PS512,
);                            // ES256/ES384/ES512, EdDSA — all supported
const v = crypto.jwt.validate(tok, publicPEM, { algorithm: "EdDSA" });`)
	note("Set opts.algorithm on validate for asymmetric tokens — that's the algo-confusion guard. PEM/HMAC cross-checks throw at the binding boundary.")

	header(31, "Barcode decode (codec.barcode.decode)")
	code(`// Round-trip a QR. PNG / JPEG / WebP input all work.
const png = await codec.barcode.encode("qr", "round-trip via gozxing");
const r = await codec.barcode.decode(png);                  // auto-detect
runtime.log(r.format, "->", r.text);

// With a format hint, only that reader runs. Mismatched hint throws.
const c = await codec.barcode.decode(png, "qr");

// Decoder set: qr / datamatrix / aztec / code128 / code39 / code93 /
// codabar / ean13 / ean8 / upca / upce / itf.  PDF417 is encode-only
// for now (no pure-Go decoder in gozxing v0.1.1).
runtime.log(codec.barcode.decodableFormats());`)
	note("Code 39 returns the Mod-43 checksum char; codabar strips A…A wrappers. EAN/UPC need a quiet zone — pass encode opts.quietZone:true to round-trip.")

	header(32, "Age encryption (crypto.encrypt.*)")
	code(`// Keygen — bech32 age1... public + AGE-SECRET-KEY-1... private.
const { publicKey, privateKey } = crypto.encrypt.keygen();

// Encrypt — default is binary age format. recipients can be one key
// or an array for multi-recipient (any listed identity can decrypt).
const ct = crypto.encrypt.encrypt("hello world", publicKey);
const ct2 = crypto.encrypt.encrypt(payloadBytes, [alicePub, bobPub]);

// opts.armored = true wraps in age's ASCII armor — safe to embed
// in JSON / YAML / email. Decrypt auto-detects either form.
const armored = crypto.encrypt.encrypt("embed me", publicKey, { armored: true });

// Decrypt — pass any identity that matches the header. Returns
// Uint8Array; use text.charset.decode(bytes, "utf-8") for a string.
const plain = crypto.encrypt.decrypt(ct, privateKey);

// Rekey — rotate recipients without exposing plaintext. Output
// format defaults to match the input (binary / armored).
const rotated = crypto.encrypt.rekey(ct, privateKey, newPublicKey);

// detectBackend — classify a recipient string by backend.
// { backend: "age" | "pgp" | "unknown", kind?: "public" | "private" }
const c = crypto.encrypt.detectBackend(somePubKey);
if (c.backend === "age") /* use crypto.encrypt.encrypt */;
else if (c.backend === "pgp") /* shell out to gpg --encrypt */;`)
	note("Cross-checks catch private-as-recipient and public-as-identity mix-ups with named-key hints. PGP encrypt/decrypt backend lands in a later cut.")

	header(33, "SQLite — stateful DB handle (db.sqlite.*)")
	code(`// open() returns a handle: { exec, query, queryValue, close }.
// ":memory:" is in-RAM; any other string is a file path.
const db = await db.sqlite.open(":memory:");

await db.exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)");
const ins = await db.exec("INSERT INTO users (name) VALUES (?)", "alice");
runtime.log("new id:", ins.lastInsertId, "rows:", ins.rowsAffected);

// query → array of row objects; queryValue → first column of first row.
const rows = await db.query("SELECT id, name FROM users");
const count = await db.queryValue("SELECT count(*) FROM users");

// Transactions: begin() returns a nested handle. commit / rollback.
const tx = await db.begin();
await tx.exec("INSERT INTO users (name) VALUES (?)", "bob");
await tx.commit();   // or tx.rollback() to discard

// Prepared statements: compile once, run many. Methods take only params.
const ins = await db.prepare("INSERT INTO users (name) VALUES (?)");
for (const n of ["carol", "dave"]) await ins.exec(n);
await ins.close();

await db.close();   // no finalizer — always close.`)
	note("Pure-Go driver (modernc.org/sqlite, no cgo). begin() must commit/rollback; prepare() must close. Params bind as ? placeholders; BLOBs round-trip as Uint8Array, TEXT as string.")

	header(36, "Script arguments (runtime.argv)")
	code(`// Everything after a standalone -- on the command line lands in
// runtime.argv, using the Node/Bun layout [program, script, ...args]:
//   sercon run.ts -- --port 8080 hello
runtime.log("program:", runtime.argv[0]);
runtime.log("script:", runtime.argv[1]);
const args = runtime.argv.slice(2);   // ["--port", "8080", "hello"]
runtime.log("args:", JSON.stringify(args));`)
	note("Always present (length >= 2). All scripts in one invocation share the same tail; argv[1] is each script's own path.")

	header(37, "Multi-pane TUI (tui.*)")
	code(`tui.layout({rows: [
  { name: "log", title: "Orchestrator" },
  { cols: [{name:"brew"}, {name:"npm"}], weight: 2 },
]});
const log = tui.pane("log");
log.writeln("Updating Homebrew…");
await services.exec.shell("brew update && brew upgrade", { pane: "brew" });
log.writeln("Updating npm globals…");
await services.exec.shell("npm -g update",              { pane: "npm" });
log.writeln("All done.");`)
	note("Tab cycles focus, PgUp/PgDn scroll. Pipe stdout (CI / make demo) to get prefixed plain-text lines instead.")

	header(49, "Interactive TUI: autoscroll + wait for a key")
	code(`tui.layout({ mouse: true, rows: [{ name: "log" }] });
const log = tui.pane("log");
for (let i = 0; i < 50; i++) log.writeln("line " + i);   // pane follows the tail
log.writeln("Done. Press any key to close.");
await tui.waitKey();`)

	header(50, "Drive a headless browser (services.agentBrowser)")
	code(`// Gate on availability — every method throws cleanly if the CLI is absent.
if (!services.agentBrowser.available) {
  runtime.log("install agent-browser to run this demo");
} else {
  // launch() is synchronous — no browser starts until the first command.
  const b = services.agentBrowser.launch({ headed: false });
  try {
    // open() / get() / fill() / isVisible() / snapshot() are all async.
    await b.open("https://example.com");
    const t = await b.get("title");
    runtime.log("title:", t.data?.title);          // data envelope: { success, data, error }

    await b.fill("#search", "sercon");             // fill an input
    const snap = await b.snapshot({ compact: true });   // accessibility tree
    runtime.log("snap keys:", Object.keys(snap).join(", "));
  } finally {
    await b.close();   // idempotent; Run-end cleanup catches any leaks
  }
}`)
	note("agent-browser --json drives headless Chrome; results arrive as { success, data, error } envelopes.")
	note("launch opts: headed, profile, proxy, userAgent, device, colorScheme, ignoreHttpsErrors, engine, executablePath.")
	note("Handle methods: open/back/forward/reload/wait/connect, click/fill/type/press/check/select/scroll/drag, get/isVisible/isEnabled/isChecked/eval/snapshot/console/errors/highlight, find/locator.")
	fmt.Fprintln(w, "")

	header(51, "Capture & configure a browser (services.agentBrowser Phase 2)")
	code(`// setDefaultOptions() flows opts into every subsequent launch().
if (!services.agentBrowser.available) {
  runtime.log("agent-browser not on PATH — skip");
} else {
  const html = "data:text/html," + encodeURIComponent("<title>demo</title><h1>Hi</h1>");
  services.agentBrowser.setDefaultOptions({ headed: false });

  const b = services.agentBrowser.launch();
  try {
    await b.open(html);
    await b.set.viewport(1280, 800);                    // set.* settings

    // screenshot(path?, opts?) — no path → bytes (number[]); with path → {path,size,format}
    const shot = await b.screenshot({ full: true });
    runtime.log("bytes:", new Uint8Array(shot.bytes).length, shot.format);

    const file = await b.screenshot("/tmp/cap.png");
    runtime.log("file:", JSON.stringify(file));         // { path, size, format }

    const pdf = await b.pdf();                          // pdf() → bytes
    runtime.log("pdf bytes:", new Uint8Array(pdf.bytes).length);
  } finally {
    await b.close();
  }

  // Flat one-shot shortcut — launch+open+act+close in one call.
  const r = await services.agentBrowser.eval(html, "document.title");
  runtime.log("eval:", r.data?.result);                 // "demo"

  services.agentBrowser.clearDefaultOptions();
}`)
	note("Capture result: path given → { path, size, format }; no path → { bytes: number[], format } (wrap with new Uint8Array(bytes) for a typed array).")
	note("set.*: viewport(w,h,scale?), device(name), geo(lat,lng), offline(on?), headers(obj), credentials(user,pass), media(scheme?,reducedMotion?).")
	note("record.*: start(path.webm, url?), stop(). One-shot shortcuts: agentBrowser.screenshot/pdf/snapshot/eval(url, ...).")
	fmt.Fprintln(w, "")

	header(52, "Browser state: cookies, storage, tabs, network (services.agentBrowser)")
	code(`// Phase 3: network interception, cookies, web storage, tab management,
// and page diffing — all as handle methods on the launch() return value.
if (!services.agentBrowser.available) {
  runtime.log("agent-browser not on PATH — skip");
} else {
  const html = "data:text/html," + encodeURIComponent("<title>demo</title><h1>Hi</h1>");
  const b = services.agentBrowser.launch();
  try {
    await b.open(html);

    // Tabs: open a second tab, list, then close it.
    await b.tabs.new(html, { label: "second" });
    const tabs = await b.tabs.list();
    runtime.log("tabs:", JSON.stringify(tabs.data?.tabs?.length), "open");
    await b.tabs.close("second");

    // Network: intercept requests (empty on data: URL, but smoke-tests binding).
    await b.network.route("**/api/*", { abort: true });
    const reqs = await b.network.requests({ clear: true });
    runtime.log("network.requests ok:", reqs.success);

    // Cookies: get the (empty) cookie jar; set needs a real HTTP origin.
    const jar = await b.cookies.get();
    runtime.log("cookies:", jar.data?.cookies?.length ?? 0);

    // Diff: snapshot the current page DOM state.
    const d = await b.diff.snapshot();
    runtime.log("diff.snapshot ok:", d.success);
  } finally {
    await b.close();
  }
}`)
	note("cookies.set + storage.* require a real HTTP origin (data: URLs have no domain). Wrap in try-catch when working with data: URLs. network.har.{start,stop} trace network activity to a HAR file. diff.screenshot compares a saved baseline PNG.")

	header(53, "Browser debug/perf, escape hatch, auth (services.agentBrowser)")
	code(`// Phase 4: vitals, escape-hatch cmd/batch, auth vault — self-skips when
// agent-browser is not installed. Uses a network-free data: URL.
if (!services.agentBrowser.available) {
  runtime.log("agent-browser not on PATH — skip");
} else {
  const html = "data:text/html," + encodeURIComponent("<title>adv</title><h1>hi</h1>");
  const b = services.agentBrowser.launch({ timeout: 8000 });
  try {
    await b.open(html);
    // Core Web Vitals for the current page.
    try { const v = await b.vitals(); runtime.log("vitals:", v.success); }
    catch (e) { runtime.log("vitals skipped"); }
    // Escape hatch: call any agent-browser command.
    try { const r = await b.cmd("get", "title"); runtime.log("title:", r.data?.title); }
    catch (e) { runtime.log("cmd skipped"); }
    // batch: multiple commands in one round-trip (returns an array).
    try { const rs = await b.batch(["get title", "get url"]); runtime.log("batch:", rs.length); }
    catch (e) { runtime.log("batch skipped"); }
  } finally { await b.close(); }
  // Auth vault (namespace-level, no session needed).
  try { const p = await services.agentBrowser.auth.list(); runtime.log("profiles:", p.data); }
  catch (e) { runtime.log("auth.list skipped"); }
}`)
	note("Phase 4 also adds: trace.{start,stop}, profiler.{start,stop}, inspect(), clipboard(op,text?), pushstate(url), react.{tree,inspect,renders,suspense} (needs launch({enable:'react-devtools'})), stream.{enable,disable,status}, chat(msg,opts?) (needs AI gateway), auth.save/show/delete (password via --password-stdin), b.auth.login(name).")

	header(38, "Server (server.http.listen + routes)")
	code(`// Bind an HTTP listener. Routes use stdlib http.ServeMux Go 1.22+
// pattern syntax: "METHOD /path/{param}/{rest...}".
const srv = await server.http.listen({
  port: 8080,
  routes: {
    "GET /":      (req, res) => res.text("hello, world"),
    "GET /json":  (req, res) => res.json({ path: req.path, query: req.query }),
    "POST /echo": (req, res) => res.status(201).json({ echoed: req.body }),
  },
});
runtime.log("listening on", srv.address);  // "tcp/0.0.0.0:8080"
// ... handle requests ...
await srv.close();   // graceful shutdown`)
	note("Handlers serialize on the goja loop — no JS data races, single-threaded throughput ceiling. Vanilla sercon keeps the loop alive while bound; sercon serve adds access log + graceful shutdown.")

	header(39, "Server middleware (onion chain)")
	code(`// Middleware: async (req, res, next) => void. Awaiting next() runs
// the rest of the chain; not awaiting it short-circuits.
const logger = async (req, res, next) => {
  const start = runtime.time.nowMs();
  await next();
  runtime.log(req.method, req.path, "→", runtime.time.nowMs() - start, "ms");
};

await server.http.listen({
  port: 8080,
  use: [logger],                        // global; runs every request
  routes: {
    "GET /": (req, res) => res.text("hi"),
    "GET /api/secure": {                // per-route middleware
      use: [authCheck],
      handler: (req, res) => res.json({ ok: true }),
    },
  },
});`)
	note("Throws bubble to the 500 path. Middleware that doesn't await next() must terminate res itself, else the engine sends 204 No Content.")

	header(40, "Server (server.http.static)")
	code(`// Serve a directory tree at a URL prefix. Route MUST include a
// {rest...} wildcard so subpaths resolve on disk.
const srv = await server.http.listen({
  port: 8081,
  routes: {
    "GET /assets/{rest...}": server.http.static({
      dir: "/var/www/public",
      stripPrefix: "/assets/",
    }),
    "GET /": (req, res) => res.text("home"),
  },
});`)
	note("Wraps stdlib http.FileServer + http.StripPrefix. ETag + range requests work out of the box; symlink escape blocked by http.Dir.")

	header(41, "Server (WebSocket via async iterator)")
	code(`// res.upgradeWebSocket returns a WebSocket that is both an async
// iterator over incoming frames AND has .send / .close methods.
const srv = await server.http.listen({
  port: 8082,
  routes: {
    "GET /ws": async (req, res) => {
      const ws = await res.upgradeWebSocket({ readBuffer: 64 });
      for await (const msg of ws) {
        if (msg.type === "text" && msg.text === "bye") {
          await ws.close(1000, "bye");
          break;
        }
        if (msg.type === "text")   await ws.send("echo:" + msg.text);
        if (msg.type === "binary") await ws.send(msg.bytes);
      }
    },
  },
});`)
	note("Backed by coder/websocket (pure Go). esbuild lowers async generators + for-await; every Run installs Symbol.asyncIterator so the lowering and user code agree on the iteration key.")

	header(42, "SMTP server + sender round-trip (server.smtp.listen + net.email.send)")
	code(`// Inbound: bind an SMTP listener with per-stage callbacks. Each
// handler returns true/undefined (250 accept), false (550), a string
// (550 + reason), or throws (451 temporary). Handlers may be async.
const srv = await server.smtp.listen({
  port: 2525,
  hostname: "mx.example.com",            // EHLO greeting; defaults to os.Hostname()
  handlers: {
    onMail: (env) => env.from.endsWith("@example.com"),
    onRcpt: (env, rcpt) => true,
    onData: (env, msg) => {
      runtime.log("got mail", msg.subject, "from", env.from);
      runtime.log(msg.body.text, "+", msg.attachments.length, "attachments");
      return true;                       // 250 OK
    },
  },
  // auth: (user, pass, env) => user === "bob" && pass === "s3cret",
  // starttls: { cert: "./cert.pem", key: "./key.pem" },
});

// Outbound: net.email.send composes MIME in-tree and returns a
// per-recipient outcome. One TCP connection per call.
const r = await net.email.send({
  to: "alice@example.com",
  from: "noreply@example.com",
  subject: "hello",
  body: "plain text",
  html: "<p>rich</p>",                   // optional multipart/alternative
  server: { host: "127.0.0.1", port: 2525, tls: "none" },
});
runtime.log("accepted", r.accepted, "rejected", r.rejected);

await srv.close();`)
	note("Inbound via emersion/go-smtp + jhillyerd/enmime; outbound MIME composed in-tree. Handlers serialize on the goja loop (one at a time) — acknowledge then process in the background for slow work. `sercon serve` adds a per-stage SMTP access log on stderr. See MANUAL.md §6.7.")

	header(43, "PHP / Perl data dumps (codec.php / codec.perl)")
	code(`// PHP serialize/unserialize round-trip. A "__class" key marks an
// object (PHP O:...); without it a plain array (a:...) is emitted.
const order = { __class: "Order", id: 7, items: ["a", "b"], paid: true };
const s = codec.php.serialize(order);                   // O:5:"Order":3:{...}
runtime.log("unserialize ->", codec.php.unserialize(s));

// var_export / var_dump have matching parsers (var_dump parse is
// best-effort — it throws on lossy markers like *RECURSION*).
const ve = codec.php.varExport(order);                  // valid PHP literal
runtime.log("parseVarExport ->", codec.php.parseVarExport(ve));

// Perl Data::Dumper. JS booleans use the JSON::XS::Boolean convention
// (a blessed scalar ref), so they survive the round-trip as booleans.
const pl = codec.perl.dumper(true);                     // $VAR1 = bless( ... );
runtime.assert.equal(codec.perl.parseDumper(pl), true, "perl bool");

// codec.xml — value <-> XML (@-attrs, #text, arrays = repeated siblings).
const xml = codec.xml.encode({ note: { "@id": "5", "#text": "hi" } });
runtime.log(xml); // <note id="5">hi</note>
runtime.log(JSON.stringify(codec.xml.decode(xml)));`)
	note("Decoded objects keep stable key order (canonical-JSON / payment-hash safe). opts.classKey (default \"__class\"), opts.perlBoolClass (default \"JSON::XS::Boolean\"), opts.indent. See MANUAL.md §codec.")

	header(44, "Console (browser/Node compat)")
	code(`// A console shim so scripts pasted from a browser / Node run as-is.
console.log("hello", 1 + 2);     // -> stdout (like runtime.log)
console.info("fyi");             // -> stdout
console.warn("heads up");        // -> stderr
console.error("oops");           // -> stderr`)
	note("log/info/debug print to stdout, warn/error to stderr — clean, space-joined, no timestamp. runtime.log is the native equivalent.")

	header(45, "SQL servers (postgres / mysql / mssql / clickhouse / oracle)")
	code(`// Same handle as db.sqlite, different engine. DSN string or opts object.
const pg = await db.postgres.open("postgres://user:pass@host:5432/app?sslmode=require");
// …or: db.postgres.open({ host, port, user, password, database, sslmode })
const rows = await pg.query("SELECT id, name FROM users WHERE active = $1", true);
await pg.close();
// db.mysql.open("user:pass@tcp(host:3306)/db")  — placeholders: ?
// db.mssql.open("sqlserver://user:pass@host:1433?database=db")  — placeholders: @p1
// db.clickhouse.open({ host, database, secure: true })  — placeholders: ?
// db.oracle.open("oracle://user:pass@host:1521/service")  — placeholders: :1`)
	note("All six SQL engines (sqlite/postgres/mysql/mssql/clickhouse/oracle) share { exec, query, queryValue, begin, prepare, close }; pure-Go drivers, pinged on open. Write your engine's placeholder syntax.")

	header(46, "Raw sockets (net.tcp / net.udp / net.icmp)")
	code(`// Long-lived client sockets with a push/callback read model (unlike
// the one-shot net.probe.* helpers). Each open() returns a handle:
// onData/onMessage(cb), onClose(cb), onError(cb), close().
const t = await net.tcp.connect("example.com", "80");
t.onData(ev => runtime.log("recv", ev.bytes.length, "bytes:", ev.text));
await t.write("GET / HTTP/1.0\r\n\r\n");
runtime.log("remote", t.remote, "local", t.local);

// UDP — connected { host, port } has send(); a loopback pair self-tests:
const srv = await net.udp.open({ bind: "127.0.0.1:0" });   // srv.local -> 127.0.0.1:PORT
const port = Number(srv.local.split(":").pop());
srv.onMessage(ev => runtime.log("got", ev.text, "from", ev.address, ev.port));
const cli = await net.udp.open({ host: "127.0.0.1", port });
await cli.send("hello-sockets");

// ICMP — raw socket, needs root / CAP_NET_RAW (open rejects otherwise).
// send() builds an Echo-shaped body; type/code customizable.
const ic = await net.icmp.open({ network: "ip4" });
await ic.send({ to: "127.0.0.1", id: 1, seq: 1, payload: "ping" });

await cli.close(); await srv.close(); await t.close();

  // Raw packet engine (needs root / CAP_NET_RAW) — send a SYN, read the reply:
  const reply = await net.raw.tcp("scanme.nmap.org", 80, { flags: ["SYN"] });
  runtime.log(reply ? reply.tcp.flags : "no answer");`)
	note("Inbound events carry bytes (Uint8Array) + text; UDP-bound add address/port, ICMP adds address/type/code. ICMP open() rejects without raw-socket privileges; its body is always Echo-shaped (id/seq/payload). See MANUAL.md §net.")

	header(47, "Raw TCP/UDP/ICMP servers (server.tcp / server.udp / server.icmp)")
	code(`// Inbound counterparts to net.tcp.connect / net.udp.open. Both bind
// synchronously (throw on bind error); port:0 picks an ephemeral port.
// TCP: the handler runs once per accepted socket; conn is the SAME
// handle as net.tcp.connect (onData/onClose/onError/write/close).
const tcp = await server.tcp.listen({ port: 0 }, (conn) => {
  conn.onData(ev => conn.write(ev.bytes));   // echo everything back
  conn.onClose(() => runtime.log("peer gone"));
});
runtime.log("tcp on", tcp.address);          // "tcp/127.0.0.1:PORT"

// UDP: the handler runs once per datagram; reply() answers its sender.
const udp = await server.udp.listen({ port: 0 }, (msg, reply) => {
  runtime.log("got", msg.text, "from", msg.address + ":" + msg.port);
  reply("ack:" + msg.text);
});
runtime.log("udp on", udp.address);          // "udp/127.0.0.1:PORT"

await tcp.close(); await udp.close();

// server.icmp.listen — raw ICMP listener (needs root / CAP_NET_RAW).
// Receives all host ICMP; reply() answers the sender. msg is
// { bytes, text, address, type, code }.
const icmp = server.icmp.listen({}, (msg, reply) => {
  if (msg.type === 8) reply({ type: 0, payload: msg.bytes }); // echo → reply
});
await icmp.close();`)
	note("Same connection-handle shape as the net.tcp/net.udp clients. Both keep the loop alive while bound; `sercon serve` adds a `READY listening on tcp|udp/…` line + graceful shutdown. See MANUAL.md §6.8.")

	header(48, "Packet capture (net.capture)")
	code(`// pure-Go gopacket (no libpcap/cgo). interfaces() + the pcap file
// round-trip are fully offline; live capture is privileged.
const ifaces = net.capture.interfaces();   // [{ name, addresses, up, loopback }]

// Offline file round-trip: write a raw frame, read it back decoded.
const w = net.capture.toFile("/tmp/x.pcap", { snaplen: 65536 });
w.write(rawEthernetFrame, { ts: Date.now() });   // rawEthernetFrame: Uint8Array
await w.close();
await net.capture.openFile("/tmp/x.pcap", (pkt) => {
  runtime.log(pkt.link, pkt.ip?.src, "->", pkt.ip?.dst, pkt.udp?.dstPort);
});

// Optional tcpdump-like filter (post-decode, userspace — not kernel BPF).
// Trailing opts arg on openFile; the 2-arg form still works.
await net.capture.openFile("/tmp/x.pcap", (pkt) => {
  runtime.log("tcp/443:", pkt.ip?.src, "->", pkt.ip?.dst);
}, { filter: "tcp port 443" });

// Live capture — Linux + macOS only, needs root / CAP_NET_RAW (Linux) or
// /dev/bpf (macOS); Windows rejects. Decoded pkt: { ts, length,
// captureLength, link, eth?, ip?, tcp?, udp?, icmp?, payload?, bytes }.
// const cap = await net.capture.open({ iface: "en0", filter: "tcp and port 80" },
//   (pkt) => runtime.log(pkt.ip?.src, "->", pkt.ip?.dst));
// await cap.close();`)
	note("Live open() is Linux/macOS-only and needs raw-socket privileges (Windows rejects); interfaces/openFile/toFile are offline and unprivileged. Optional filter is a tcpdump-like subset (tcp/udp/icmp/ip/ip6, host/net CIDR, port/portrange, src/dst, and/or/not) evaluated post-decode in userspace — not a kernel BPF program; malformed throws. Common-layer decode only (exotic protocols surface as bytes). See MANUAL.md §net.")

	header(54, "Drive a browser via W3C WebDriver (services.webdriver)")
	code(`// W3C WebDriver client — self-skips when no chromedriver/geckodriver is on PATH.
// Uses a data: URL so the integration is fully network-free when a driver is installed.
if (!services.webdriver.available) {
  runtime.log("no chromedriver/geckodriver on PATH — skipping webdriver demo.");
} else {
  const d = await services.webdriver.connect({ browser: "chrome", headless: true });
  try {
    await d.get("data:text/html," + encodeURIComponent(
      "<title>wd demo</title><h1 id=hi>Hello</h1><input id=box>"));
    runtime.log("title:", await d.title());
    const h1 = await d.find("id", "hi");
    runtime.log("h1 text:", await h1.text(), "visible:", await h1.isDisplayed());
    const box = await d.find("css", "#box");
    await box.sendKeys("typed by sercon");
    runtime.log("box value:", await box.getAttribute("value"));
    runtime.log("eval 6*7:", await d.executeScript("return 6*7", []));
    const shot = await d.screenshot();
    runtime.log("screenshot bytes:", new Uint8Array(shot.bytes).length, shot.format);
  } finally {
    await d.quit();
    runtime.log("session quit.");
  }
}`)
	note("services.webdriver.available gates on chromedriver or geckodriver being on PATH.")
	note("connect(opts?) dials a running driver (opts.url) or starts an installed local one. Sessions quit on Run end.")
	note("Locator strategies: css, xpath, id, name, tag, className, linkText, partialLinkText.")
	note("Session: get/url/title/back/forward/refresh/find/findAll/source/screenshot/executeScript/cookies/setCookie/deleteCookie/deleteAllCookies/setImplicitWait/waitFor/quit.")
	note("Element handles: click/sendKeys/clear/submit/text/getAttribute/cssValue/tagName/isDisplayed/isEnabled/isSelected/find/findAll/screenshot.")

	header(55, "OS keystore secrets (runtime.secrets)")
	code(`// runtime.secrets — read/write credentials in the OS keystore (macOS Keychain,
// Linux Secret Service / libsecret, Windows Credential Manager).
// Self-skip on headless CI boxes by checking runtime.secrets.available first.
if (runtime.secrets.available) {
  await runtime.secrets.set("devshop", "tess@example.com", "hunter2");
  const pw = await runtime.secrets.get("devshop", "tess@example.com"); // "hunter2"
  await runtime.secrets.delete("devshop", "tess@example.com");
}
// keystore service = prefix + name (default "sercon/"); override with --secrets-prefix`)
	note("runtime.secrets.available is advisory — false on headless boxes without a keystore daemon.")
	note("get returns null (not undefined) when the entry is absent; delete returns true if removed.")
	note("Prefix namespace: service stored as PREFIX+name. Set prefix via --secrets-prefix or $SERCON_SECRETS_PREFIX.")

	header(56, "Server-Sent Events (res.sse)")
	code(`// res.sse starts a one-way text/event-stream. send() takes a
// string (-> data:) or {event, data, id, retry}; object data is
// JSON-encoded. close() ends it; ` + "`closed`" + ` resolves on close/disconnect.
server.http.listen({
  port: 8080,
  routes: {
    "GET /events": (req, res) => {
      const s = res.sse({ keepAlive: 15000 });
      let n = 0;
      const t = setInterval(() => s.send({ event: "tick", data: { n: n++ } }), 1000);
      s.closed.then(() => clearInterval(t));
      return s.closed;
    },
  },
});`)
	note("The dispatcher parks until the stream closes (the connection isn't hijacked); a pump goroutine owns the writer and flushes each event. keepAlive sends ': ping' comments to defeat idle-proxy timeouts.")

	header(57, "Host clipboard (runtime.clipboard)")
	code(`// Read/write the host OS system clipboard text. available gates it
// (false on headless boxes); read()/write() are async.
if (runtime.clipboard.available) {
  await runtime.clipboard.write("copied from sercon");
  runtime.log("clipboard:", await runtime.clipboard.read());
}
// PNG image I/O is gated separately by imageAvailable.
if (runtime.clipboard.imageAvailable) {
  await runtime.clipboard.writeImage(pngBytes); // Uint8Array, PNG-only
  const png = await runtime.clipboard.readImage(); // Uint8Array | null
}`)
	note("External-CLI fallback: macOS pbcopy/pbpaste, Linux wl-clipboard or xclip/xsel, Windows clip + PowerShell. Throws a clean error when none is installed; the static binary stays fully functional without them.")
	note("Image is PNG-only and feature-detected separately (imageAvailable): macOS image read needs pngpaste, Linux needs wl-clipboard or xclip (not xsel), Windows uses PowerShell.")

	header(58, "HTTP load self-test (net.load.http)")
	code(`// Authorized HTTP load / resilience self-test: a worker pool drives a
// target at a chosen concurrency for a fixed request count (or duration),
// returning latency percentiles + an error rate. Loopback/private hosts are
// always allowed; public targets require confirm:true (authorized use only).
const report = await net.load.http({
  url: "http://127.0.0.1:8080/",
  requests: 200,
  concurrency: 10,
});
runtime.log("rps:", report.rps, "p95 ms:", report.latency.p95, "errorRate:", report.errorRate);`)
	note("Dual-use guardrail: refuses public hosts without confirm:true; concurrency capped at 1000. HTTP-only — no raw packets / spoofing.")

	header(59, "Images (image.decode → transform → encode)")
	code(`// Decode PNG/JPEG/GIF/TIFF/BMP/WebP (or rasterize an SVG subset), then
// chain synchronous, immutable transforms; each op returns a fresh handle.
const im = image.open("avatar.png");          // sniffs format from magic bytes
const thumb = im.resize(128, 0)                // 0 height → preserve aspect
               .grayscale()
               .blur(0.5);
runtime.log("thumb:", thumb.width + "x" + thumb.height);
thumb.save("thumb.webp");                       // webp encode is lossless
const png = im.crop(0, 0, 64, 64).bytes("png"); // → Uint8Array`)
	note("Pure-Go (imaging + x/image + nativewebp + oksvg). resize(0,…)/(…,0) keeps aspect; webp encode is lossless (quality ignored); SVG is rasterize-in only; GIF decode is first-frame.")

	header(60, "Typesetting with Typst (services.typst)")
	code(`// External-CLI binding to the typst compiler — feature-detected on PATH.
// Provide inline source (or a .typ input). With no output, a PDF is
// returned as bytes; with an output path, PNG/SVG/PDF is written there.
if (services.typst.available) {
  const pdf = await services.typst.compile({ source: "= Hello\nFrom Typst." });
  runtime.log(pdf.format, "bytes:", pdf.bytes.length);   // "pdf bytes: …"
  await services.typst.compile({ source: "= Hi", output: "/tmp/out.png", ppi: 144 });
  // query metadata/elements as JSON:
  const v = await services.typst.query({
    source: "#metadata(42) <answer>", selector: "<answer>", field: "value", one: true,
  });
  runtime.log("answer:", v);                              // 42
}`)
	note("Throws cleanly when typst isn't installed (gate on services.typst.available). PDF bytes only without an output path; png/svg require one. Inline source compiles in a temp dir — use input/root for documents that import sibling files.")

	header(61, "Script timeout control (runtime.setDeadline)")
	code(`// Adjust the script's own wall-clock kill deadline at runtime — the same
// deadline the -timeout flag sets. Distinct from the JS global setTimeout,
// which schedules a callback rather than moving the run's kill timer.
runtime.log("ms remaining:", runtime.getDeadline()); // e.g. 9998, or null
runtime.setDeadline(30000);   // give this run 30s from now
runtime.clearDeadline();      // remove the deadline (like -timeout 0)
runtime.log("ms remaining:", runtime.getDeadline()); // null`)
	note("setDeadline(ms) moves the kill deadline to now + ms (ms<=0 disables); clearDeadline() removes it; getDeadline() returns ms remaining or null. Not the JS global setTimeout.")

	header(62, "Browser frames / nested iframes")
	code(`// WebDriver: nested cross-origin frames. switchToFrame takes a CSS
// selector now, and frameChain switches through each level in one call.
const d = await services.webdriver.connect({ browser: "chrome" });
await d.frameChain(["#klarna-checkout-iframe", "#klarna-fullscreen-iframe"]);
const btn = await d.find("css", "button.kco-confirm");   // scoped to the inner frame
await d.switchToDefaultContent();                          // back to the top document

// agentBrowser: single-level frame switch (CDP). frame("main") returns.
const b = await services.agentBrowser.launch();
await b.open("https://shop.example/checkout");
await b.frame("#payment-iframe");   // switch context (one level)
await b.click("#pay");
await b.frame("main");`)
	note("WebDriver does nested + cross-origin frames (W3C /frame; queries are frame-scoped after a switch). agentBrowser.frame() is single-level — agent-browser resolves the selector against the main document and can't descend into nested frames; use WebDriver for nested cases like Klarna Checkout.")

	header(63, "Tool requirements (services.doctor / --doctor)")
	code(`// Report every external tool sercon can use and assert prerequisites.
// CLI form: ` + "`sercon --doctor`" + ` prints a category-grouped table and
// exits 0 (missing tools are optional) or 5 on a compatibility conflict.
const report = await services.doctor();
runtime.log(report.tools.filter((t) => t.installed).length, "tools installed; ok=", report.ok);

// Pass feature/category names (or specific binaries) to assert prerequisites.
const need = await services.doctor(["git"]);
runtime.assert.ok(need.satisfied, "git is required");

// An unmet optional feature lands in unmet (not an error) so a script can self-skip.
const opt = await services.doctor(["typst", "webdriver"]);
runtime.log("unmet:", JSON.stringify(opt.unmet));`)
	note("Missing tools are fine (installed:false, ok:true — optional). Only a compatibility conflict (chromedriver↔Chrome major mismatch) sets ok:false and exits --doctor with code 5. An unknown requirement name throws.")

	header(64, "Reliable clicks in async/iframe UIs (clickWhenReady)")
	code(`// find/element interaction follow the active frame (switchToFrame /
// frameChain). For buttons that render or enable late (e.g. a Klarna
// Checkout "Pay order"), clickWhenReady waits (visible+enabled) in the
// active frame then issues a native (trusted) click that fires React handlers.
const d = await services.webdriver.connect({ browser: "chrome" });
await d.frameChain(["#klarna-checkout-iframe", "#payment-frame"]);
await d.clickWhenReady("css", "button.pay", { timeout: 8000 });
// waitFor also takes { enabled: true } to wait past a disabled→enabled flip.`)
	note("clickWhenReady = waitFor(visible+enabled, in the active frame) + a native trusted click. find already follows the frame chain; what made nested-checkout flows flaky was timing, not frame context.")

	header(65, "Trusted clicks in nested cross-origin iframes (cdpClick)")
	code(`// When the W3C Element Click hit-tests to a parent iframe and throws
// "element click intercepted" (e.g. a Klarna Checkout "Pay order" deep in
// nested cross-origin frames), cdpClick locates the element across the whole
// frame tree and dispatches a trusted CDP mouse click at its real viewport
// coords. Chrome-only. No switchToFrame needed — it pierces every frame.
const d = await services.webdriver.connect({ browser: "chrome" });
await d.get(checkoutUrl);
await d.cdpClick("xpath", '//button[normalize-space()="Pay order"]', { timeout: 8000 });
// Raw escape hatch for any other CDP command:
const ver = await d.cdp("Browser.getVersion", {});`)
	note("cdpClick = locate across the pierced frame tree (DOM.getDocument{pierce}/performSearch) + DOM.getContentQuads + a trusted Input.dispatchMouseEvent. Use it when clickWhenReady throws 'element click intercepted' inside nested cross-origin frames.")

	header(66, "Clicks across out-of-process iframes (OOPIFs)")
	code(`// A cross-site iframe (e.g. a Klarna Checkout on another domain) runs
// out-of-process; chromedriver's page-session CDP can't reach it. cdpClick
// now dispatches input over a browser-level CDP connection, which is
// hit-tested across OOPIF boundaries. targets()/attach() expose the targets.
const d = await services.webdriver.connect({ browser: "chrome" });
await d.get(shopUrl);
await d.cdpClick("xpath", '//button[normalize-space()="Pay order"]', { timeout: 8000 });
// Inspect / drive a specific OOPIF target directly:
const t = (await d.targets()).find(t => /klarna/.test(t.url));
const s = await d.attach(t);
await s.cdp("Runtime.evaluate", { expression: "location.href", returnByValue: true });
await s.detach();`)
	note("A true OOPIF requires a cross-SITE iframe (different eTLD+1) — a different port is same-site and stays in-process. cdpClick attaches to every page/iframe target and clicks in the one that owns the element.")

	header(67, "Read & write files (fs)")
	code(`// General file I/O on the fs global. Writes do not create parent dirs —
// call fs.mkdir first (it is mkdir -p). Paths are used as given.
await fs.mkdir("report");
await fs.writeText("report/index.html", "<h1>Run report</h1>");
await fs.writeBytes("report/shot.png", new Uint8Array(shot.bytes));
if (await fs.exists("report/index.html")) {
  const st = await fs.stat("report/index.html");
  runtime.log("report:", st.size, "bytes,", st.modifiedMs);
}
const html = await fs.readText("report/index.html");
await fs.remove("report"); // file or dir tree; no error if absent`)
	note("fs.writeText/writeBytes fail if the parent directory is missing (Node-like) — fs.mkdir is the mkdir -p. readBytes returns a Uint8Array; stat gives { size, isDir, modifiedMs }. See examples/scripts/fs-report.ts for a screenshot report built on these.")

	header(68, "Bundled libraries (paymentproviders, favro)")
	code(`// Compiled into the binary — import it directly, no install.
import { kcov3 } from "paymentproviders";
const api = kcov3.client();                 // KCO_MERCHANT_ID/KCO_SHARED_SECRET/KCO_ENV from env
const order = await api.getPayment(orderId);
await api.capturePayment(orderId, { amount: order.order_amount });
await api.refundPayment(orderId, { amount: 500 });
import { client } from "favro";  // Favro board/ticket API (pagination, CRUD, attachments)`)
	note("kcov3 (Kustom), netsv1 (Nets), sveacheckout2 (Svea), qlirov2 (Qliro), swedbankpayv2/v3 (SwedbankPay, HAL operations). Non-2xx throws PaymentError{provider,status,body}. Versioned namespaces. See examples/scripts/paymentproviders-*.ts. favro: organizations/collections/widgets/columns/cards/comments/tasks/tasklists/tags/customFields/groups/webhooks, list/listAll/iterate pagination, bounded 429 retry, FavroError. See examples/scripts/favro.ts.")

	header(69, "Fetch & parse web content (web)")
	code(`// Feeds (RSS/Atom/JSON), sitemaps, and lenient HTML — from a string or URL.
const feed = await web.feed.load("https://example.com/feed.xml");
feed.items.forEach(i => console.log(i.title, i.link));

const sm = await web.sitemap.load("https://example.com/sitemap.xml", { expand: true });
console.log(sm.urls.length, "urls");

const doc = await web.html.load("https://example.com");
doc.findAll("a.product").forEach(a => console.log(a.text(), a.attr("href")));
doc.xpathAll("//h2").forEach(h => console.log(h.text()));`)
	note("web.html.parse is lenient (real-world tag soup OK); query with CSS find/findAll or XPath xpath/xpathAll. load() reuses net.http options + a default User-Agent and throws on non-2xx. See examples/scripts/web-*.ts.")

	header(70, "Serve an MCP server (mcp)")
	code(`// Tools/resources/prompts, served over stdio (Claude Desktop, Unix-only)
// or Streamable HTTP (cross-platform, any number of clients).
const srv = mcp.serve({ name: "my-tools", version: "1.0.0" });

srv.tool({
  name: "add",
  description: "add two numbers",
  inputSchema: { type: "object", properties: { a: { type: "number" }, b: { type: "number" } }, required: ["a", "b"] },
  async handler(args) { return String(args.a + args.b); },
});

// Pick ONE transport per handle:
// await srv.stdio();                          // stdin/stdout JSON-RPC (Unix only)
const h = await srv.listen({ port: 38080 });    // Streamable HTTP
runtime.log("listening at", h.url);
await h.close();`)
	note("stdio() keeps stdout pure JSON-RPC (console/runtime output is redirected to stderr) and rejects with a clear error on Windows — use listen() there instead. A tool/resource/prompt handler can return a string, {content}, or {structuredContent}; a thrown/rejected handler becomes an isError tool result, not a crash. See examples/scripts/mcp-server-stdio.ts and examples/scripts/mcp-server-http.ts, and MANUAL.md §5.15 / the generated §17.9 reference.")

	header(71, "MCP cookbook: expose sercon, sampling, OAuth (mcp)")
	code(`// ctx gains sample()/elicit()/roots() — server-initiated calls OUT to the
// client, not just handlers responding to the client's own requests.
srv.tool({
  name: "summarize",
  inputSchema: { type: "object", properties: { text: { type: "string" } }, required: ["text"] },
  async handler(args, ctx) {
    const r = await ctx.sample({
      messages: [{ role: "user", content: { type: "text", text: "Summarize: " + args.text } }],
      maxTokens: 200,
    });
    return r.content.text; // asks the CLIENT's own LLM, not an sercon-side provider
  },
});

srv.onRootsChanged((roots) => runtime.log("roots changed:", JSON.stringify(roots)));

// listen() can also be an OAuth 2.1 resource server:
const h = await srv.listen({
  port: 38080,
  auth: {
    verify: (token) => (token === "good-token" ? { subject: "demo-user", scopes: ["mcp"] } : null),
    resourceMetadata: { authorizationServers: ["https://auth.example.com"] },
  },
});`)
	note("ctx.sample/ctx.elicit/ctx.roots each reject with a clear \"mcp: client does not support <capability>\" error if the connected client never advertised it — there's no silent degrade path, and they only do something interesting against a real client (they're Go-tested, not make-demo scripts; see examples/scripts/mcp-sampling.ts, mcp-elicit.ts, mcp-roots.ts). listen()'s auth option makes sercon an OAuth resource server only — it validates bearer tokens and publishes /.well-known/oauth-protected-resource metadata; it never issues tokens or does dynamic client registration (that's the authorization server's job). Also see examples/scripts/mcp-toolbox.ts (sercon-as-toolbox) and mcp-bridge.ts (bridging a tool to an external API), both make-demo scripts. Full walkthroughs in MANUAL.md §5.15.2.10–§5.15.2.14 and the generated §17.9 reference.")

	header(72, "Connect to an MCP server (mcp.connect)")
	code(`// sercon as an MCP CLIENT: connect to someone else's server, then call
// its tools/resources/prompts. Two transports, same session handle.
const c = await mcp.connect.http("http://127.0.0.1:38080/mcp");
// or: await mcp.connect.stdio({ command: ["sercon", "server.ts"] });

runtime.log("connected to", c.serverInfo.name, c.serverInfo.version);

const result = await c.callTool("add", { a: 2, b: 40 });
runtime.log(result.isError ? "tool errored" : result.content[0].text); // "42"

const doc = await c.readResource("cfg://app");
runtime.log(doc.contents[0].text);

await c.ping();
await c.close();`)
	note("callTool never throws for a tool-level failure — check isError, same soft-failure contract as mcp.serve's tool handlers; a thrown/rejected promise here means the transport or protocol itself failed. Phase 1 is consume-only: no host-side responder yet for the server's own sampling/elicitation/roots calls, no change notifications/subscriptions, no OAuth client — those are later phases. See examples/scripts/mcp-client-http.ts (hermetic, make-demo) and mcp-client-stdio.ts (subprocess-driven, Go-tested via cmd/sercon/mcp_client_test.go), and MANUAL.md §5.15.3 / the generated §17.9.1 reference.")

	header(73, "Case conversion (text.case)")
	code(`// Convert identifiers between naming conventions; auto-detecting split too.
const id = "getHTTPResponseCode";

text.case.snake(id);           // "get_http_response_code"
text.case.kebab(id);           // "get-http-response-code"
text.case.pascal(id);          // "GetHttpResponseCode"
text.case.convert(id, "title"); // "Get Http Response Code" (dynamic dispatch)

text.case.split(id);           // ["get","http","response","code"] (acronym-aware)
text.case.detect("get_http");  // "snake"
text.case.names();             // all 16 canonical case names`)
	note("16 canonical cases (camel/pascal/snake/screamingSnake/ada/camelSnake/kebab/train/screamingKebab/flat/upperFlat/dot/path/title/sentence/capital) plus header/cobol/slug aliases. The tokenizer is acronym-aware (HTTPServer → [http, server]) and works from any input case, not just the one you started with. Pure-Go, no dependencies. See examples/scripts/text-case.ts and MANUAL.md §5.4 / the generated §17.14 reference.")

	header(74, "Redirect your own output (runtime.stdout / stderr / stdin)")
	code(`// Every setter returns an idempotent restore fn; redirects nest.
const restore = runtime.stderr.silence();
await noisyThing();
restore();

// scoped() restores even if the body throws; capture() gives you the text.
const text = await runtime.stdout.capture(() => { console.log("hi"); });

// A file (with tee), a fold onto the other stream, or your own handler.
runtime.stdout.toFile("/tmp/out.log", { append: true, tee: true });
runtime.stderr.to("stdout");
runtime.stdout.to(line => myLogger.info(line));

// stdin is readable and swappable — testable without a real pipe.
runtime.stdin.fromString("a\nb\n");
for await (const line of runtime.stdin.lines()) runtime.log(line);`)
	note("Covers sercon's own writers (console.*, runtime.log, the default-export JSON, PASS/FAIL, --verbose, the TUI fallback). services.exec.shell/.run, services.git, services.gh and services.typst.* capture a child's output into a buffer instead of touching sercon's streams, so redirection is simply irrelevant to them; only services.exec.interactive genuinely bypasses it, by inheriting fd 1/2 (and fd 0) directly. Redirects reset at the start of each script, so `sercon a.ts b.ts` and --watch re-runs start clean, plus once more on the way out (after the last reporting write) so a line callback left pushed can't swallow the result JSON or PASS/FAIL. See examples/scripts/stdio-redirect.ts and MANUAL.md §5.2 / the generated §17.11 reference.")

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, s.dim("End of tour. Run `sercon --help` for flags, or open MANUAL.md."))
}

// exampleCount stays in sync with the header() calls above; bump it when
// adding an example so the [N/M] counters stay correct.
const exampleCount = 74
