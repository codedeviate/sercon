package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// newFavroEngine builds an engine with the favro loader + full CLI surface.
func newFavroEngine(t *testing.T) *scriptengine.Engine {
	t.Helper()
	opts := scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second}
	opts.ModuleLoader = favroLoader(opts.ModuleLoader)
	eng := scriptengine.New(opts)
	if err := registerSurface(eng); err != nil {
		t.Fatalf("registerSurface: %v", err)
	}
	return eng
}

func TestFavro_ImportResolves(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		const c = client({ email: "e@x.com", apiToken: "tok", organizationId: "org1" });
		if (typeof c.cards.get !== "function") throw new Error("no cards.get");
		if (typeof c.organizations.get !== "function") throw new Error("no organizations.get");
	`)
	if err != nil {
		t.Fatalf("import favro: %v", err)
	}
}

func TestFavro_MissingCredsThrows(t *testing.T) {
	t.Setenv("FAVRO_EMAIL", "")
	t.Setenv("FAVRO_API_TOKEN", "")
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		let threw = false;
		try { client({ organizationId: "o" }); } catch (e) { threw = true; if (!String(e).includes("FAVRO_EMAIL")) throw e; }
		if (!threw) throw new Error("expected missing-cred throw");
	`)
	if err != nil {
		t.Fatalf("missing-creds: %v", err)
	}
}

func TestFavro_AuthAndOrgHeader(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		const seen: any = {};
		const srv = await server.http.listen({ port: 38310, routes: {
			"GET /cards/card_1": (q: any, r: any) => {
				seen.cardAuth = q.headers["authorization"][0];
				seen.cardOrg = (q.headers["organizationid"] || [])[0];
				return r.json({ cardId: "card_1", name: "hello" });
			},
			"GET /organizations/org_1": (q: any, r: any) => {
				seen.orgAuth = q.headers["authorization"][0];
				seen.orgOrg = (q.headers["organizationid"] || [])[0];
				return r.json({ organizationId: "org_1", name: "Acme" });
			},
		}});
		try {
			const c = client({ email: "e@x.com", apiToken: "tok", organizationId: "org_1", baseUrl: "http://127.0.0.1:38310" });
			const card = await c.cards.get("card_1");
			runtime.assert.equal(card.name, "hello", "card parsed");
			// base64("e@x.com:tok")
			runtime.assert.equal(seen.cardAuth, "Basic " + text.str.base64Encode("e@x.com:tok"), "card basic auth");
			runtime.assert.equal(seen.cardOrg, "org_1", "org header on org-scoped route");
			const org = await c.organizations.get("org_1");
			runtime.assert.equal(org.name, "Acme", "org parsed");
			runtime.assert.ok(!seen.orgOrg, "NO org header on user-level organizations route");
		} finally {
			await srv.close();
		}
	`)
	if err != nil {
		t.Fatalf("auth/org header: %v", err)
	}
}

// TestFavro_DoesNotShadowUserFile: the loader must only serve the bare import,
// never a user's relative ./favro.ts.
func TestFavro_DoesNotShadowUserFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "favro.ts"),
		[]byte(`export const client = "USER_FILE";`), 0o644); err != nil {
		t.Fatal(err)
	}
	opts := scriptengine.Options{ScriptRoot: dir, Timeout: 10 * time.Second}
	opts.ModuleLoader = favroLoader(opts.ModuleLoader)
	eng := scriptengine.New(opts)
	if err := registerSurface(eng); err != nil {
		t.Fatalf("registerSurface: %v", err)
	}
	_, err := eng.Run(context.Background(), filepath.Join(dir, "main.ts"), `
		import { client } from "./favro";
		if (client !== "USER_FILE") throw new Error("user file was shadowed: " + JSON.stringify(client));
	`)
	if err != nil {
		t.Fatalf("relative import must resolve to the user's file, not the bundle: %v", err)
	}
}

func TestFavro_NonOkThrowsFavroError(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		const srv = await server.http.listen({ port: 38318, routes: {
			"GET /cards/missing": (q: any, r: any) => r.status(404).json({ message: "not found", requestId: "rq-404" }),
		}});
		try {
			const c = client({ email: "e@x.com", apiToken: "t", organizationId: "o", baseUrl: "http://127.0.0.1:38318" });
			let e: any = null;
			try { await c.cards.get("missing"); } catch (err) { e = err; }
			runtime.assert.ok(e, "threw on 404");
			runtime.assert.equal(e.name, "FavroError", "FavroError name");
			runtime.assert.equal(e.status, 404, "FavroError status 404");
			runtime.assert.equal(e.body.message, "not found", "FavroError body carries parsed json");
			runtime.assert.equal(e.requestId, "rq-404", "FavroError requestId from body");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("non-ok FavroError: %v", err)
	}
}

func TestFavro_RetriesOn429(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		let calls = 0;
		const srv = await server.http.listen({ port: 38311, routes: {
			"GET /cards/c1": (q: any, r: any) => {
				calls++;
				if (calls === 1) return r.header("retry-after", "0").status(429).json({ message: "slow down" });
				return r.json({ cardId: "c1", name: "ok" });
			},
		}});
		try {
			const c = client({ email: "e@x.com", apiToken: "t", organizationId: "o", baseUrl: "http://127.0.0.1:38311" });
			const card = await c.cards.get("c1");
			runtime.assert.equal(card.name, "ok", "second attempt succeeded");
			runtime.assert.equal(calls, 2, "retried exactly once");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("429 retry: %v", err)
	}
}

func TestFavro_RetryFalseThrows429(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		let calls = 0;
		const srv = await server.http.listen({ port: 38312, routes: {
			"GET /cards/c1": (q: any, r: any) => { calls++; return r.header("x-ratelimit-remaining", "0").status(429).json({ message: "no" }); },
		}});
		try {
			const c = client({ email: "e@x.com", apiToken: "t", organizationId: "o", baseUrl: "http://127.0.0.1:38312", retry: false });
			let err: any = null;
			try { await c.cards.get("c1"); } catch (e) { err = e; }
			runtime.assert.ok(err, "threw");
			runtime.assert.equal(err.status, 429, "FavroError status 429");
			runtime.assert.equal(err.name, "FavroError", "FavroError name");
			runtime.assert.equal(err.rateLimit.remaining, 0, "rateLimit.remaining parsed");
			runtime.assert.equal(calls, 1, "no retry when retry:false");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("retry:false 429: %v", err)
	}
}

func TestFavro_Pagination(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		const seen: any[] = [];
		const srv = await server.http.listen({ port: 38313, routes: {
			"GET /collections": (q: any, r: any) => {
				seen.push({ page: (q.query.page || [])[0], requestId: (q.query.requestId || [])[0], backend: (q.headers["x-favro-backend-identifier"] || [])[0] });
				const page = Number((q.query.page || ["0"])[0]);
				if (page === 0) {
					return r.header("X-Favro-Backend-Identifier", "backend-9").json({ limit: 100, page: 0, pages: 2, requestId: "req-1", entities: [{ collectionId: "a" }] });
				}
				return r.json({ limit: 100, page: 1, pages: 2, requestId: "req-1", entities: [{ collectionId: "b" }] });
			},
		}});
		try {
			const c = client({ email: "e@x.com", apiToken: "t", organizationId: "o", baseUrl: "http://127.0.0.1:38313" });
			const all = await c.collections.listAll();
			runtime.assert.equal(all.length, 2, "listAll flattened both pages");
			runtime.assert.equal(all[0].collectionId + all[1].collectionId, "ab", "order preserved");
			// second request echoed page + requestId and the backend header
			runtime.assert.equal(seen[1].page, "1", "page=1 echoed");
			runtime.assert.equal(seen[1].requestId, "req-1", "requestId echoed");
			runtime.assert.equal(seen[1].backend, "backend-9", "backend identifier pinned");

			const got: string[] = [];
			for await (const col of c.collections.iterate()) got.push(col.collectionId);
			runtime.assert.equal(got.join(""), "ab", "iterate yielded both pages");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("pagination: %v", err)
	}
}

func TestFavro_ScopedListValidation(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		const c = client({ email: "e@x.com", apiToken: "t", organizationId: "o" });
		async function throws(fn: () => Promise<any>, needle: string) {
			try { await fn(); return false; } catch (e) { return String(e).includes(needle); }
		}
		runtime.assert.ok(await throws(() => c.columns.list({}), "widgetCommonId"), "columns needs widgetCommonId");
		runtime.assert.ok(await throws(() => c.tasks.list({}), "cardCommonId"), "tasks needs cardCommonId");
		runtime.assert.ok(await throws(() => c.tasklists.list({}), "cardCommonId"), "tasklists needs cardCommonId");
	`)
	if err != nil {
		t.Fatalf("scoped list validation: %v", err)
	}
}

func TestFavro_TagsCrudRoundtrip(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		const seen: any[] = [];
		const srv = await server.http.listen({ port: 38314, routes: {
			"POST /tags": (q: any, r: any) => { seen.push({ m: "POST", body: q.body }); return r.status(201).json({ tagId: "t1", name: "urgent" }); },
			"PUT /tags/t1": (q: any, r: any) => { seen.push({ m: "PUT", body: q.body }); return r.json({ tagId: "t1", name: "URGENT" }); },
			"DELETE /tags/t1": (q: any, r: any) => { seen.push({ m: "DELETE" }); return r.status(204).text(""); },
		}});
		try {
			const c = client({ email: "e@x.com", apiToken: "t", organizationId: "o", baseUrl: "http://127.0.0.1:38314" });
			const created = await c.tags.create({ name: "urgent" });
			runtime.assert.equal(created.tagId, "t1", "create parsed");
			runtime.assert.equal(JSON.parse(seen[0].body).name, "urgent", "create body sent");
			const updated = await c.tags.update("t1", { name: "URGENT" });
			runtime.assert.equal(updated.name, "URGENT", "update parsed");
			await c.tags.remove("t1");
			runtime.assert.equal(seen[2].m, "DELETE", "remove issued DELETE");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("tags crud: %v", err)
	}
}

func TestFavro_OrganizationsListOmitsOrgHeader(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		let sawOrgHeader = true;
		const srv = await server.http.listen({ port: 38319, routes: {
			"GET /organizations": (q: any, r: any) => {
				sawOrgHeader = !!(q.headers["organizationid"] && q.headers["organizationid"].length);
				return r.json({ limit: 100, page: 0, pages: 1, requestId: "r", entities: [{ organizationId: "o1" }] });
			},
		}});
		try {
			const c = client({ email: "e@x.com", apiToken: "t", organizationId: "o1", baseUrl: "http://127.0.0.1:38319" });
			const orgs = await c.organizations.listAll();
			runtime.assert.equal(orgs.length, 1, "organizations listed");
			runtime.assert.ok(!sawOrgHeader, "organizations.listAll must NOT send the organizationId header (user-level)");
		} finally { await srv.close(); }
	`)
	if err != nil {
		t.Fatalf("organizations list org-header: %v", err)
	}
}

func TestFavro_CardsListRequiresScope(t *testing.T) {
	eng := newFavroEngine(t)
	_, err := eng.Run(context.Background(), filepath.Join(t.TempDir(), "main.ts"), `
		import { client } from "favro";
		const c = client({ email: "e@x.com", apiToken: "t", organizationId: "o" });
		let threw = false;
		try { await c.cards.list({}); } catch (e) { threw = String(e).includes("is required"); }
		runtime.assert.ok(threw, "cards.list without a scope throws");
	`)
	if err != nil {
		t.Fatalf("cards scope: %v", err)
	}
}
