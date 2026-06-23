package main

import (
	_ "embed"
	"strings"
)

//go:embed paymentproviders/paymentproviders.bundle.js
var paymentProvidersBundle string

// paymentprovidersLoader returns a ModuleLoader (chained over next) that serves
// the embedded paymentproviders bundle for the bare `paymentproviders` import.
//
// The match is scoped to the node_modules-resolved candidate: a bare specifier
// (`import … from "paymentproviders"`) resolves through node_modules, so the
// engine probes a candidate ending in `/node_modules/paymentproviders.ts`
// (regardless of whether node_modules exists on disk). A user's *relative* file
// (`./paymentproviders.ts`, `./mypaymentproviders.ts`, `assets/…`) never carries
// that prefix, so it is NOT shadowed — it falls through to next/the filesystem.
// Matching the `.ts` candidate (not the extension-less one) lets the engine
// transpile the served source.
func paymentprovidersLoader(next func(string) (string, bool, error)) func(string) (string, bool, error) {
	return func(path string) (string, bool, error) {
		if strings.HasSuffix(path, "/node_modules/paymentproviders.ts") {
			return paymentProvidersBundle, true, nil
		}
		if next != nil {
			return next(path)
		}
		return "", false, nil
	}
}
