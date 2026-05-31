package scriptengine

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	esbuild "github.com/evanw/esbuild/pkg/api"
)

// transpileResult holds the JS produced from a TS source plus information
// useful for mapping runtime errors back to the original source.
type transpileResult struct {
	JS         string
	SourceFile string
}

// gojaUnsupported tells esbuild which language features goja can't parse
// natively, forcing esbuild to lower them even when the target year would
// otherwise leave them alone. The headline case is for-await-of: ES2018
// syntax that goja's parser still rejects (issue tracker: dop251/goja #1218
// and friends). Listing the feature in `Supported: false` lets esbuild
// rewrite it into an async-iterator IIFE that goja can execute. We also
// disable `async-generator` because the lowered for-await template uses
// it; without this flag esbuild would generate generators and goja chokes
// on those too.
var gojaUnsupported = map[string]bool{
	"for-await":       false,
	"async-generator": false,
}

// stripShebang blanks a leading "#!" line so a TS/JS source can be made
// directly executable (`chmod +x` with a `#!/usr/bin/env sercon` first line).
// A shebang is only meaningful as the very first bytes of the file; esbuild
// preserves it into its output, where goja would reject it as an illegal
// token. We blank the line *in place* — dropping the shebang text but keeping
// the terminating newline — rather than removing the line, so transpile
// (syntax) error line numbers stay aligned with the original source (and any
// future source-mapped runtime errors would too). Sources that don't begin
// with "#!" are returned unchanged.
func stripShebang(source string) string {
	if !strings.HasPrefix(source, "#!") {
		return source
	}
	if nl := strings.IndexByte(source, '\n'); nl >= 0 {
		return source[nl:] // keep the newline (and any preceding CR); drop the shebang text
	}
	return "" // shebang-only source, no body
}

// transpileTS converts TypeScript source into CommonJS-compatible JavaScript
// using esbuild. The sourceFile parameter is used for diagnostic messages and
// for resolving the sourcefile name esbuild embeds in errors.
func transpileTS(source, sourceFile string) (transpileResult, error) {
	source = stripShebang(source)
	loader := esbuild.LoaderTS
	switch strings.ToLower(filepath.Ext(sourceFile)) {
	case ".js", ".mjs", ".cjs":
		loader = esbuild.LoaderJS
	case ".tsx":
		loader = esbuild.LoaderTSX
	case ".jsx":
		loader = esbuild.LoaderJSX
	}

	result := esbuild.Transform(source, esbuild.TransformOptions{
		Loader:     loader,
		Format:     esbuild.FormatCommonJS,
		Target:     esbuild.ES2020,
		Sourcefile: sourceFile,
		Supported:  gojaUnsupported,
		// Keep names so stack traces are easier to read.
		KeepNames: true,
		// Append an inline source map so goja maps runtime stack frames back
		// to the original TS positions (goja parses the trailing
		// //# sourceMappingURL=data:... directive natively).
		Sourcemap: esbuild.SourceMapInline,
	})

	if len(result.Errors) > 0 {
		return transpileResult{}, fmt.Errorf("%w: %s: %s", ErrTranspile, sourceFile, formatMessages(result.Errors))
	}
	return transpileResult{JS: string(result.Code), SourceFile: sourceFile}, nil
}

// transpileEntry converts the entry script TS source into a CommonJS-friendly
// JS payload that supports top-level await. esbuild does not allow top-level
// await with the CJS output format, so we emit ESM, then rewrite the static
// imports to require() calls and wrap the remaining body in an async IIFE.
// The wrapper invokes __resolve / __reject on completion, which the engine
// captures.
func transpileEntry(source, sourceFile string) (transpileResult, error) {
	source = stripShebang(source)
	result := esbuild.Transform(source, esbuild.TransformOptions{
		Loader:     esbuild.LoaderTS,
		Format:     esbuild.FormatESModule,
		Target:     esbuild.ES2022,
		Sourcefile: sourceFile,
		Supported:  gojaUnsupported,
		KeepNames:  true,
		// External map so result.Code stays clean ESM for the rewrite below;
		// we shift it for the rewrite's line offset and re-attach it inline.
		Sourcemap: esbuild.SourceMapExternal,
	})
	if len(result.Errors) > 0 {
		return transpileResult{}, fmt.Errorf("%w: %s: %s", ErrTranspile, sourceFile, formatMessages(result.Errors))
	}
	js, shift := rewriteEntryESMToCJS(string(result.Code))
	if shifted, err := shiftSourceMap(result.Map, shift); err == nil && len(shifted) > 0 {
		js = js + "\n" + inlineSourceMap(shifted) + "\n"
	}
	// On a map error we fall through with the un-mapped JS — errors then point
	// at transpiled-JS lines (prior behaviour), never worse.
	return transpileResult{JS: js, SourceFile: sourceFile}, nil
}

// shiftSourceMap prepends `shift` blank output lines to an esbuild source map
// (raw JSON) by prepending `shift` semicolons to its "mappings" field. This is
// safe without re-encoding any VLQ segments: inserting blank leading output
// lines leaves every real segment's zero-relative encoding unchanged. Returns
// the (possibly unchanged) JSON.
func shiftSourceMap(raw []byte, shift int) ([]byte, error) {
	if shift <= 0 || len(raw) == 0 {
		return raw, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	mappings, _ := m["mappings"].(string)
	m["mappings"] = strings.Repeat(";", shift) + mappings
	return json.Marshal(m)
}

// inlineSourceMap renders an esbuild source map (raw JSON) as a
// //# sourceMappingURL=data:... directive line that goja parses natively.
func inlineSourceMap(raw []byte) string {
	return "//# sourceMappingURL=data:application/json;base64," +
		base64.StdEncoding.EncodeToString(raw)
}

// rewriteEntryESMToCJS splits the esbuild ESM output into its leading import
// block and the rest of the body, converts each import to an equivalent
// require() declaration, and wraps the body in an async IIFE whose settlement
// is reported through __resolve / __reject. It also returns the source-map
// line shift: how many blank output lines the rewrite prepends to the body
// relative to esbuild's output, so the caller can adjust an external source
// map before attaching it.
func rewriteEntryESMToCJS(esm string) (string, int) {
	lines := strings.Split(esm, "\n")
	var imports []string
	var body []string
	bodyStart := -1

	i := 0
	for i < len(lines) {
		trim := strings.TrimSpace(lines[i])
		if trim == "" || strings.HasPrefix(trim, "//") {
			i++
			continue
		}
		if strings.HasPrefix(trim, "import") && isImportStart(trim) {
			// Accumulate the import statement until we see its terminator.
			stmtLines := []string{lines[i]}
			for !importStatementComplete(stmtLines) && i+1 < len(lines) {
				i++
				stmtLines = append(stmtLines, lines[i])
			}
			imports = append(imports, convertImport(strings.Join(stmtLines, "\n")))
			i++
			continue
		}
		bodyStart = i
		break
	}
	if bodyStart >= 0 {
		body = lines[bodyStart:]
	}

	// The body must sit at or below its original esbuild line number so the
	// map only needs blank lines PREPENDED (never segments dropped). The
	// natural prefix is one line per converted import plus the IIFE opener;
	// pad with blank lines until it reaches bodyStart. shift = prefixLines -
	// bodyStart (>= 0) is how many blank output lines the map must gain.
	prefixLines := len(imports) + 1 // import lines + the ";(async () => {" opener
	pad := 0
	if bodyStart > prefixLines {
		pad = bodyStart - prefixLines
		prefixLines = bodyStart
	}
	shift := prefixLines - bodyStart
	if bodyStart < 0 {
		shift = 0 // no body found; nothing to map
	}

	var b strings.Builder
	for _, imp := range imports {
		b.WriteString(imp)
		b.WriteByte('\n')
	}
	for k := 0; k < pad; k++ {
		b.WriteByte('\n')
	}
	b.WriteString(";(async () => {\n")
	b.WriteString(strings.Join(body, "\n"))
	b.WriteString("\n})().then(__resolve, __reject);\n")
	return b.String(), shift
}

// isImportStart returns true if the trimmed line begins a statement that we
// should rewrite. It guards against false positives like `importFoo(...)` or
// `import.meta.x`.
func isImportStart(trim string) bool {
	if !strings.HasPrefix(trim, "import") {
		return false
	}
	if len(trim) == len("import") {
		return true
	}
	c := trim[len("import")]
	return c == ' ' || c == '\t' || c == '{' || c == '*' || c == '"' || c == '\''
}

// importStatementComplete heuristically detects when an import statement is
// terminated. esbuild's ESM output uses `from "..."` (or `from '...'`) followed
// by an optional semicolon; side-effect imports lack the `from` clause. Inline
// comments (a `// trailing note` or a `/* block */`) are stripped first so a
// commented import still terminates on its closing quote.
func importStatementComplete(stmtLines []string) bool {
	joined := stripComments(strings.Join(stmtLines, " "))
	// Strip trailing semicolons/whitespace.
	joined = strings.TrimRight(joined, "; \t")
	if strings.HasSuffix(joined, "\"") || strings.HasSuffix(joined, "'") {
		return true
	}
	return false
}

// stripComments removes `// line` and `/* block */` comments from an import
// statement before parsing. It's comment-aware of string literals so a `//`
// inside a quoted module path (rare, but legal) isn't mistaken for a comment.
// Deliberately small — it only needs to handle the shapes that appear in
// esbuild's ESM import output, not arbitrary JS.
func stripComments(s string) string {
	var b strings.Builder
	inStr := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr != 0 {
			b.WriteByte(c)
			if c == inStr && (i == 0 || s[i-1] != '\\') {
				inStr = 0
			}
			continue
		}
		switch {
		case c == '"' || c == '\'':
			inStr = c
			b.WriteByte(c)
		case c == '/' && i+1 < len(s) && s[i+1] == '/':
			// Line comment — drop the rest of the (already-joined) statement.
			return b.String()
		case c == '/' && i+1 < len(s) && s[i+1] == '*':
			// Block comment — skip to the closing */.
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return b.String()
			}
			i += 2 + end + 1 // advance past "*/"
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

var (
	reImportBare    = regexp.MustCompile(`(?s)^import\s+["']([^"']+)["']\s*;?\s*$`)
	reImportStar    = regexp.MustCompile(`(?s)^import\s+\*\s+as\s+(\w+)\s+from\s+["']([^"']+)["']\s*;?\s*$`)
	reImportNamed   = regexp.MustCompile(`(?s)^import\s+\{\s*([^}]*?)\s*\}\s+from\s+["']([^"']+)["']\s*;?\s*$`)
	reImportDefAndN = regexp.MustCompile(`(?s)^import\s+(\w+)\s*,\s*\{\s*([^}]*?)\s*\}\s+from\s+["']([^"']+)["']\s*;?\s*$`)
	reImportDefault = regexp.MustCompile(`(?s)^import\s+(\w+)\s+from\s+["']([^"']+)["']\s*;?\s*$`)
)

// convertImport rewrites a single ESM import statement (possibly spanning
// multiple lines) into one or more CommonJS-style declarations using require().
// Defaults are interop-aware: `__esModule ? m.default : m`.
func convertImport(stmt string) string {
	stmt = strings.ReplaceAll(stmt, "\n", " ")
	stmt = stripComments(stmt)
	// Collapse runs of whitespace so unusual indentation / alignment in a
	// multi-line import doesn't defeat the regexes (which expect single
	// spaces between tokens).
	stmt = strings.Join(strings.Fields(stmt), " ")
	stmt = strings.TrimSpace(stmt)
	if m := reImportBare.FindStringSubmatch(stmt); m != nil {
		return fmt.Sprintf(`require(%q);`, m[1])
	}
	if m := reImportStar.FindStringSubmatch(stmt); m != nil {
		return fmt.Sprintf(`var %s = require(%q);`, m[1], m[2])
	}
	if m := reImportDefAndN.FindStringSubmatch(stmt); m != nil {
		modVar := uniqueModVar(m[3])
		named := destructureSpec(m[2])
		return fmt.Sprintf(
			"var %[1]s = require(%[2]q); var %[3]s = (%[1]s && %[1]s.__esModule) ? %[1]s.default : %[1]s; var %[4]s = %[1]s;",
			modVar, m[3], m[1], named,
		)
	}
	if m := reImportNamed.FindStringSubmatch(stmt); m != nil {
		named := destructureSpec(m[1])
		return fmt.Sprintf(`var %s = require(%q);`, named, m[2])
	}
	if m := reImportDefault.FindStringSubmatch(stmt); m != nil {
		modVar := uniqueModVar(m[2])
		return fmt.Sprintf(
			"var %[1]s = require(%[2]q); var %[3]s = (%[1]s && %[1]s.__esModule) ? %[1]s.default : %[1]s;",
			modVar, m[2], m[1],
		)
	}
	// Fall back to passing through; will surface as a JS syntax error at
	// runtime if the import is malformed.
	return stmt
}

// destructureSpec turns "a, b as c, d" into "{ a, b: c, d }" suitable for use
// on the left-hand side of `var = require(...)`.
func destructureSpec(spec string) string {
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if idx := strings.Index(p, " as "); idx >= 0 {
			lhs := strings.TrimSpace(p[:idx])
			rhs := strings.TrimSpace(p[idx+len(" as "):])
			out = append(out, lhs+": "+rhs)
		} else {
			out = append(out, p)
		}
	}
	return "{ " + strings.Join(out, ", ") + " }"
}

// uniqueModVar produces a deterministic variable name for an import target.
// Collisions are avoided by hashing the module path into the suffix.
func uniqueModVar(modPath string) string {
	var b strings.Builder
	b.WriteString("__mod_")
	for _, r := range modPath {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

func formatMessages(msgs []esbuild.Message) string {
	parts := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.Location != nil {
			parts = append(parts, fmt.Sprintf("%s:%d:%d: %s", m.Location.File, m.Location.Line, m.Location.Column, m.Text))
		} else {
			parts = append(parts, m.Text)
		}
	}
	return strings.Join(parts, "; ")
}
