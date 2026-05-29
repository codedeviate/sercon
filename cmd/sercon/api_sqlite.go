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

// sqliteNamespace wires `db.sqlite.*`. The namespace only exposes
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
			return sqliteExec(ctx, db, call, "sqlite.exec")
		}).Func,
		"query": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) ([]map[string]any, error) {
			return sqliteQuery(ctx, db, call, "sqlite.query")
		}).Func,
		"queryValue": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			return sqliteQueryValue(ctx, db, call, "sqlite.queryValue")
		}).Func,
		"begin": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqliteBegin(vm, loop, ctx, db)
		}).Func,
		"prepare": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqlitePrepare(vm, loop, ctx, db, call)
		}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := db.Close(); err != nil {
				return nil, fmt.Errorf("sqlite.close: %w", err)
			}
			return nil, nil
		}).Func,
	}
}

// sqlitePrepare compiles a SQL statement once and returns a handle
// whose exec / query / queryValue execute it repeatedly with fresh
// bind parameters — no SQL string on those calls, just the `?`
// params. Worthwhile for batch-insert / repeated-lookup loops where
// the per-call parse + plan cost would otherwise dominate.
//
// Lifetime: a prepared statement holds driver resources until
// close(). Scripts MUST close it — a leaked statement keeps a
// connection pinned. The statement is bound to the database handle,
// not a transaction; using it inside a transaction is out of scope
// for this cut (it complicates ownership — see OUT-OF-SCOPE).
func sqlitePrepare(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, db *sql.DB, call goja.FunctionCall) (map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, errors.New("sqlite.prepare: sql argument required")
	}
	query := call.Argument(0).String()
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("sqlite.prepare: %w", err)
	}
	return map[string]any{
		"exec": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			res, err := stmt.ExecContext(ctx, sqliteArgsFrom(call, 0)...)
			if err != nil {
				return nil, fmt.Errorf("sqlite.stmt.exec: %w", err)
			}
			rowsAffected, _ := res.RowsAffected()
			lastInsertID, _ := res.LastInsertId()
			return map[string]any{"rowsAffected": rowsAffected, "lastInsertId": lastInsertID}, nil
		}).Func,
		"query": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) ([]map[string]any, error) {
			rows, err := stmt.QueryContext(ctx, sqliteArgsFrom(call, 0)...)
			if err != nil {
				return nil, fmt.Errorf("sqlite.stmt.query: %w", err)
			}
			defer func() { _ = rows.Close() }()
			return scanRows(rows, "sqlite.stmt.query")
		}).Func,
		"queryValue": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			rows, err := stmt.QueryContext(ctx, sqliteArgsFrom(call, 0)...)
			if err != nil {
				return nil, fmt.Errorf("sqlite.stmt.queryValue: %w", err)
			}
			defer func() { _ = rows.Close() }()
			if !rows.Next() {
				if err := rows.Err(); err != nil {
					return nil, fmt.Errorf("sqlite.stmt.queryValue: %w", err)
				}
				return nil, nil
			}
			var raw any
			if err := rows.Scan(&raw); err != nil {
				return nil, fmt.Errorf("sqlite.stmt.queryValue: scan: %w", err)
			}
			return sqliteScanValue(raw), nil
		}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := stmt.Close(); err != nil {
				return nil, fmt.Errorf("sqlite.stmt.close: %w", err)
			}
			return nil, nil
		}).Func,
	}, nil
}

// sqliteBegin opens a transaction and returns its handle object —
// the same { exec, query, queryValue } surface as the top-level
// handle, plus commit / rollback to finalize. The transaction
// reuses the exec/query/queryValue helpers through the sqlExecutor
// interface; *sql.Tx satisfies it.
//
// Lifetime: a transaction holds a connection out of the pool until
// it's committed or rolled back. Scripts MUST call one of them —
// a leaked transaction pins a connection until GC. There's no
// auto-rollback on handle close in this cut; the documented
// pattern is begin / … / commit-or-rollback in a try/finally.
//
// Once committed or rolled back, the *sql.Tx is spent: further
// exec/query calls return `sql.ErrTxDone`, which surfaces to the
// script as a thrown `sqlite.tx.*: ...` error. commit-after-commit
// and rollback-after-commit behave the same way.
func sqliteBegin(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, db *sql.DB) (map[string]any, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("sqlite.begin: %w", err)
	}
	return map[string]any{
		"exec": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqliteExec(ctx, tx, call, "sqlite.tx.exec")
		}).Func,
		"query": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) ([]map[string]any, error) {
			return sqliteQuery(ctx, tx, call, "sqlite.tx.query")
		}).Func,
		"queryValue": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			return sqliteQueryValue(ctx, tx, call, "sqlite.tx.queryValue")
		}).Func,
		"commit": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("sqlite.tx.commit: %w", err)
			}
			return nil, nil
		}).Func,
		"rollback": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := tx.Rollback(); err != nil {
				return nil, fmt.Errorf("sqlite.tx.rollback: %w", err)
			}
			return nil, nil
		}).Func,
	}, nil
}

// sqlExecutor is the slice of database/sql both *sql.DB and *sql.Tx
// satisfy. The exec / query / queryValue helpers take it (rather
// than a concrete *sql.DB) so the transaction handle from begin()
// reuses the exact same code paths as the top-level handle — only
// the executor differs.
type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// sqliteExec runs a non-query statement (DDL, INSERT, UPDATE,
// DELETE) and reports row counters. The SQL string is the first
// arg; all remaining args bind as `?` placeholders in order. Both
// lastInsertId and rowsAffected are pulled out via driver methods —
// not all statements populate them, so callers should expect zero
// values for e.g. CREATE TABLE. `label` prefixes errors so a
// transaction's exec reports `sqlite.tx.exec:` rather than the
// top-level `sqlite.exec:`.
func sqliteExec(ctx context.Context, ex sqlExecutor, call goja.FunctionCall, label string) (map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("%s: sql argument required", label)
	}
	stmt := call.Argument(0).String()
	args := sqlitePositionalArgs(call)
	res, err := ex.ExecContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
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
func sqliteQuery(ctx context.Context, ex sqlExecutor, call goja.FunctionCall, label string) ([]map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("%s: sql argument required", label)
	}
	stmt := call.Argument(0).String()
	args := sqlitePositionalArgs(call)
	rows, err := ex.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	defer func() { _ = rows.Close() }()
	return scanRows(rows, label)
}

// scanRows drains a *sql.Rows into one column-name-keyed map per
// row. Shared by the handle, transaction, and (future) prepared-
// statement query paths.
func scanRows(rows *sql.Rows, label string) ([]map[string]any, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("%s: columns: %w", label, err)
	}
	out := []map[string]any{}
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", label, err)
		}
		row := make(map[string]any, len(cols))
		for i, name := range cols {
			row[name] = sqliteScanValue(raw[i])
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: iterate: %w", label, err)
	}
	return out, nil
}

// sqliteQueryValue runs a statement expected to produce a scalar —
// `SELECT count(*) FROM ...`, `SELECT name FROM ... WHERE id = ?`,
// `PRAGMA user_version`. Returns the first column of the first
// row, or `nil` (JS null) when no rows match. Anything beyond the
// first row is discarded.
func sqliteQueryValue(ctx context.Context, ex sqlExecutor, call goja.FunctionCall, label string) (any, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("%s: sql argument required", label)
	}
	stmt := call.Argument(0).String()
	args := sqlitePositionalArgs(call)
	rows, err := ex.QueryContext(ctx, stmt, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	defer func() { _ = rows.Close() }()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
		return nil, nil
	}
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return nil, fmt.Errorf("%s: scan: %w", label, err)
	}
	return sqliteScanValue(raw), nil
}

// sqlitePositionalArgs reads the SQL bind parameters from the JS
// call (everything after the SQL string at position 0). goja
// exports JS numbers as int64 / float64, strings as string, bools
// as bool, null as nil, and Uint8Array as []byte — all of which
// modernc.org/sqlite accepts directly. Pass-through is enough.
func sqlitePositionalArgs(call goja.FunctionCall) []any {
	return sqliteArgsFrom(call, 1)
}

// sqliteArgsFrom reads bind parameters starting at `start`. The
// top-level exec/query/queryValue read from index 1 (after the SQL
// string); prepared-statement methods read from index 0 (the SQL
// was supplied at prepare() time, so the call carries only params).
func sqliteArgsFrom(call goja.FunctionCall, start int) []any {
	if len(call.Arguments) <= start {
		return nil
	}
	out := make([]any, 0, len(call.Arguments)-start)
	for _, arg := range call.Arguments[start:] {
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
