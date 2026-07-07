package main

import (
	"context"
	"net"
	"net/url"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	_ "github.com/sijms/go-ora/v2" // register the pure-Go "oracle" database/sql driver

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// oracleNamespace wires `db.oracle.*` over the pure-Go go-ora driver (no cgo,
// unlike the OCI-bound godror). open() takes an oracle:// DSN string, or an
// options object {host, port, user, password, database} where `database` is
// the service name, assembled into an oracle:// URL. It returns the shared
// database/sql handle (exec/query/queryValue/begin/prepare/close — see
// db_sql.go). Oracle uses :1, :2, … (or :name) bind placeholders. Default
// listener port is 1521.
func oracleNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			dsn, opts, err := dbConnArg(call, "oracle")
			if err != nil {
				return nil, err
			}
			if opts != nil {
				dsn = oracleDSN(opts)
			}
			return sqlOpen(vm, loop, ctx, "oracle", dsn, "oracle")
		}),
	}
}

// oracleDSN assembles an oracle:// URL from a connection-options object (the
// service name goes in the path, per the go-ora URL form). url.URL handles
// percent-escaping of credentials.
func oracleDSN(opts map[string]any) string {
	u := url.URL{
		Scheme: "oracle",
		Host:   net.JoinHostPort(optString(opts, "host", "localhost"), dbOptPort(opts, "1521")),
		Path:   "/" + optString(opts, "database", ""),
	}
	if user := optString(opts, "user", ""); user != "" {
		if pw := optString(opts, "password", ""); pw != "" {
			u.User = url.UserPassword(user, pw)
		} else {
			u.User = url.User(user)
		}
	}
	return u.String()
}
