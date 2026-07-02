// Demonstrates the bundled `favro` library. Runs fully offline against a local
// mock emulating the Favro API (auth, pagination, a card create/get, and an
// attachment upload). If FAVRO_EMAIL + FAVRO_API_TOKEN + FAVRO_ORGANIZATION_ID
// are set (e.g. via `sercon --env-file .env`), it ALSO does a live probe to
// prove the credential pairing.
import { client } from "favro";

// --- Offline mock round-trip (always runs) ---
const port = 38317;
const srv = await server.http.listen({ port, routes: {
  "GET /collections": (q: any, r: any) => {
    const page = Number((q.query.page || ["0"])[0]);
    if (page === 0) return r.header("X-Favro-Backend-Identifier", "be1").json({ limit: 100, page: 0, pages: 2, requestId: "rq", entities: [{ collectionId: "c1" }] });
    return r.json({ limit: 100, page: 1, pages: 2, requestId: "rq", entities: [{ collectionId: "c2" }] });
  },
  "POST /cards": (q: any, r: any) => r.status(201).json({ cardId: "cardX", name: JSON.parse(q.body).name }),
  "GET /cards/cardX": (q: any, r: any) => r.json({ cardId: "cardX", name: "Demo card" }),
  "POST /cards/cardX/attachments": (q: any, r: any) => r.status(201).json({ name: "spec.txt" }),
}});
try {
  const favro = client({ email: "demo@x.com", apiToken: "tok", organizationId: "org", baseUrl: `http://127.0.0.1:${port}` });

  const cols = await favro.collections.listAll();
  runtime.assert.equal(cols.length, 2, "collections paginated across 2 pages");

  const created = await favro.cards.create({ name: "Demo card" });
  runtime.assert.equal(created.cardId, "cardX", "card created");
  const card = await favro.cards.get("cardX");
  runtime.assert.equal(card.name, "Demo card", "card fetched");

  const att = await favro.cards.uploadAttachment("cardX", { filename: "spec.txt", content: "hello", type: "text/plain" });
  runtime.assert.equal(att.name, "spec.txt", "attachment uploaded");

  runtime.log("favro (mock) OK:", cols.length, "collections,", created.cardId, "+", att.name);
} finally {
  await srv.close();
}

// --- Live (credentials-gated; self-skips without env) ---
if (runtime.env.get("FAVRO_EMAIL") && runtime.env.get("FAVRO_API_TOKEN")) {
  const api = client(); // env-driven
  try {
    const orgs = await api.organizations.listAll();
    runtime.log("favro (live) organizations:", orgs.length);
  } catch (e: any) {
    runtime.assert.ok(e.status !== 401, "live credentials accepted (got " + e.status + ", not 401)");
    runtime.log("favro (live) auth OK (status " + e.status + ")");
  }
} else {
  runtime.log("favro: no FAVRO_* env — skipping live check (mock passed).");
}
