package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runSQLiteScript wires the sqlite namespace + a __capture side
// channel and runs the script body. Unlike the sync-binding
// harnesses, sqlite methods return Promises, so test bodies use
// async/await freely; the engine's event loop drains before Run
// returns. The captured value (whatever the script passes to
// __capture) is returned for assertions.
func runSQLiteScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot: t.TempDir(),
		Timeout:    10 * time.Second,
	})
	if err := eng.RegisterNamespaceFactory("sqlite", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return sqliteNamespace(vm, loop)
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatalf("register __capture: %v", err)
	}
	if _, err := eng.Run(context.Background(), "sql.ts", body); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// In-memory open + DDL + insert + query round-trip. The whole
// lifecycle in one script.
func TestSQLite_InMemoryRoundTrip(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)");
		const ins = await db.exec("INSERT INTO t (name) VALUES (?)", "alice");
		const rows = await db.query("SELECT id, name FROM t");
		await db.close();
		__capture([ins.lastInsertId, ins.rowsAffected, rows.length, rows[0].name].join(","));
	`)
	if got != "1,1,1,alice" {
		t.Errorf("round-trip: %v", got)
	}
}

// exec reports rowsAffected for UPDATE / DELETE and lastInsertId
// for INSERT. CREATE TABLE reports zero for both (no rows touched).
func TestSQLite_ExecCounters(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		const ddl = await db.exec("CREATE TABLE t (id INTEGER PRIMARY KEY, n INTEGER)");
		await db.exec("INSERT INTO t (n) VALUES (1), (2), (3)");
		const upd = await db.exec("UPDATE t SET n = n + 10 WHERE n >= 2");
		const del = await db.exec("DELETE FROM t WHERE n > 11");
		await db.close();
		__capture([ddl.rowsAffected, upd.rowsAffected, del.rowsAffected].join(","));
	`)
	// DDL: 0 rows. UPDATE matches n=2,3 → 2 rows. After +10 they're
	// 12,13; DELETE n>11 removes both → 2 rows.
	if got != "0,2,2" {
		t.Errorf("counters: %v", got)
	}
}

// query returns one object per row, keyed by column name, in the
// SQL-specified order.
func TestSQLite_QueryRowsOrdered(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (id INTEGER PRIMARY KEY, age INTEGER)");
		await db.exec("INSERT INTO t (age) VALUES (30), (27), (45)");
		const rows = await db.query("SELECT age FROM t ORDER BY age");
		await db.close();
		__capture(rows.map(r => r.age).join(","));
	`)
	if got != "27,30,45" {
		t.Errorf("ordered query: %v", got)
	}
}

// queryValue returns the first column of the first row, or null on
// no match.
func TestSQLite_QueryValue(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)");
		await db.exec("INSERT INTO t (name) VALUES ('alice'), ('bob')");
		const count = await db.queryValue("SELECT count(*) FROM t");
		const name  = await db.queryValue("SELECT name FROM t WHERE id = ?", 2);
		const none  = await db.queryValue("SELECT name FROM t WHERE id = ?", 999);
		await db.close();
		__capture([count, name, none === null].join(","));
	`)
	if got != "2,bob,true" {
		t.Errorf("queryValue: %v", got)
	}
}

// Each common parameter type binds correctly: string, integer,
// float, null. (BLOB has its own test below.)
func TestSQLite_ParameterTypes(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (s TEXT, i INTEGER, f REAL, n TEXT)");
		await db.exec("INSERT INTO t (s, i, f, n) VALUES (?, ?, ?, ?)",
			"hello", 42, 3.14, null);
		const row = (await db.query("SELECT s, i, f, n FROM t"))[0];
		await db.close();
		__capture([row.s, row.i, row.f, row.n === null].join(","));
	`)
	if got != "hello,42,3.14,true" {
		t.Errorf("param types: %v", got)
	}
}

// BLOB columns round-trip as bytes (Uint8Array), not coerced to
// string — binary payloads that aren't valid UTF-8 stay binary.
func TestSQLite_BlobRoundTrip(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (data BLOB)");
		await db.exec("INSERT INTO t (data) VALUES (?)", new Uint8Array([0, 1, 2, 255, 128]));
		const blob = await db.queryValue("SELECT data FROM t");
		await db.close();
		__capture(Array.from(blob).join(","));
	`)
	if got != "0,1,2,255,128" {
		t.Errorf("blob round-trip: %v", got)
	}
}

// File-backed database persists across handles: write with one
// handle, close, reopen, read it back.
func TestSQLite_FileBackedPersists(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	got := runSQLiteScriptWithVar(t, "__dbpath", dbPath, `
		const db1 = await sqlite.open(__dbpath);
		await db1.exec("CREATE TABLE t (v TEXT)");
		await db1.exec("INSERT INTO t (v) VALUES ('persisted')");
		await db1.close();

		const db2 = await sqlite.open(__dbpath);
		const v = await db2.queryValue("SELECT v FROM t");
		await db2.close();
		__capture(v);
	`)
	if got != "persisted" {
		t.Errorf("file persistence: %v", got)
	}
}

// Invalid SQL throws with sercon's binding-named prefix rather than
// the raw driver error.
func TestSQLite_InvalidSQLThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("sqlite", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return sqliteNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `
		const db = await sqlite.open(":memory:");
		await db.query("SELECT * FROM nonexistent_table");
	`)
	if err == nil {
		t.Fatal("expected throw for invalid SQL")
	}
	if !strings.Contains(err.Error(), "sqlite.query") {
		t.Errorf("expected sqlite.query: prefix; got %v", err)
	}
}

// Empty path throws with the :memory: hint.
func TestSQLite_EmptyPathThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("sqlite", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return sqliteNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `await sqlite.open("");`)
	if err == nil {
		t.Fatal("expected throw for empty path")
	}
	if !strings.Contains(err.Error(), ":memory:") {
		t.Errorf("expected :memory: hint; got %v", err)
	}
}

// Using a handle after close() surfaces an error (the driver
// reports the closed connection).
func TestSQLite_UseAfterCloseErrors(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("sqlite", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return sqliteNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `
		const db = await sqlite.open(":memory:");
		await db.close();
		await db.exec("CREATE TABLE t (x INTEGER)");
	`)
	if err == nil {
		t.Fatal("expected throw for use-after-close")
	}
	if !strings.Contains(err.Error(), "sqlite.exec") {
		t.Errorf("expected sqlite.exec: prefix; got %v", err)
	}
}

// runSQLiteScriptWithVar is runSQLiteScript plus one extra
// registered global so a test can hand a value (here: a temp file
// path) to the script without string-concatenating it into the
// source.
func runSQLiteScriptWithVar(t *testing.T, varName, varVal, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("sqlite", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return sqliteNamespace(vm, loop)
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := eng.Register(varName, varVal); err != nil {
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
	if _, err := eng.Run(context.Background(), "sql.ts", body); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// Transaction commit makes inserts visible to the outer handle.
func TestSQLite_TxCommit(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (v TEXT)");
		const tx = await db.begin();
		await tx.exec("INSERT INTO t (v) VALUES ('a'), ('b')");
		const insideCount = await tx.queryValue("SELECT count(*) FROM t");
		await tx.commit();
		const outsideCount = await db.queryValue("SELECT count(*) FROM t");
		await db.close();
		__capture([insideCount, outsideCount].join(","));
	`)
	if got != "2,2" {
		t.Errorf("tx commit: %v (want 2,2)", got)
	}
}

// Transaction rollback discards inserts — the outer handle never
// sees them.
func TestSQLite_TxRollback(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (v TEXT)");
		await db.exec("INSERT INTO t (v) VALUES ('seed')");
		const tx = await db.begin();
		await tx.exec("INSERT INTO t (v) VALUES ('discarded')");
		const insideCount = await tx.queryValue("SELECT count(*) FROM t");
		await tx.rollback();
		const outsideCount = await db.queryValue("SELECT count(*) FROM t");
		await db.close();
		__capture([insideCount, outsideCount].join(","));
	`)
	// Inside the tx: seed + discarded = 2. After rollback: just seed = 1.
	if got != "2,1" {
		t.Errorf("tx rollback: %v (want 2,1)", got)
	}
}

// tx.query returns rows the same shape as the handle's query.
func TestSQLite_TxQuery(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)");
		const tx = await db.begin();
		await tx.exec("INSERT INTO t (name) VALUES ('alice'), ('bob')");
		const rows = await tx.query("SELECT name FROM t ORDER BY name");
		await tx.commit();
		await db.close();
		__capture(rows.map(r => r.name).join(","));
	`)
	if got != "alice,bob" {
		t.Errorf("tx query: %v", got)
	}
}

// A constraint violation inside a transaction throws (with the
// sqlite.tx.exec prefix); the script can then roll back cleanly and
// the seed row is preserved.
func TestSQLite_TxConstraintThenRollback(t *testing.T) {
	got := runSQLiteScript(t, `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (v TEXT UNIQUE)");
		await db.exec("INSERT INTO t (v) VALUES ('dup')");
		const tx = await db.begin();
		let caught = "";
		try {
			await tx.exec("INSERT INTO t (v) VALUES ('dup')");  // UNIQUE violation
		} catch (e) {
			caught = String(e);
		}
		await tx.rollback();
		const count = await db.queryValue("SELECT count(*) FROM t");
		await db.close();
		__capture([caught.includes("sqlite.tx.exec"), count].join(","));
	`)
	if got != "true,1" {
		t.Errorf("tx constraint+rollback: %v (want true,1)", got)
	}
}

// Using a transaction after commit throws with sql.ErrTxDone surfaced
// through the sqlite.tx.* prefix.
func TestSQLite_TxUseAfterCommitThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("sqlite", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return sqliteNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (v TEXT)");
		const tx = await db.begin();
		await tx.exec("INSERT INTO t (v) VALUES ('x')");
		await tx.commit();
		await tx.exec("INSERT INTO t (v) VALUES ('y')");  // tx is spent
	`)
	if err == nil {
		t.Fatal("expected throw for use-after-commit")
	}
	if !strings.Contains(err.Error(), "sqlite.tx.exec") {
		t.Errorf("expected sqlite.tx.exec: prefix; got %v", err)
	}
}

// Double commit throws (sql.ErrTxDone via sqlite.tx.commit).
func TestSQLite_TxDoubleCommitThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("sqlite", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return sqliteNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `
		const db = await sqlite.open(":memory:");
		await db.exec("CREATE TABLE t (v TEXT)");
		const tx = await db.begin();
		await tx.exec("INSERT INTO t (v) VALUES ('x')");
		await tx.commit();
		await tx.commit();
	`)
	if err == nil {
		t.Fatal("expected throw for double commit")
	}
	if !strings.Contains(err.Error(), "sqlite.tx.commit") {
		t.Errorf("expected sqlite.tx.commit: prefix; got %v", err)
	}
}
