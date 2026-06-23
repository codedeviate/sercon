package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// newPPEngine builds an engine with the paymentproviders loader + full surface.
func newPPEngine(t *testing.T) *scriptengine.Engine {
	t.Helper()
	opts := scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second}
	opts.ModuleLoader = paymentprovidersLoader(opts.ModuleLoader)
	eng := scriptengine.New(opts)
	if err := registerSurface(eng); err != nil {
		t.Fatalf("registerSurface: %v", err)
	}
	return eng
}

func TestPaymentProviders_ImportResolves(t *testing.T) {
	eng := newPPEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { kcov3 } from "paymentproviders";
		const c = kcov3.client({ merchantId: "m", sharedSecret: "s", env: "test" });
		if (typeof c.getPayment !== "function") throw new Error("no getPayment");
		if (typeof c.capturePayment !== "function") throw new Error("no capturePayment");
		if (typeof c.refundPayment !== "function") throw new Error("no refundPayment");
		if (typeof c.cancelPayment !== "function") throw new Error("no cancelPayment");
	`)
	if err != nil {
		t.Fatalf("import paymentproviders: %v", err)
	}
}

func TestPaymentProviders_MissingCredsThrows(t *testing.T) {
	// Ensure creds are unset in this process so the throw path is exercised
	// even if the host environment defines them.
	t.Setenv("KCO_MERCHANT_ID", "")
	t.Setenv("KCO_SHARED_SECRET", "")
	eng := newPPEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { kcov3 } from "paymentproviders";
		let threw = false;
		try { kcov3.client({ env: "test" }); } catch (e) { threw = true; if (!String(e).includes("KCO_MERCHANT_ID")) throw e; }
		if (!threw) throw new Error("expected missing-cred throw");
	`)
	if err != nil {
		t.Fatalf("missing-creds: %v", err)
	}
}
