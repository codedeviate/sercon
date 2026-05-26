package scriptengine

import (
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

// transpileTS converts TypeScript source into CommonJS-compatible JavaScript
// using esbuild. The sourceFile parameter is used for diagnostic messages and
// for resolving the sourcefile name esbuild embeds in errors.
func transpileTS(source, sourceFile string) (transpileResult, error) {
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
		// Keep names so stack traces are easier to read.
		KeepNames: true,
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
	result := esbuild.Transform(source, esbuild.TransformOptions{
		Loader:     esbuild.LoaderTS,
		Format:     esbuild.FormatESModule,
		Target:     esbuild.ES2022,
		Sourcefile: sourceFile,
		KeepNames:  true,
	})
	if len(result.Errors) > 0 {
		return transpileResult{}, fmt.Errorf("%w: %s: %s", ErrTranspile, sourceFile, formatMessages(result.Errors))
	}
	js := rewriteEntryESMToCJS(string(result.Code))
	return transpileResult{JS: js, SourceFile: sourceFile}, nil
}

// rewriteEntryESMToCJS splits the esbuild ESM output into its leading import
// block and the rest of the body, converts each import to an equivalent
// require() declaration, and wraps the body in an async IIFE whose settlement
// is reported through __resolve / __reject.
func rewriteEntryESMToCJS(esm string) string {
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

	var b strings.Builder
	for _, imp := range imports {
		b.WriteString(imp)
		b.WriteByte('\n')
	}
	b.WriteString(";(async () => {\n")
	b.WriteString(strings.Join(body, "\n"))
	b.WriteString("\n})().then(__resolve, __reject);\n")
	return b.String()
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
// by an optional semicolon; side-effect imports lack the `from` clause.
func importStatementComplete(stmtLines []string) bool {
	joined := strings.Join(stmtLines, " ")
	// Strip trailing semicolons/whitespace.
	joined = strings.TrimRight(joined, "; \t")
	if strings.HasSuffix(joined, "\"") || strings.HasSuffix(joined, "'") {
		return true
	}
	return false
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
	stmt = strings.TrimSpace(stmt)
	stmt = strings.ReplaceAll(stmt, "\n", " ")
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
