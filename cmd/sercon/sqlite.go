package main

import (
	"context"
	"errors"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	_ "modernc.org/sqlite" // register the pure-Go "sqlite" database/sql driver

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// sqliteNamespace wires `db.sqlite.*`. The namespace exposes only `open`; the
// handle it returns carries the real surface (exec / query / queryValue /
// begin / prepare / close), which is the engine-agnostic database/sql handle
// shared with db.postgres / db.mysql / db.mssql (see db_sql.go).
//
// `modernc.org/sqlite` is the pure-Go SQLite — no cgo, so the project's
// cgo-free rule holds. The blank import registers it as the "sqlite" driver.
func sqliteNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return sqliteOpen(vm, loop, ctx, call)
		}),
	}
}

// sqliteOpen connects to a SQLite database and returns the handle. `path` is
// ":memory:" for an in-RAM database or a filesystem path (missing files are
// created by the driver). The shared sqlOpen pings immediately so a bad path
// surfaces at open() rather than the first query.
func sqliteOpen(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	if len(call.Arguments) < 1 {
		return nil, errors.New("sqlite.open: path argument required (use \":memory:\" for in-RAM)")
	}
	path := call.Argument(0).String()
	if path == "" {
		return nil, errors.New("sqlite.open: path is empty (use \":memory:\" for in-RAM)")
	}
	return sqlOpen(vm, loop, ctx, "sqlite", path, "sqlite")
}
