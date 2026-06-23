package main

import (
	_ "embed"
	"strings"
)

//go:embed paymentproviders/paymentproviders.bundle.js
var paymentProvidersBundle string

// paymentprovidersLoader returns a ModuleLoader (chained over next) that serves
// the embedded paymentproviders bundle for the bare `paymentproviders` import.
// It matches the `.ts` resolution candidate (not the extension-less one) so the
// engine transpiles the served source; everything else falls through to next.
func paymentprovidersLoader(next func(string) (string, bool, error)) func(string) (string, bool, error) {
	return func(path string) (string, bool, error) {
		if strings.HasSuffix(path, "paymentproviders.ts") {
			return paymentProvidersBundle, true, nil
		}
		if next != nil {
			return next(path)
		}
		return "", false, nil
	}
}
