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
	// MariaDB speaks the MySQL wire protocol; db.mysql.open accepts MariaDB DSNs
	// (there is no separate db.mariadb binding).
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

func TestIntegration_Redis(t *testing.T) {
	url := os.Getenv("SERCON_TEST_REDIS_URL")
	if url == "" {
		t.Skip("SERCON_TEST_REDIS_URL not set")
	}
	got := runDBIntegrationScript(t, `
		const r = await db.redis.open(`+strconv.Quote(url)+`);
		const pong = await r.ping();
		await r.close();
		const __result = pong;
	`)
	if got != "PONG" {
		t.Errorf("ping = %v, want PONG", got)
	}
}

func TestIntegration_Valkey(t *testing.T) {
	url := os.Getenv("SERCON_TEST_VALKEY_URL")
	if url == "" {
		t.Skip("SERCON_TEST_VALKEY_URL not set")
	}
	got := runDBIntegrationScript(t, `
		const r = await db.valkey.open(`+strconv.Quote(url)+`);
		const pong = await r.ping();
		await r.close();
		const __result = pong;
	`)
	if got != "PONG" {
		t.Errorf("ping = %v, want PONG", got)
	}
}

func TestIntegration_Memcached(t *testing.T) {
	addr := os.Getenv("SERCON_TEST_MEMCACHED_ADDR")
	if addr == "" {
		t.Skip("SERCON_TEST_MEMCACHED_ADDR not set")
	}
	got := runDBIntegrationScript(t, `
		const mc = await db.memcached.open(`+strconv.Quote(addr)+`);
		await mc.set("sercon:integration", "ok");
		const __result = await mc.get("sercon:integration");
	`)
	if got != "ok" {
		t.Errorf("memcached round-trip = %v, want ok", got)
	}
}

func TestIntegration_LDAP(t *testing.T) {
	url := os.Getenv("SERCON_TEST_LDAP_URL")
	if url == "" {
		t.Skip("SERCON_TEST_LDAP_URL not set")
	}
	bindDN := envOr("SERCON_TEST_LDAP_BINDDN", "cn=admin,dc=example,dc=org")
	password := envOr("SERCON_TEST_LDAP_PASSWORD", "adminpw")
	base := envOr("SERCON_TEST_LDAP_BASE", "dc=example,dc=org")
	got := runDBIntegrationScript(t, `
		const l = await db.ldap.open(`+strconv.Quote(url)+`, { bindDN: `+strconv.Quote(bindDN)+`, password: `+strconv.Quote(password)+` });
		const entries = await l.search(`+strconv.Quote(base)+`, "(uid=alice)");
		await l.close();
		const __result = String(Array.isArray(entries) ? entries.length : -1);
	`)
	s, _ := got.(string)
	if n, _ := strconv.Atoi(s); n < 1 {
		t.Errorf("ldap search returned %q entries, want >= 1", s)
	}
}
