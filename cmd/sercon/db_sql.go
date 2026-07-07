package main

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"unicode/utf8"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// This file holds the engine-agnostic database/sql handle shared by every
// db.<engine> binding (sqlite, postgres, mysql, mssql, clickhouse, oracle).
// Each engine's
// namespace differs only in the registered driver, how it builds the DSN, and
// the `engine` string that prefixes error labels — everything else (the
// open→handle shape, transactions, prepared statements, row scanning) lives
// here. db.sqlite was the original; this generalises it.

// sqlExecutor is the slice of database/sql that both *sql.DB and *sql.Tx
// satisfy, so the exec/query/queryValue helpers serve the top-level handle and
// the transaction handle through one code path.
type sqlExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// sqlOpen opens a database/sql connection for the named driver + DSN, pings it
// (so a bad DSN / unreachable server surfaces at open() rather than the first
// query, and the *sql.DB is closed on failure rather than leaked), and returns
// the JS-facing handle. Shared by every db.<engine>.open.
func sqlOpen(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, driver, dsn, engine string) (map[string]any, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("%s.open: %w", engine, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("%s.open: ping: %w", engine, err)
	}
	return sqlHandle(vm, loop, db, engine), nil
}

// sqlHandle builds the JS object open() returns: Promise-returning methods over
// the *sql.DB. `engine` prefixes error labels (e.g. "postgres.exec"). The
// `.Func` unwrap is required because this map is built at script-run time (past
// the engine's registration-time AsyncBinding unwrap) — see the original note
// in sqlite history.
func sqlHandle(vm *goja.Runtime, loop *eventloop.EventLoop, db *sql.DB, engine string) map[string]any {
	return map[string]any{
		"exec": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqlExec(ctx, db, call, engine+".exec")
		}).Func,
		"query": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) ([]*scriptengine.Ordered, error) {
			return sqlQuery(ctx, db, call, engine+".query")
		}).Func,
		"queryValue": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			return sqlQueryValue(ctx, db, call, engine+".queryValue")
		}).Func,
		"begin": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqlBegin(vm, loop, ctx, db, engine)
		}).Func,
		"prepare": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqlPrepare(vm, loop, ctx, db, call, engine)
		}).Func,
		"close": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := db.Close(); err != nil {
				return nil, fmt.Errorf("%s.close: %w", engine, err)
			}
			return nil, nil
		}).Func,
	}
}

// sqlPrepare compiles a SQL statement once and returns a handle whose
// exec/query/queryValue execute it repeatedly with fresh bind params. The
// statement holds driver resources until close(); scripts MUST close it.
func sqlPrepare(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, db *sql.DB, call goja.FunctionCall, engine string) (map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("%s.prepare: sql argument required", engine)
	}
	query := call.Argument(0).String()
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s.prepare: %w", engine, err)
	}
	return map[string]any{
		"exec": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			res, err := stmt.ExecContext(ctx, sqlArgsFrom(call, 0)...)
			if err != nil {
				return nil, fmt.Errorf("%s.stmt.exec: %w", engine, err)
			}
			rowsAffected, _ := res.RowsAffected()
			lastInsertID, _ := res.LastInsertId()
			return map[string]any{"rowsAffected": rowsAffected, "lastInsertId": lastInsertID}, nil
		}).Func,
		"query": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) ([]*scriptengine.Ordered, error) {
			rows, err := stmt.QueryContext(ctx, sqlArgsFrom(call, 0)...)
			if err != nil {
				return nil, fmt.Errorf("%s.stmt.query: %w", engine, err)
			}
			defer func() { _ = rows.Close() }()
			return scanRows(rows, engine+".stmt.query")
		}).Func,
		"queryValue": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			rows, err := stmt.QueryContext(ctx, sqlArgsFrom(call, 0)...)
			if err != nil {
				return nil, fmt.Errorf("%s.stmt.queryValue: %w", engine, err)
			}
			defer func() { _ = rows.Close() }()
			return scanFirstValue(rows, engine+".stmt.queryValue")
		}).Func,
		"close": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := stmt.Close(); err != nil {
				return nil, fmt.Errorf("%s.stmt.close: %w", engine, err)
			}
			return nil, nil
		}).Func,
	}, nil
}

// sqlBegin opens a transaction and returns the same exec/query/queryValue
// surface plus commit/rollback. Scripts MUST finalize it (commit or rollback);
// a leaked tx pins a pooled connection.
func sqlBegin(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, db *sql.DB, engine string) (map[string]any, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("%s.begin: %w", engine, err)
	}
	return map[string]any{
		"exec": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqlExec(ctx, tx, call, engine+".tx.exec")
		}).Func,
		"query": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) ([]*scriptengine.Ordered, error) {
			return sqlQuery(ctx, tx, call, engine+".tx.query")
		}).Func,
		"queryValue": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			return sqlQueryValue(ctx, tx, call, engine+".tx.queryValue")
		}).Func,
		"commit": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("%s.tx.commit: %w", engine, err)
			}
			return nil, nil
		}).Func,
		"rollback": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := tx.Rollback(); err != nil {
				return nil, fmt.Errorf("%s.tx.rollback: %w", engine, err)
			}
			return nil, nil
		}).Func,
	}, nil
}

// sqlExec runs a non-query statement and reports row counters. SQL string is
// arg 0; remaining args bind as positional placeholders (the script writes the
// engine's placeholder syntax — `?` for sqlite/mysql, `$1` for postgres,
// `@p1` for mssql). `label` prefixes errors.
func sqlExec(ctx context.Context, ex sqlExecutor, call goja.FunctionCall, label string) (map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("%s: sql argument required", label)
	}
	stmt := call.Argument(0).String()
	res, err := ex.ExecContext(ctx, stmt, sqlPositionalArgs(call)...)
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

// sqlQuery runs a SELECT-style statement and returns one ordered object per
// row, keyed by column name in column order.
func sqlQuery(ctx context.Context, ex sqlExecutor, call goja.FunctionCall, label string) ([]*scriptengine.Ordered, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("%s: sql argument required", label)
	}
	stmt := call.Argument(0).String()
	rows, err := ex.QueryContext(ctx, stmt, sqlPositionalArgs(call)...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	defer func() { _ = rows.Close() }()
	return scanRows(rows, label)
}

// scanRows drains a *sql.Rows into one ordered object per row, keyed by column
// name in column order so JSON.stringify output is stable run-to-run.
func scanRows(rows *sql.Rows, label string) ([]*scriptengine.Ordered, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("%s: columns: %w", label, err)
	}
	out := []*scriptengine.Ordered{}
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("%s: scan: %w", label, err)
		}
		row := scriptengine.NewOrdered()
		for i, name := range cols {
			row.Set(name, sqlScanValue(raw[i]))
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%s: iterate: %w", label, err)
	}
	return out, nil
}

// sqlQueryValue runs a statement expected to produce a scalar and returns the
// first column of the first row, or nil (JS null) when no rows match.
func sqlQueryValue(ctx context.Context, ex sqlExecutor, call goja.FunctionCall, label string) (any, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("%s: sql argument required", label)
	}
	stmt := call.Argument(0).String()
	rows, err := ex.QueryContext(ctx, stmt, sqlPositionalArgs(call)...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	defer func() { _ = rows.Close() }()
	return scanFirstValue(rows, label)
}

// scanFirstValue returns the first column of the first row (nil if none).
// Shared by the handle/tx queryValue and the prepared-statement queryValue.
func scanFirstValue(rows *sql.Rows, label string) (any, error) {
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
	return sqlScanValue(raw), nil
}

// sqlPositionalArgs reads the SQL bind parameters from the JS call (everything
// after the SQL string at index 0).
func sqlPositionalArgs(call goja.FunctionCall) []any {
	return sqlArgsFrom(call, 1)
}

// sqlArgsFrom reads bind parameters starting at `start`. goja exports JS
// numbers as int64/float64, strings as string, bools as bool, null as nil, and
// Uint8Array as []byte — all of which the SQL drivers accept directly.
func sqlArgsFrom(call goja.FunctionCall, start int) []any {
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

// sqlScanValue normalises a rows.Scan(*any) value into a JS-friendly type:
// a []byte that is valid UTF-8 becomes a string (the common TEXT case), while
// genuinely binary bytes stay []byte (surfaced to JS as Uint8Array).
func sqlScanValue(v any) any {
	if b, ok := v.([]byte); ok && utf8.Valid(b) {
		return string(b)
	}
	return v
}

// dbConnArg interprets the first argument of a server-engine `open()`
// (postgres / mysql / mssql): a string is used verbatim as the driver DSN; an
// object is a connection-spec map the engine assembles into a DSN. Exactly one
// of (dsn, opts) is non-empty on success.
func dbConnArg(call goja.FunctionCall, engine string) (dsn string, opts map[string]any, err error) {
	if len(call.Arguments) < 1 {
		return "", nil, fmt.Errorf("%s.open: a DSN string or connection-options object is required", engine)
	}
	switch v := call.Argument(0).Export().(type) {
	case string:
		if v == "" {
			return "", nil, fmt.Errorf("%s.open: DSN string is empty", engine)
		}
		return v, nil, nil
	case map[string]any:
		return "", v, nil
	default:
		return "", nil, fmt.Errorf("%s.open: argument must be a DSN string or an options object", engine)
	}
}

// dbOptPort reads opts["port"] as a string (accepting a JS number or string),
// falling back to def.
func dbOptPort(opts map[string]any, def string) string {
	switch n := opts["port"].(type) {
	case int64:
		return strconv.FormatInt(n, 10)
	case int:
		return strconv.Itoa(n)
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case string:
		if n != "" {
			return n
		}
	}
	return def
}
