// Demonstrates the bundled paymentproviders library: swedbankpayv3 (and v2).
// SwedbankPay is HAL/hypermedia — capture/refund/cancel POST to the operation
// href found in the payment-order response. Offline mock always runs; the live
// check self-skips without SWEDBANKPAY_* env (set via `sercon --env-file .env`).
import { swedbankpayv3 } from "paymentproviders";

const base = "http://127.0.0.1:38279";
const srv = await server.http.listen({ port: 38279, routes: {
  "POST /psp/paymentorders": (q: any, r: any) => r.status(201).json({ paymentOrder: {
    id: "/psp/paymentorders/demo",
    operations: [{ rel: "create-paymentorder-capture", href: base + "/psp/paymentorders/demo/captures", method: "POST" }],
  }}),
  "POST /psp/paymentorders/demo/captures": (q: any, r: any) => r.status(201).json({ capture: { id: "cap_demo", state: "Completed" } }),
}});
try {
  const mock = swedbankpayv3.client({ accessToken: "demo-token", merchantId: "demo-payee", baseUrl: base });
  const po = await mock.createPaymentOrder({ paymentorder: { operation: "Purchase", amount: 1500 } });
  const cap = await mock.capturePayment(po, { transaction: { amount: 1500, description: "demo" } });
  runtime.assert.equal(cap.capture.id, "cap_demo", "mock capture via HAL operation");
  runtime.log("paymentproviders swedbankpayv3 (mock) OK: captured", cap.capture.id);
} finally { await srv.close(); }

if (runtime.env.get("SWEDBANKPAY_ACCESS_TOKEN") && runtime.env.get("SWEDBANKPAY_MERCHANT_ID")) {
  const api = swedbankpayv3.client();
  try {
    await api.getPaymentOrder("/psp/paymentorders/00000000-0000-0000-0000-000000000000");
  } catch (e: any) {
    runtime.assert.ok(e.status !== 401, "swedbankpay live credentials accepted (status " + e.status + ", not 401)");
    runtime.log("paymentproviders swedbankpayv3 (live) auth OK (status " + e.status + ")");
  }
} else {
  runtime.log("paymentproviders swedbankpay: no SWEDBANKPAY_* env — skipping live check (mock passed).");
}
