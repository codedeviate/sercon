package main

import (
	"context"
	"net"
	"net/url"

	_ "github.com/ClickHouse/clickhouse-go/v2" // register the pure-Go "clickhouse" database/sql driver
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// clickhouseNamespace wires `db.clickhouse.*` over the pure-Go clickhouse-go
// v2 driver (its database/sql adapter). open() takes a clickhouse:// DSN
// string, or an options object {host, port, user, password, database, secure}
// assembled into a clickhouse:// URL. It returns the shared database/sql
// handle (exec/query/queryValue/begin/prepare/close — see db_sql.go).
// ClickHouse uses ? positional placeholders (or @name). Default native-protocol
// port is 9000 (9440 when secure).
func clickhouseNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			dsn, opts, err := dbConnArg(call, "clickhouse")
			if err != nil {
				return nil, err
			}
			if opts != nil {
				dsn = clickhouseDSN(opts)
			}
			return sqlOpen(vm, loop, ctx, "clickhouse", dsn, "clickhouse")
		}),
	}
}

// clickhouseDSN assembles a clickhouse:// URL from a connection-options object
// (database in the path, per the clickhouse-go URL form). url.URL handles
// percent-escaping of credentials.
func clickhouseDSN(opts map[string]any) string {
	u := url.URL{
		Scheme: "clickhouse",
		Host:   net.JoinHostPort(optString(opts, "host", "localhost"), dbOptPort(opts, "9000")),
		Path:   "/" + optString(opts, "database", ""),
	}
	if user := optString(opts, "user", ""); user != "" {
		if pw := optString(opts, "password", ""); pw != "" {
			u.User = url.UserPassword(user, pw)
		} else {
			u.User = url.User(user)
		}
	}
	if b, ok := opts["secure"].(bool); ok && b {
		q := url.Values{}
		q.Set("secure", "true")
		u.RawQuery = q.Encode()
	}
	return u.String()
}
