package main

import (
	_ "embed"
	"strings"
)

//go:embed favro/favro.bundle.js
var favroBundle string

// favroLoader returns a ModuleLoader (chained over next) that serves the
// embedded favro bundle for the bare `favro` import. It matches only the
// node_modules-resolved candidate (`/node_modules/favro.ts`), so a user's
// relative `./favro.ts` is never shadowed — it falls through to next.
func favroLoader(next func(string) (string, bool, error)) func(string) (string, bool, error) {
	return func(path string) (string, bool, error) {
		if strings.HasSuffix(path, "/node_modules/favro.ts") {
			return favroBundle, true, nil
		}
		if next != nil {
			return next(path)
		}
		return "", false, nil
	}
}
