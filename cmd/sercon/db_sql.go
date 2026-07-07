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

// sqlStmtArgs is the plain-Go carrier for a SQL statement plus its positional
// bind parameters, extracted on the event loop from the JS call. Everything in
// it is post-Export Go data (string / int64 / float64 / bool / []byte / nil),
// which database/sql accepts directly.
type sqlStmtArgs struct {
	stmt  string
	binds []any
}

// sqlStmtExtract builds the on-loop extract half for exec/query/queryValue:
// arg 0 is the SQL string (required), the rest bind as positional parameters.
// `label` prefixes the missing-argument error.
func sqlStmtExtract(label string) func(goja.FunctionCall) (sqlStmtArgs, error) {
	return func(call goja.FunctionCall) (sqlStmtArgs, error) {
		if len(call.Arguments) < 1 {
			return sqlStmtArgs{}, fmt.Errorf("%s: sql argument required", label)
		}
		return sqlStmtArgs{stmt: call.Argument(0).String(), binds: sqlPositionalArgs(call)}, nil
	}
}

// sqlBindsExtract is the on-loop extract half for prepared-statement methods,
// where the SQL is already bound: every argument is a bind parameter.
func sqlBindsExtract(call goja.FunctionCall) ([]any, error) {
	return sqlArgsFrom(call, 0), nil
}

// dbNoArgs is the extract half for zero-argument db bindings (close, commit,
// rollback, ping, ...). Shared across the db.* binding files.
func dbNoArgs(goja.FunctionCall) (struct{}, error) { return struct{}{}, nil }

// dbDSNExtract builds the on-loop extract half for a server-engine open():
// it resolves the first JS argument (a DSN string or a connection-options
// object) into the final driver DSN, using `assemble` to turn an options map
// into a DSN. Shared by db.postgres / db.mysql / db.mssql / db.oracle /
// db.clickhouse.
func dbDSNExtract(engine string, assemble func(map[string]any) string) func(goja.FunctionCall) (string, error) {
	return func(call goja.FunctionCall) (string, error) {
		dsn, opts, err := dbConnArg(call, engine)
		if err != nil {
			return "", err
		}
		if opts != nil {
			dsn = assemble(opts)
		}
		return dsn, nil
	}
}

// sqlHandle builds the JS object open() returns: Promise-returning methods over
// the *sql.DB. `engine` prefixes error labels (e.g. "postgres.exec"). The
// `.Func` unwrap is required because this map is built at script-run time (past
// the engine's registration-time AsyncBinding unwrap) — see the original note
// in sqlite history.
//
// Threading note on begin/prepare: their work funcs run in a goroutine and
// call sqlBegin/sqlPrepare, which *construct* new PromisifyAsync bindings
// capturing vm/loop. Construction touches neither (it only builds Go closures
// and reflects on Go types); every closure body runs on the event loop when JS
// later calls the method, and the returned map is materialised into a JS
// object on-loop at resolve time — so goja's threading contract holds.
func sqlHandle(vm *goja.Runtime, loop *eventloop.EventLoop, db *sql.DB, engine string) map[string]any {
	return map[string]any{
		"exec": scriptengine.PromisifyAsync(vm, loop, sqlStmtExtract(engine+".exec"),
			func(ctx context.Context, args sqlStmtArgs) (map[string]any, error) {
				return sqlExec(ctx, db, args, engine+".exec")
			}).Func,
		"query": scriptengine.PromisifyAsync(vm, loop, sqlStmtExtract(engine+".query"),
			func(ctx context.Context, args sqlStmtArgs) ([]*scriptengine.Ordered, error) {
				return sqlQuery(ctx, db, args, engine+".query")
			}).Func,
		"queryValue": scriptengine.PromisifyAsync(vm, loop, sqlStmtExtract(engine+".queryValue"),
			func(ctx context.Context, args sqlStmtArgs) (any, error) {
				return sqlQueryValue(ctx, db, args, engine+".queryValue")
			}).Func,
		"begin": scriptengine.PromisifyAsync(vm, loop, dbNoArgs,
			func(ctx context.Context, _ struct{}) (map[string]any, error) {
				return sqlBegin(vm, loop, ctx, db, engine)
			}).Func,
		"prepare": scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (string, error) {
				if len(call.Arguments) < 1 {
					return "", fmt.Errorf("%s.prepare: sql argument required", engine)
				}
				return call.Argument(0).String(), nil
			},
			func(ctx context.Context, query string) (map[string]any, error) {
				return sqlPrepare(vm, loop, ctx, db, query, engine)
			}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, dbNoArgs,
			func(ctx context.Context, _ struct{}) (any, error) {
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
// `query` is extracted on-loop by the prepare binding in sqlHandle.
func sqlPrepare(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, db *sql.DB, query string, engine string) (map[string]any, error) {
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s.prepare: %w", engine, err)
	}
	return map[string]any{
		"exec": scriptengine.PromisifyAsync(vm, loop, sqlBindsExtract,
			func(ctx context.Context, binds []any) (map[string]any, error) {
				res, err := stmt.ExecContext(ctx, binds...)
				if err != nil {
					return nil, fmt.Errorf("%s.stmt.exec: %w", engine, err)
				}
				rowsAffected, _ := res.RowsAffected()
				lastInsertID, _ := res.LastInsertId()
				return map[string]any{"rowsAffected": rowsAffected, "lastInsertId": lastInsertID}, nil
			}).Func,
		"query": scriptengine.PromisifyAsync(vm, loop, sqlBindsExtract,
			func(ctx context.Context, binds []any) ([]*scriptengine.Ordered, error) {
				rows, err := stmt.QueryContext(ctx, binds...)
				if err != nil {
					return nil, fmt.Errorf("%s.stmt.query: %w", engine, err)
				}
				defer func() { _ = rows.Close() }()
				return scanRows(rows, engine+".stmt.query")
			}).Func,
		"queryValue": scriptengine.PromisifyAsync(vm, loop, sqlBindsExtract,
			func(ctx context.Context, binds []any) (any, error) {
				rows, err := stmt.QueryContext(ctx, binds...)
				if err != nil {
					return nil, fmt.Errorf("%s.stmt.queryValue: %w", engine, err)
				}
				defer func() { _ = rows.Close() }()
				return scanFirstValue(rows, engine+".stmt.queryValue")
			}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, dbNoArgs,
			func(ctx context.Context, _ struct{}) (any, error) {
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
		"exec": scriptengine.PromisifyAsync(vm, loop, sqlStmtExtract(engine+".tx.exec"),
			func(ctx context.Context, args sqlStmtArgs) (map[string]any, error) {
				return sqlExec(ctx, tx, args, engine+".tx.exec")
			}).Func,
		"query": scriptengine.PromisifyAsync(vm, loop, sqlStmtExtract(engine+".tx.query"),
			func(ctx context.Context, args sqlStmtArgs) ([]*scriptengine.Ordered, error) {
				return sqlQuery(ctx, tx, args, engine+".tx.query")
			}).Func,
		"queryValue": scriptengine.PromisifyAsync(vm, loop, sqlStmtExtract(engine+".tx.queryValue"),
			func(ctx context.Context, args sqlStmtArgs) (any, error) {
				return sqlQueryValue(ctx, tx, args, engine+".tx.queryValue")
			}).Func,
		"commit": scriptengine.PromisifyAsync(vm, loop, dbNoArgs,
			func(ctx context.Context, _ struct{}) (any, error) {
				if err := tx.Commit(); err != nil {
					return nil, fmt.Errorf("%s.tx.commit: %w", engine, err)
				}
				return nil, nil
			}).Func,
		"rollback": scriptengine.PromisifyAsync(vm, loop, dbNoArgs,
			func(ctx context.Context, _ struct{}) (any, error) {
				if err := tx.Rollback(); err != nil {
					return nil, fmt.Errorf("%s.tx.rollback: %w", engine, err)
				}
				return nil, nil
			}).Func,
	}, nil
}

// sqlExec runs a non-query statement and reports row counters. The SQL string
// and bind parameters (the script writes the engine's placeholder syntax —
// `?` for sqlite/mysql, `$1` for postgres, `@p1` for mssql) arrive pre-
// extracted in `args` (see sqlStmtExtract). `label` prefixes errors.
func sqlExec(ctx context.Context, ex sqlExecutor, args sqlStmtArgs, label string) (map[string]any, error) {
	res, err := ex.ExecContext(ctx, args.stmt, args.binds...)
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
func sqlQuery(ctx context.Context, ex sqlExecutor, args sqlStmtArgs, label string) ([]*scriptengine.Ordered, error) {
	rows, err := ex.QueryContext(ctx, args.stmt, args.binds...)
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
func sqlQueryValue(ctx context.Context, ex sqlExecutor, args sqlStmtArgs, label string) (any, error) {
	rows, err := ex.QueryContext(ctx, args.stmt, args.binds...)
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
// after the SQL string at index 0). Touches goja values — extract-half only.
func sqlPositionalArgs(call goja.FunctionCall) []any {
	return sqlArgsFrom(call, 1)
}

// sqlArgsFrom reads bind parameters starting at `start`. goja exports JS
// numbers as int64/float64, strings as string, bools as bool, null as nil, and
// Uint8Array as []byte — all of which the SQL drivers accept directly.
// Touches goja values — extract-half only.
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
