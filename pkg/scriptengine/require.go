package scriptengine

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dop251/goja_nodejs/require"
)

// resolutionExts is the ordered list of extensions tried when resolving a
// bare require/import path. Matches Node's behaviour, with .ts prioritised
// so TS files compose cleanly.
var resolutionExts = []string{"", ".ts", ".tsx", ".js", ".cjs", ".mjs", ".json"}

// indexFiles is the list of files tried when a require path resolves to a
// directory.
var indexFiles = []string{"index.ts", "index.tsx", "index.js"}

// newSourceLoader returns a SourceLoader for goja_nodejs/require that:
//   - resolves bare and relative paths against ScriptRoot using Node-style
//     extension fallback;
//   - transpiles .ts/.tsx files via esbuild before returning source to the
//     registry;
//   - is safe to call concurrently (Engine guards its caches).
func (e *Engine) newSourceLoader() require.SourceLoader {
	return func(reqPath string) ([]byte, error) {
		resolved, err := e.resolveRequirePath(reqPath)
		if err != nil {
			return nil, err
		}

		// Read the file from disk. The require registry handles its own
		// per-runtime module caching; we only need to hand it source.
		raw, err := os.ReadFile(resolved)
		if err != nil {
			return nil, err
		}

		ext := filepath.Ext(resolved)
		switch ext {
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
// path on disk, applying Node-style extension and index-file fallback.
func (e *Engine) resolveRequirePath(reqPath string) (string, error) {
	// The require registry passes paths that may already be absolute (for
	// resolved cases) or relative (for the initial lookup, with ScriptRoot
	// as base via the path resolver default).
	candidate := reqPath
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(e.opts.ScriptRoot, candidate)
	}

	// Try exact path and extension variants.
	for _, ext := range resolutionExts {
		p := candidate + ext
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}

	// Try as directory with index files.
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		for _, idx := range indexFiles {
			p := filepath.Join(candidate, idx)
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				return p, nil
			}
		}
	}

	return "", fmt.Errorf("%w: %s", errModuleNotFound, reqPath)
}

var errModuleNotFound = errors.New("module not found")
