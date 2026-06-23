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

func TestPaymentProviders_KcoMockRoundtrip(t *testing.T) {
	eng := newPPEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { kcov3 } from "paymentproviders";

		const seen: any[] = [];
		const srv = await server.http.listen({ port: 38270, routes: {
			"GET /ordermanagement/v1/orders/ord_1": (q: any, r: any) => {
				seen.push({ m: "GET", path: q.path, auth: q.headers["authorization"][0] });
				return r.json({ order_id: "ord_1", status: "AUTHORIZED", order_amount: 1000 });
			},
			"POST /ordermanagement/v1/orders/ord_1/captures": (q: any, r: any) => {
				seen.push({ m: "POST", path: q.path, auth: q.headers["authorization"][0],
				            idem: q.headers["klarna-idempotency-key"][0], body: q.body });
				return r.status(201).json({ capture_id: "cap_1" });
			},
			"GET /ordermanagement/v1/orders/missing": (q: any, r: any) => r.status(404).json({ error_code: "NOT_FOUND" }),
		}});
		try {
			const api = kcov3.client({ merchantId: "M", sharedSecret: "S", baseUrl: "http://127.0.0.1:38270" });

			const order = await api.getPayment("ord_1");
			runtime.assert.equal(order.status, "AUTHORIZED", "parsed order status");
			runtime.assert.equal(seen[0].auth, "Basic TTpT", "basic auth header"); // base64("M:S")

			const cap = await api.capturePayment("ord_1", { amount: 1000 });
			runtime.assert.equal(cap.capture_id, "cap_1", "parsed capture id");
			const c = seen[1];
			runtime.assert.ok(c.idem && c.idem.length > 0, "idempotency key sent");
			runtime.assert.equal(JSON.parse(c.body).captured_amount, 1000, "capture body amount");

			let perr: any = null;
			try { await api.getPayment("missing"); } catch (e) { perr = e; }
			runtime.assert.ok(perr, "missing order threw");
			runtime.assert.equal(perr.status, 404, "PaymentError status");
			runtime.assert.equal(perr.provider, "kcov3", "PaymentError provider");
		} finally {
			await srv.close();
		}
	`)
	if err != nil {
		t.Fatalf("kco mock roundtrip: %v", err)
	}
}
