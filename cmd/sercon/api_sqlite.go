package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	_ "modernc.org/sqlite" // register the "sqlite" database/sql driver

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// sqliteNamespace wires `api.sqlite.*`. The namespace only exposes
// `open`; the handle object that `open` returns is where the real
// surface lives (exec / query / queryValue / close). This is the
// first stateful-handle binding sercon ships — the pattern future
// network-protocol bindings (redis / ldap / etc.) will reuse: a
// thin factory function on the namespace that returns a map of
// closure methods, each bound to the resource that `open`
// allocated.
//
// `modernc.org/sqlite` is the pure-Go SQLite — no cgo, so the
// project's cgo-free rule is preserved. The blank import above
// registers it as the database/sql driver named "sqlite".
func sqliteNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqliteOpen(vm, loop, ctx, call)
		}),
	}
}

// sqliteOpen connects to a SQLite database and returns the handle
// object scripts use to drive it. `path` is either ":memory:" for
// an in-RAM database (data evaporates with the handle), or a
// filesystem path. Missing files are created automatically by the
// driver.
//
// We Ping immediately so a misspelled path / permission error
// surfaces at the open() call rather than the first exec(). The
// db.Close() on error is the standard belt-and-suspenders — leaving
// the *sql.DB un-closed would leak the underlying connection until
// GC.
func sqliteOpen(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, errors.New("sqlite.open: path argument required (use \":memory:\" for in-RAM)")
	}
	path := call.Argument(0).String()
	if path == "" {
		return nil, errors.New("sqlite.open: path is empty (use \":memory:\" for in-RAM)")
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("sqlite.open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("sqlite.open: ping: %w", err)
	}
	return sqliteHandle(vm, loop, db), nil
}

// sqliteHandle builds the JS-facing object returned by open(). Each
// method is a PromisifyAsync-wrapped closure over the *sql.DB, so
// the calling script sees Promise-returning functions. Closures
// capture (vm, loop) from the outer namespace factory.
//
// `close` is included because callers can't otherwise release the
// underlying connection. We don't ship a finalizer — scripts that
// forget to close leak a connection until the process exits, but
// adding GC-time cleanup would mask the bug rather than fix it.
// The pattern's documented as "open / use / close" in MANUAL.
func sqliteHandle(vm *goja.Runtime, loop *eventloop.EventLoop, db *sql.DB) map[string]any {
	// PromisifyAsync returns an AsyncBinding carrier that the engine
	// unwraps to a goja-callable at *registration* time. The handle
	// map here is built at script-run time (inside open()'s
	// resolution), past that unwrap point — so we take `.Func`, the
	// raw `func(goja.FunctionCall) goja.Value` that goja recognises
	// as a host function directly. Without this, the methods export
	// to JS as plain objects and `db.exec(...)` throws "Not a
	// function".
	return map[string]any{
		"exec": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqliteExec(ctx, db, call)
		}).Func,
		"query": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) ([]map[string]any, error) {
			return sqliteQuery(ctx, db, call)
		}).Func,
		"queryValue": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			return sqliteQueryValue(ctx, db, call)
		}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := db.Close(); err != nil {
				return nil, fmt.Errorf("sqlite.close: %w", err)
			}
			return nil, nil
		}).Func,
	}
}

// sqliteExec runs a non-query statement (DDL, INSERT, UPDATE,
// DELETE) and reports row counters. The SQL string is the first
// arg; all remaining args bind as `?` placeholders in order. Both
// lastInsertId and rowsAffected are pulled out via driver methods —
// not all statements populate them, so callers should expect zero
// values for e.g. CREATE TABLE.
func sqliteExec(ctx context.Context, db *sql.DB, call goja.FunctionCall) (map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, errors.New("sqlite.exec: sql argument required")
	}
	stmt := call.Argument(0).String()
	args := sqlitePositionalArgs(call)
	res, err := db.ExecContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite.exec: %w", err)
	}
	rowsAffected, _ := res.RowsAffected()
	lastInsertID, _ := res.LastInsertId()
	return map[string]any{
		"rowsAffected": rowsAffected,
		"lastInsertId": lastInsertID,
	}, nil
}

// sqliteQuery runs a SELECT-style statement and returns one map per
// row, keyed by column name. Column-name conflicts (rare but
// possible when joining tables) overwrite earlier values — SQL
// scripts that need both should alias one. Column types are mapped
// by sqliteScanValue.
func sqliteQuery(ctx context.Context, db *sql.DB, call goja.FunctionCall) ([]map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, errors.New("sqlite.query: sql argument required")
	}
	stmt := call.Argument(0).String()
	args := sqlitePositionalArgs(call)
	rows, err := db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite.query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("sqlite.query: columns: %w", err)
	}
	out := []map[string]any{}
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("sqlite.query: scan: %w", err)
		}
		row := make(map[string]any, len(cols))
		for i, name := range cols {
			row[name] = sqliteScanValue(raw[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sqlite.query: iterate: %w", err)
	}
	return out, nil
}

// sqliteQueryValue runs a statement expected to produce a scalar —
// `SELECT count(*) FROM ...`, `SELECT name FROM ... WHERE id = ?`,
// `PRAGMA user_version`. Returns the first column of the first
// row, or `nil` (JS null) when no rows match. Anything beyond the
// first row is discarded.
func sqliteQueryValue(ctx context.Context, db *sql.DB, call goja.FunctionCall) (any, error) {
	if len(call.Arguments) < 1 {
		return nil, errors.New("sqlite.queryValue: sql argument required")
	}
	stmt := call.Argument(0).String()
	args := sqlitePositionalArgs(call)
	rows, err := db.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("sqlite.queryValue: %w", err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("sqlite.queryValue: %w", err)
		}
		return nil, nil
	}
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return nil, fmt.Errorf("sqlite.queryValue: scan: %w", err)
	}
	return sqliteScanValue(raw), nil
}

// sqlitePositionalArgs reads the SQL bind parameters from the JS
// call (everything after the SQL string at position 0). goja
// exports JS numbers as int64 / float64, strings as string, bools
// as bool, null as nil, and Uint8Array as []byte — all of which
// modernc.org/sqlite accepts directly. Pass-through is enough.
func sqlitePositionalArgs(call goja.FunctionCall) []any {
	if len(call.Arguments) <= 1 {
		return nil
	}
	out := make([]any, 0, len(call.Arguments)-1)
	for _, arg := range call.Arguments[1:] {
		if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
			out = append(out, nil)
			continue
		}
		out = append(out, arg.Export())
	}
	return out
}

// sqliteScanValue normalises a value that came back from
// rows.Scan(*interface{}) into the JS-friendly type. modernc.org's
// driver hands back the SQLite native types directly, but []byte
// for TEXT columns is the case worth normalising — we'd rather
// scripts see a string for TEXT and a Uint8Array for BLOB, so we
// convert []byte to string when it's valid UTF-8 and leave it as
// bytes otherwise (the heuristic is "did it round-trip through
// utf8" — TEXT columns always do, BLOB usually don't).
func sqliteScanValue(v any) any {
	switch x := v.(type) {
	case []byte:
		// SQLite has no rigid TEXT/BLOB distinction at the storage
		// layer — both come back as []byte from the driver. We
		// promote to string when the bytes are valid UTF-8, which
		// covers the overwhelmingly common TEXT case. Truly binary
		// BLOBs (PNG bytes, encrypted payloads) stay as []byte and
		// surface to JS as Uint8Array.
		if utf8.Valid(x) {
			return string(x)
		}
		return x
	default:
		return v
	}
}
