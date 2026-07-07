package main

import (
	"context"
	"net"
	"net/url"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	_ "github.com/jackc/pgx/v5/stdlib" // register the pure-Go "pgx" database/sql driver

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// postgresNamespace wires `db.postgres.*` over the pure-Go pgx driver (its
// database/sql stdlib adapter). open() takes a libpq DSN/URL string, or an
// options object {host, port, user, password, database, sslmode} assembled
// into a postgres:// URL. It returns the shared database/sql handle
// (exec/query/queryValue/begin/prepare/close — see db_sql.go). Postgres uses
// $1, $2, … placeholders. CockroachDB and other Postgres-wire engines work
// through the same driver.
func postgresNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			dsn, opts, err := dbConnArg(call, "postgres")
			if err != nil {
				return nil, err
			}
			if opts != nil {
				dsn = postgresDSN(opts)
			}
			return sqlOpen(vm, loop, ctx, "pgx", dsn, "postgres")
		}),
	}
}

// postgresDSN assembles a postgres:// URL from a connection-options object.
// url.URL handles percent-escaping of credentials.
func postgresDSN(opts map[string]any) string {
	u := url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(optString(opts, "host", "localhost"), dbOptPort(opts, "5432")),
		Path:   "/" + optString(opts, "database", ""),
	}
	if user := optString(opts, "user", ""); user != "" {
		if pw := optString(opts, "password", ""); pw != "" {
			u.User = url.UserPassword(user, pw)
		} else {
			u.User = url.User(user)
		}
	}
	q := url.Values{}
	if ssl := optString(opts, "sslmode", ""); ssl != "" {
		q.Set("sslmode", ssl)
	}
	u.RawQuery = q.Encode()
	return u.String()
}
