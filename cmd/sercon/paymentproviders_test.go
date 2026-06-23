package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
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

func TestPaymentProviders_NetsMockRoundtrip(t *testing.T) {
	eng := newPPEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { netsv1 } from "paymentproviders";
		const seen: any[] = [];
		const srv = await server.http.listen({ port: 38272, routes: {
			"POST /v1/payments": (q: any, r: any) => { seen.push({ path: q.path, auth: q.headers["authorization"][0], body: q.body }); return r.status(201).json({ paymentId: "pay_1" }); },
			"GET /v1/payments/pay_1": (q: any, r: any) => { seen.push({ path: q.path, auth: q.headers["authorization"][0] }); return r.json({ payment: { paymentId: "pay_1" } }); },
			"POST /v1/payments/pay_1/charges": (q: any, r: any) => { seen.push({ path: q.path, body: q.body }); return r.status(201).json({ chargeId: "ch_1" }); },
		}});
		try {
			const api = netsv1.client({ secretKey: "sk_test_123", baseUrl: "http://127.0.0.1:38272" });
			const created = await api.createPayment({ order: { amount: 1000 } });
			runtime.assert.equal(created.paymentId, "pay_1", "create parsed");
			runtime.assert.equal(seen[0].auth, "sk_test_123", "secret key is the Authorization header verbatim");
			const got = await api.getPayment("pay_1");
			runtime.assert.equal(got.payment.paymentId, "pay_1", "get parsed");
			const ch = await api.capturePayment("pay_1", { amount: 1000 });
			runtime.assert.equal(ch.chargeId, "ch_1", "charge parsed");
			runtime.assert.equal(JSON.parse(seen[2].body).amount, 1000, "charge body amount");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("nets mock roundtrip: %v", err)
	}
}

func TestPaymentProviders_SveaSignerRoundtrip(t *testing.T) {
	eng := newPPEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { sveacheckout2 } from "paymentproviders";
		const MID = "123123", SECRET = "sharedSecret";
		let verified = false, sawTimestamp = "";
		const srv = await server.http.listen({ port: 38273, routes: {
			"POST /api/orders": (q: any, r: any) => {
				const ts = q.headers["timestamp"][0];
				sawTimestamp = ts;
				const expectHash = crypto.hash.sha512(q.body + SECRET + ts).toUpperCase();
				const expectToken = "Svea " + text.str.base64Encode(MID + ":" + expectHash);
				verified = q.headers["authorization"][0] === expectToken;
				return r.status(201).json({ OrderId: 777 });
			},
		}});
		try {
			const api = sveacheckout2.client({ merchantId: MID, secretKey: SECRET, baseUrl: "http://127.0.0.1:38273" });
			const o = await api.createOrder({ Currency: "SEK", amount: 1000 });
			runtime.assert.equal(o.OrderId, 777, "create parsed");
			runtime.assert.ok(verified, "Svea token recomputes from the received body+secret+timestamp");
			runtime.assert.ok(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(sawTimestamp), "timestamp format YYYY-MM-DD HH:MM:SS");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("svea signer roundtrip: %v", err)
	}
}

func TestPaymentProviders_QliroSignerRoundtrip(t *testing.T) {
	eng := newPPEngine(t)
	// The client sends createOrder({"OrderId":0}) → serialized body is
	// {"OrderId":0,"MerchantApiKey":"K"} — the client spreads the caller's order
	// then injects MerchantApiKey last (key order = insertion order).
	const secret = "apipw_123"
	body := `{"OrderId":0,"MerchantApiKey":"K"}`
	sum := sha256.Sum256([]byte(body + secret))
	wantAuth := "Qliro " + base64.StdEncoding.EncodeToString(sum[:])
	script := `
		import { qlirov2 } from "paymentproviders";
		let sawAuth = "", sawBody = "";
		const srv = await server.http.listen({ port: 38274, routes: {
			"POST /checkout/merchantapi/Orders": (q: any, r: any) => { sawAuth = q.headers["authorization"][0]; sawBody = q.body; return r.json({ OrderId: 999 }); },
		}});
		try {
			const api = qlirov2.client({ apiKey: "K", apiPassword: "` + secret + `", baseUrl: "http://127.0.0.1:38274" });
			const o = await api.createOrder({ OrderId: 0 });
			runtime.assert.equal(o.OrderId, 999, "create parsed");
			runtime.assert.equal(sawBody, ` + "`" + body + "`" + `, "serialized body matches expectation");
			runtime.assert.equal(sawAuth, "` + wantAuth + `", "Qliro token equals independent SHA256-base64 vector");
		} finally { await srv.close(); }
	`
	if _, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), script); err != nil {
		t.Fatalf("qliro signer roundtrip: %v", err)
	}
}

func TestPaymentProviders_SwedbankPayHALRoundtrip(t *testing.T) {
	eng := newPPEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { swedbankpayv3 } from "paymentproviders";
		const base = "http://127.0.0.1:38278";
		let createAuth = "", capPath = "", capAuth = "", capBody = "";
		const srv = await server.http.listen({ port: 38278, routes: {
			"POST /psp/paymentorders": (q: any, r: any) => {
				createAuth = q.headers["authorization"][0];
				return r.status(201).json({ paymentOrder: { id: "/psp/paymentorders/abc", operations: [
					{ rel: "view-paymentorder", href: base + "/psp/paymentorders/abc", method: "GET" },
					{ rel: "create-paymentorder-capture", href: base + "/psp/paymentorders/abc/captures", method: "POST" }
				]}});
			},
			"POST /psp/paymentorders/abc/captures": (q: any, r: any) => {
				capPath = q.path; capAuth = q.headers["authorization"][0]; capBody = q.body;
				return r.status(201).json({ capture: { id: "cap1" } });
			},
		}});
		try {
			const api = swedbankpayv3.client({ accessToken: "tok123", merchantId: "m1", baseUrl: base });
			const po = await api.createPaymentOrder({ paymentorder: { amount: 1000 } });
			runtime.assert.equal(createAuth, "Bearer tok123", "bearer on create");

			const cap = await api.capturePayment(po, { transaction: { amount: 1000 } });
			runtime.assert.equal(cap.capture.id, "cap1", "capture parsed");
			runtime.assert.equal(capAuth, "Bearer tok123", "bearer on capture");
			runtime.assert.ok(capPath.indexOf("/psp/paymentorders/abc/captures") >= 0, "POSTed to the operation href");
			runtime.assert.equal(JSON.parse(capBody).transaction.amount, 1000, "capture body forwarded");

			let threw = false;
			try { await api.operation(po, "no-such-op", {}); } catch (e) { threw = String(e).includes("not available"); }
			runtime.assert.ok(threw, "missing operation throws");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("swedbankpay HAL roundtrip: %v", err)
	}
}

// TestPaymentProviders_DoesNotShadowUserFile ensures the loader only serves the
// node_modules-resolved bare import, never a user's relative file that happens
// to be named paymentproviders.ts.
func TestPaymentProviders_DoesNotShadowUserFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "paymentproviders.ts"),
		[]byte(`export const kcov3 = "USER_FILE";`), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := scriptengine.Options{ScriptRoot: dir, Timeout: 10 * time.Second}
	opts.ModuleLoader = paymentprovidersLoader(opts.ModuleLoader)
	eng := scriptengine.New(opts)
	if err := registerSurface(eng); err != nil {
		t.Fatalf("registerSurface: %v", err)
	}
	_, err := eng.Run(context.Background(), filepath.Join(dir, "main.ts"), `
		import { kcov3 } from "./paymentproviders";
		if (kcov3 !== "USER_FILE") throw new Error("user file was shadowed: " + JSON.stringify(kcov3));
	`)
	if err != nil {
		t.Fatalf("relative import must resolve to the user's file, not the bundle: %v", err)
	}
}
