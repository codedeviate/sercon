// Demonstrates paymentproviders: sveacheckout2 (Svea Checkout). Offline mock
// verifies the SHA512 signature; live check self-skips without SCO_* env.
import { sveacheckout2 } from "paymentproviders";

const srv = await server.http.listen({ port: 38276, routes: {
  "POST /api/orders": (q: any, r: any) => {
    const ts = q.headers["timestamp"][0];
    const expect = "Svea " + text.str.base64Encode("merch1:" + crypto.hash.sha512(q.body + "shh" + ts).toUpperCase());
    if (q.headers["authorization"][0] !== expect) return r.status(401).json({ error: "bad sig" });
    return r.status(201).json({ OrderId: 555 });
  },
}});
try {
  const mock = sveacheckout2.client({ merchantId: "merch1", secretKey: "shh", baseUrl: "http://127.0.0.1:38276" });
  const o = await mock.createOrder({ Currency: "SEK", Cart: { Items: [] } });
  runtime.assert.equal(o.OrderId, 555, "mock create (signature verified by mock)");
  runtime.log("paymentproviders sveacheckout2 (mock) OK:", o.OrderId);
} finally { await srv.close(); }

if (runtime.env.get("SCO_MERCHANT_ID") && runtime.env.get("SCO_SECRET_KEY")) {
  const api = sveacheckout2.client();
  try {
    await api.getOrder("0");
  } catch (e: any) {
    runtime.assert.ok(e.status !== 401, "sveacheckout2 live credentials accepted (status " + e.status + ", not 401)");
    runtime.log("paymentproviders sveacheckout2 (live) auth OK (status " + e.status + ")");
  }
} else {
  runtime.log("paymentproviders sveacheckout2: no SCO_* env — skipping live check (mock passed).");
}
