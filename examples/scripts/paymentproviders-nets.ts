// Demonstrates the bundled paymentproviders library: netsv1 (Nexi/Nets Checkout
// Payment API v1). Offline mock always runs; live check self-skips without
// NETS_SECRET_KEY (set via `sercon --env-file .env`).
import { netsv1 } from "paymentproviders";

const srv = await server.http.listen({ port: 38275, routes: {
  "POST /v1/payments": (q: any, r: any) => r.status(201).json({ paymentId: "pay_demo" }),
  "GET /v1/payments/pay_demo": (q: any, r: any) => r.json({ payment: { paymentId: "pay_demo", summary: { reservedAmount: 1000 } } }),
}});
try {
  const mock = netsv1.client({ secretKey: "demo-secret", baseUrl: "http://127.0.0.1:38275" });
  const created = await mock.createPayment({ order: { amount: 1000, currency: "SEK" } });
  runtime.assert.equal(created.paymentId, "pay_demo", "mock create");
  const got = await mock.getPayment("pay_demo");
  runtime.assert.equal(got.payment.paymentId, "pay_demo", "mock get");
  runtime.log("paymentproviders netsv1 (mock) OK:", created.paymentId);
} finally { await srv.close(); }

if (runtime.env.get("NETS_SECRET_KEY")) {
  const api = netsv1.client();
  try {
    await api.getPayment("00000000000000000000000000000000");
  } catch (e: any) {
    runtime.assert.ok(e.status !== 401, "netsv1 live credentials accepted (status " + e.status + ", not 401)");
    runtime.log("paymentproviders netsv1 (live) auth OK (status " + e.status + ")");
  }
} else {
  runtime.log("paymentproviders netsv1: no NETS_SECRET_KEY — skipping live check (mock passed).");
}
