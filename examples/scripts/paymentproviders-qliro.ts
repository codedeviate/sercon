// Demonstrates paymentproviders: qlirov2 (Qliro One). Offline mock checks the
// Qliro signature header shape; live check self-skips without QLIRO_* env.
import { qlirov2 } from "paymentproviders";

const srv = await server.http.listen({ port: 38277, routes: {
  "POST /checkout/merchantapi/Orders": (q: any, r: any) => {
    if (!/^Qliro .+/.test(q.headers["authorization"][0])) return r.status(401).json({ error: "bad auth" });
    return r.json({ OrderId: 321 });
  },
}});
try {
  const mock = qlirov2.client({ apiKey: "K", apiPassword: "p", baseUrl: "http://127.0.0.1:38277" });
  const o = await mock.createOrder({ Currency: "SEK", OrderItems: [] });
  runtime.assert.equal(o.OrderId, 321, "mock create (Qliro auth header present)");
  runtime.log("paymentproviders qlirov2 (mock) OK:", o.OrderId);
} finally { await srv.close(); }

if (runtime.env.get("QLIRO_API_KEY") && runtime.env.get("QLIRO_APIPASSWORD")) {
  const api = qlirov2.client();
  try {
    await api.getOrder(0);
  } catch (e: any) {
    runtime.assert.ok(e.status !== 401, "qlirov2 live credentials accepted (status " + e.status + ", not 401)");
    runtime.log("paymentproviders qlirov2 (live) auth OK (status " + e.status + ")");
  }
} else {
  runtime.log("paymentproviders qlirov2: no QLIRO_* env — skipping live check (mock passed).");
}
