// Demonstrates the bundled `paymentproviders` library (KCO v3). Runs fully
// offline against a local mock that emulates the Kustom Order-Management API,
// proving the request/auth/parse path. If KCO_MERCHANT_ID + KCO_SHARED_SECRET
// are set (e.g. via `sercon --env-file .env`), it ALSO does a real getPayment
// against the configured environment to prove the live credential pairing.
import { kcov3 } from "paymentproviders";

// --- Offline mock round-trip (always runs) ---
const srv = await server.http.listen({ port: 38271, routes: {
  "GET /ordermanagement/v1/orders/ord_demo": (q: any, r: any) =>
    r.json({ order_id: "ord_demo", status: "AUTHORIZED", order_amount: 1500, captured_amount: 0 }),
  "POST /ordermanagement/v1/orders/ord_demo/captures": (q: any, r: any) =>
    r.status(201).json({ capture_id: "cap_demo", captured_amount: 1500 }),
}});
try {
  const mock = kcov3.client({ merchantId: "demo", sharedSecret: "secret", baseUrl: "http://127.0.0.1:38271" });
  const order = await mock.getPayment("ord_demo");
  runtime.assert.equal(order.status, "AUTHORIZED", "mock getPayment");
  const cap = await mock.capturePayment("ord_demo", { amount: order.order_amount });
  runtime.assert.equal(cap.captured_amount, 1500, "mock capture");
  runtime.log("paymentproviders kcov3 (mock) OK:", order.order_id, "captured", cap.captured_amount);
} finally {
  await srv.close();
}

// --- Live (credentials-gated; self-skips without env) ---
if (runtime.env.get("KCO_MERCHANT_ID") && runtime.env.get("KCO_SHARED_SECRET")) {
  const api = kcov3.client(); // env-driven (KCO_ENV defaults to test)
  const id = runtime.env.get("KCO_TEST_ORDER_ID");
  if (id) {
    const order = await api.getPayment(id);
    runtime.log("paymentproviders kcov3 (live) order:", order.order_id ?? id, order.status);
  } else {
    // No order id: prove auth is accepted — a bogus id should 404 (auth OK),
    // not 401 (auth rejected).
    try {
      await api.getPayment("does-not-exist");
    } catch (e: any) {
      runtime.assert.ok(e.status !== 401, "live credentials accepted (got " + e.status + ", not 401)");
      runtime.log("paymentproviders kcov3 (live) auth OK (status " + e.status + " for bogus id)");
    }
  }
} else {
  runtime.log("paymentproviders kcov3: no KCO_* env — skipping live check (mock passed).");
}
