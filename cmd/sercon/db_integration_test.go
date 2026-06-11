package main

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/dop251/goja"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runDBIntegrationScript registers the full reserved-global surface (so db.* is
// present), registers __capture, runs body, and returns the captured __result.
// A script error fails the test — this is the "env var set but the server is
// unreachable / the query failed" path. Modeled on runSocketScript.
func runDBIntegrationScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 30 * time.Second})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "it.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("integration script failed: %v", err)
	}
	return captured
}

// envOr returns the env var value, or def when it is unset/empty. Used for the
// LDAP bind/base companions so exporting only SERCON_TEST_LDAP_URL still works
// against a stock dbplayground fleet.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func TestIntegration_Postgres(t *testing.T) {
	dsn := os.Getenv("SERCON_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("SERCON_TEST_PG_DSN not set")
	}
	got := runDBIntegrationScript(t, `
		const h = await db.postgres.open(`+strconv.Quote(dsn)+`);
		const n = await h.queryValue("SELECT count(*) FROM orders");
		await h.close();
		const __result = String(Number(n));
	`)
	if got != "4" {
		t.Errorf("orders count = %v, want 4", got)
	}
}

func TestIntegration_MySQL(t *testing.T) {
	dsn := os.Getenv("SERCON_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SERCON_TEST_MYSQL_DSN not set")
	}
	got := runDBIntegrationScript(t, `
		const h = await db.mysql.open(`+strconv.Quote(dsn)+`);
		const n = await h.queryValue("SELECT count(*) FROM orders");
		await h.close();
		const __result = String(Number(n));
	`)
	if got != "4" {
		t.Errorf("orders count = %v, want 4", got)
	}
}

func TestIntegration_MariaDB(t *testing.T) {
	dsn := os.Getenv("SERCON_TEST_MARIADB_DSN")
	if dsn == "" {
		t.Skip("SERCON_TEST_MARIADB_DSN not set")
	}
	got := runDBIntegrationScript(t, `
		const h = await db.mysql.open(`+strconv.Quote(dsn)+`);
		const n = await h.queryValue("SELECT count(*) FROM orders");
		await h.close();
		const __result = String(Number(n));
	`)
	if got != "4" {
		t.Errorf("orders count = %v, want 4", got)
	}
}

func TestIntegration_ClickHouse(t *testing.T) {
	dsn := os.Getenv("SERCON_TEST_CLICKHOUSE_DSN")
	if dsn == "" {
		t.Skip("SERCON_TEST_CLICKHOUSE_DSN not set")
	}
	got := runDBIntegrationScript(t, `
		const h = await db.clickhouse.open(`+strconv.Quote(dsn)+`);
		const n = await h.queryValue("SELECT count(*) FROM orders");
		await h.close();
		const __result = String(Number(n));
	`)
	if got != "4" {
		t.Errorf("orders count = %v, want 4", got)
	}
}
