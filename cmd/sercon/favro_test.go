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
