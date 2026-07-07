package main

import (
	"context"
	"fmt"
	"net"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	_ "github.com/go-sql-driver/mysql" // register the pure-Go "mysql" database/sql driver

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mysqlNamespace wires `db.mysql.*` (also MariaDB — one driver, same wire
// protocol) over the pure-Go go-sql-driver/mysql. open() takes a go-sql-driver
// DSN string (`user:pass@tcp(host:port)/db?params`) or an options object
// {host, port, user, password, database}. Returns the shared database/sql
// handle (see db_sql.go). MySQL uses ? placeholders.
func mysqlNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, dbDSNExtract("mysql", mysqlDSN),
			func(ctx context.Context, dsn string) (map[string]any, error) {
				return sqlOpen(vm, loop, ctx, "mysql", dsn, "mysql")
			}),
	}
}

// mysqlDSN assembles a go-sql-driver DSN (user:pass@tcp(host:port)/db) from a
// connection-options object.
func mysqlDSN(opts map[string]any) string {
	addr := net.JoinHostPort(optString(opts, "host", "localhost"), dbOptPort(opts, "3306"))
	cred := ""
	if user := optString(opts, "user", ""); user != "" {
		if pw := optString(opts, "password", ""); pw != "" {
			cred = user + ":" + pw + "@"
		} else {
			cred = user + "@"
		}
	}
	return fmt.Sprintf("%stcp(%s)/%s", cred, addr, optString(opts, "database", ""))
}
