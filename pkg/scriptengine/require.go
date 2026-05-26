package scriptengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja_nodejs/require"
)

// resolutionExts is the ordered list of extensions tried when resolving a
// bare require/import path. Matches Node's behaviour, with .ts prioritised
// so TS files compose cleanly.
var resolutionExts = []string{"", ".ts", ".tsx", ".js", ".cjs", ".mjs", ".json"}

// indexFiles is the list of files tried when a require path resolves to a
// directory.
var indexFiles = []string{"index.ts", "index.tsx", "index.js"}

// jsExtensions enumerates the JavaScript extensions whose presence in a
// require path triggers our `.js -> .ts` swap when the literal file is
// absent. This covers package.json `main` fields that point at compiled
// output (`dist/index.js`) when only the TypeScript source is on disk.
var jsExtensions = []string{".js", ".cjs", ".mjs"}

// newSourceLoader returns a SourceLoader for goja_nodejs/require that:
//   - resolves bare and relative paths against ScriptRoot using Node-style
//     extension fallback (plus .js -> .ts / .tsx swap);
//   - transpiles .ts / .tsx files via esbuild before returning source to
//     the registry;
//   - intercepts package.json reads to prefer a `source` field over `main`
//     when `source` points at a TypeScript file that exists;
//   - is safe to call concurrently (Engine guards its caches).
func (e *Engine) newSourceLoader() require.SourceLoader {
	return func(reqPath string) ([]byte, error) {
		// package.json gets read by goja_nodejs's loadAsDirectory before
		// any module resolution; rewrite `main` -> the TS source when a
		// `source` field is present. Done here rather than in the path
		// resolver because the registry needs the bytes back, not a path.
		if filepath.Base(reqPath) == "package.json" {
			if data, ok := e.maybeRewritePackageJSON(reqPath); ok {
				return data, nil
			}
		}

		resolved, err := e.resolveRequirePath(reqPath)
		if err != nil {
			return nil, err
		}
		if resolved != reqPath {
			e.trace("require resolved %s -> %s", reqPath, resolved)
		} else {
			e.trace("require resolved %s", reqPath)
		}

		raw, err := os.ReadFile(resolved)
		if err != nil {
			return nil, err
		}

		switch filepath.Ext(resolved) {
		case ".ts", ".tsx":
			res, err := transpileTS(string(raw), resolved)
			if err != nil {
				return nil, err
			}
			return []byte(res.JS), nil
		default:
			return raw, nil
		}
	}
}

// resolveRequirePath maps a require/import specifier to an absolute file
// path on disk. The order of preference is:
//
//  1. Exact path (if the literal file exists).
//  2. Same path with .ts / .tsx / .js / .cjs / .mjs / .json appended.
//  3. JS -> TS swap: if the candidate ends in `.js`/`.cjs`/`.mjs` and that
//     file does not exist, try the same path with `.ts` / `.tsx`. Handles
//     package.json `main` fields that point at compiled output where only
//     the TS source is present.
//  4. Directory + index.{ts,tsx,js}.
func (e *Engine) resolveRequirePath(reqPath string) (string, error) {
	candidate := reqPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(e.opts.ScriptRoot, candidate)
	}

	// 1 + 2: literal + extension append.
	for _, ext := range resolutionExts {
		p := candidate + ext
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}

	// 3: JS -> TS swap.
	for _, jsExt := range jsExtensions {
		if !strings.HasSuffix(candidate, jsExt) {
			continue
		}
		base := strings.TrimSuffix(candidate, jsExt)
		for _, tsExt := range []string{".ts", ".tsx"} {
			p := base + tsExt
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p, nil
			}
		}
	}

	// 4: directory with index files.
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		for _, idx := range indexFiles {
			p := filepath.Join(candidate, idx)
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p, nil
			}
		}
	}

	// Return the registry's documented sentinel so goja_nodejs's
	// loadAsFile / loadAsFileOrDirectory chain treats this as "try the
	// next fallback" instead of a hard error.
	return "", require.ModuleFileDoesNotExistError
}

// maybeRewritePackageJSON reads p (a package.json), and if it has a
// `source` field pointing at an existing .ts/.tsx file, returns a JSON
// payload identical to the original except that `main` is replaced with
// that source path. Returns (nil, false) when no rewrite is needed or the
// file can't be read/parsed (let the normal path handle the error).
//
// Convention: `source` is the field used by several bundlers (parcel,
// microbundle, …) to point at the original TS source when `main` ships
// compiled output. Surfacing it lets sercon run TS-first projects that
// follow this convention without a separate build step.
func (e *Engine) maybeRewritePackageJSON(p string) ([]byte, bool) {
	raw, err := os.ReadFile(p)
	if err != nil {
		return nil, false
	}
	var pkg map[string]any
	if err := json.Unmarshal(raw, &pkg); err != nil {
		return nil, false
	}
	source, _ := pkg["source"].(string)
	if source == "" {
		return nil, false
	}
	switch filepath.Ext(source) {
	case ".ts", ".tsx":
	default:
		return nil, false
	}
	sourcePath := filepath.Join(filepath.Dir(p), source)
	if info, err := os.Stat(sourcePath); err != nil || info.IsDir() {
		return nil, false
	}
	pkg["main"] = source
	out, err := json.Marshal(pkg)
	if err != nil {
		return nil, false
	}
	return out, true
}
