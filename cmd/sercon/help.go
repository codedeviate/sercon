package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

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
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
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
	fmt.Fprintf(w, "    %s --examples | --help | --version\n\n", s.cyan("sercon"))

	fmt.Fprintln(w, s.bold("DESCRIPTION"))
	fmt.Fprintln(w, "    Runs one or more TypeScript files against a built-in `api` surface")
	fmt.Fprintln(w, "    (logging, assertions, http, time, env). Each script gets a fresh")
	fmt.Fprintln(w, "    runtime; helpers are loaded via require()/import. See MANUAL.md for")
	fmt.Fprintln(w, "    the full reference.")
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, s.bold("FLAGS"))
	flagLine := func(name, args, desc string) {
		fmt.Fprintf(w, "    %s %s\n        %s\n",
			s.green(name), s.dim(args), desc)
	}
	flagLine("-timeout", "duration", "Per-script wall-clock limit (default 10s; 0 disables).")
	flagLine("-root", "dir", "Script root for require/import resolution (default: dir of the first script).")
	flagLine("-emit-dts", "path", "Write a .d.ts for the example bindings to `path` and exit.")
	flagLine("-v", "", "Verbose: trace the rewritten entry-script JS and each module resolution to stderr; also print duration on script failure.")
	flagLine("--help, -h", "", "Show this help and exit.")
	flagLine("--examples", "", "Show colourised script examples covering every feature; then exit.")
	flagLine("--version", "", "Print the engine version (plus goja / esbuild versions) and exit.")
	fmt.Fprintln(w, "")

	fmt.Fprintln(w, s.bold("ARGUMENTS"))
	fmt.Fprintln(w, "    Each positional argument is either a path to a `.ts`/`.tsx` file or")
	fmt.Fprintln(w, "    `-` to read an entry script from standard input. Arguments are run in")
	fmt.Fprintln(w, "    order; their results compose into the final exit code (highest wins).")
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
		s.cyan("sercon --emit-dts api.d.ts"))
	fmt.Fprintf(w, "    %s\n        One-liner from a shell pipeline (reads from stdin).\n",
		s.cyan(`echo 'api.log(1+2);' | sercon -`))
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
	fmt.Fprintln(w, s.dim("`sercon path/to/file.ts`. Snippets use the built-in `api` surface."))

	header(1, "Logging")
	code(`api.log("hello", 1 + 2, { a: 1 });
// any arguments are coerced to strings and space-joined`)

	header(2, "Assertions")
	code(`api.assert.equal(1 + 1, 2);
api.assert.ok([1, 2, 3].length > 0, "non-empty");`)
	note("Failure throws and surfaces as a non-zero exit.")

	header(3, "HTTP — sync via top-level await")
	code(`const r = await api.http.get("https://example.com");
api.log(r.status, r.body.length);

const p = await api.http.post("https://httpbin.org/post", "hello");
api.log(p.status);`)
	note("api.http.get/post return Promise<{status:number, body:string}>.")

	header(4, "Time")
	code(`const start = api.time.nowMs();
await api.time.sleep(50);
api.log("waited", api.time.nowMs() - start, "ms");`)

	header(5, "Environment")
	code(`const home = api.env.get("HOME") ?? "(unset)";
api.log("home:", home);`)

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
  api.time.sleep(20).then(() => "fast"),
  api.time.sleep(60).then(() => "slow"),
]);
api.log(winner); // → fast`)

	header(9, "Catching Go-side errors")
	code(`try {
  await api.http.get("http://this-host-does-not-resolve.invalid");
} catch (e) {
  api.log("caught:", String(e));
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
api.log(JSON.stringify([...m]), [...s].reduce((a, b) => a + b, 0));
api.log(Math.PI.toFixed(3), new Date().toISOString());`)
	note("See MANUAL.md → 'JavaScript runtime built-ins' for the full list.")

	header(12, "Console + setTimeout (from goja_nodejs)")
	code(`console.log("via console module");
setTimeout(() => console.log("tick"), 10);
await api.time.sleep(50);`)

	header(13, "Hashing (api.hash.*)")
	code(`api.log(api.hash.md5("abc"));      // 900150983cd24fb0d6963f7d28e17f72
api.log(api.hash.sha256("abc"));   // ba7816bf...
api.log(api.hash.sha3_512("abc")); // SHA-3
api.log(api.hash.blake3("abc"));   // BLAKE3
api.log(api.hash.crc32("abc"));    // 352441c2`)
	note("All algos take a UTF-8 string and return lowercase hex (crc32 is zero-padded to 8 chars).")

	header(14, "String utilities (api.str.*)")
	code(`api.log(api.str.trim("  hi  "));               // "hi"
api.log(api.str.trim("///x///", "/"));          // "x"
api.log(api.str.reverse("café"));               // "éfac" (rune-aware)
api.log(api.str.stripHtml("<b>bold</b>"));      // "bold"
api.log(api.str.base64Encode("hello"));         // "aGVsbG8="
api.log(api.str.urlEncode("a b/c"));            // "a+b%2Fc"
api.log(api.str.sprintf("%-6s %d", "name", 42)); // "name   42"
api.log(api.str.lpad("7", 4, "0"));             // "0007"`)
	note("All members accept JS strings. sprintf uses Go fmt verbs (%s/%d/%x/%.2f/...).")

	header(15, "Paths and time formatting (api.path.* / api.time.format)")
	code(`api.log(api.path.dirname("/a/b/c.txt"));        // "/a/b"
api.log(api.path.basename("/a/b/c.txt", ".txt")); // "c"
api.log(api.time.format(api.time.nowMs(), "%F %T", "UTC"));
// strftime tokens supported: %Y %y %m %d %H %M %S %F %T %j %A %a %B %b %z %Z %%`)

	header(16, "Protocol probes (api.net.*)")
	code(`// All five return Promises and hit the real network.
const t = await api.net.tcp("example.com:443");
api.log("ip:", t.ip, "latencyMs:", t.latencyMs);

const d = await api.net.dns("example.com", { types: ["a", "mx"] });
api.log("ips:", d.a, "mx:", d.mx);

const c = await api.net.tls("example.com");
api.log("cn:", c.cn, "daysRemaining:", c.daysRemaining);

const n = await api.net.ntp("pool.ntp.org");
api.log("offsetMs:", n.offsetMs, "rttMs:", n.rttMs, "stratum:", n.stratum);

const w = await api.net.whois("example.com");
api.log("registrar:", w.registrar?.name, "expires:", w.domain?.expirationDate);`)
	note("Optional { timeout: ms } on every probe. Default ports: tcp 80, tls 443, ntp 123.")

	header(17, "Compression (api.compression.*)")
	code(`// Nine pure-Go algorithms with a uniform interface. Inputs are strings
// (UTF-8) or ArrayBuffer; outputs are ArrayBuffer.
api.log(api.compression.algos().join(", "));

const c = await api.compression.compress("zstd", "hello world");
api.log("zstd:", new Uint8Array(c).length, "bytes");

const back = await api.compression.decompress("zstd", c);
api.log("round-trip ok:", Array.from(new Uint8Array(back)).map(b => String.fromCharCode(b)).join(""));`)
	note("All algos round-trip byte-for-byte. gzip / deflate / zlib / bzip2 use stdlib; the others are klauspost / brotli / lz4 / xz / snappy.")

	header(18, "Email authentication (api.email.*)")
	code(`// Five individual probes plus an aggregate. All return
// { present: boolean, ... } and resolve `+"`present: false`"+` for NXDOMAIN
// or missing record rather than throwing.
const spf    = await api.email.spf("google.com");
const dmarc  = await api.email.dmarc("google.com");
const mtaSts = await api.email.mtaSts("google.com");
const tlsRpt = await api.email.tlsRpt("google.com");
const bimi   = await api.email.bimi("google.com");

// Or all five in parallel:
const all = await api.email.all("google.com");
api.log("dmarc policy:", all.dmarc.policy);
api.log("mta-sts mode:", all.mtaSts.policy?.mode);`)
	note("BIMI accepts opts.selector (defaults to 'default').")

	fmt.Fprintln(w, "")
	fmt.Fprintln(w, s.dim("End of tour. Run `sercon --help` for flags, or open MANUAL.md."))
}

// exampleCount stays in sync with the header() calls above; bump it when
// adding an example so the [N/M] counters stay correct.
const exampleCount = 18
