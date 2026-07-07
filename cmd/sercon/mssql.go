package main

import (
	"context"
	"net"
	"net/url"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	_ "github.com/microsoft/go-mssqldb" // register the pure-Go "sqlserver" database/sql driver

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// mssqlNamespace wires `db.mssql.*` over the pure-Go microsoft/go-mssqldb.
// open() takes a sqlserver:// URL DSN string, or an options object {host,
// port, user, password, database} assembled into one. Returns the shared
// database/sql handle (see db_sql.go). SQL Server uses @p1, @p2, …
// placeholders.
func mssqlNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, dbDSNExtract("mssql", mssqlDSN),
			func(ctx context.Context, dsn string) (map[string]any, error) {
				return sqlOpen(vm, loop, ctx, "sqlserver", dsn, "mssql")
			}),
	}
}

// mssqlDSN assembles a sqlserver:// URL from a connection-options object
// (the database goes in the query string, per the go-mssqldb URL form).
func mssqlDSN(opts map[string]any) string {
	u := url.URL{
		Scheme: "sqlserver",
		Host:   net.JoinHostPort(optString(opts, "host", "localhost"), dbOptPort(opts, "1433")),
	}
	if user := optString(opts, "user", ""); user != "" {
		if pw := optString(opts, "password", ""); pw != "" {
			u.User = url.UserPassword(user, pw)
		} else {
			u.User = url.User(user)
		}
	}
	q := url.Values{}
	if db := optString(opts, "database", ""); db != "" {
		q.Set("database", db)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
