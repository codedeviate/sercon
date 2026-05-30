package main

import (
	"testing"

	"github.com/dop251/goja"
)

// The DSN builders are pure functions over a connection-options map; assert
// they assemble the exact driver DSN (credentials escaped, port/db placed
// per each engine's format).
func TestEngineDSNBuilders(t *testing.T) {
	opts := map[string]any{
		"host":     "db.example.com",
		"port":     int64(5433),
		"user":     "alice",
		"password": "s3cret",
		"database": "app",
		"sslmode":  "require",
	}
	if got, want := postgresDSN(opts), "postgres://alice:s3cret@db.example.com:5433/app?sslmode=require"; got != want {
		t.Errorf("postgresDSN:\n got: %s\nwant: %s", got, want)
	}
	if got, want := mysqlDSN(opts), "alice:s3cret@tcp(db.example.com:5433)/app"; got != want {
		t.Errorf("mysqlDSN:\n got: %s\nwant: %s", got, want)
	}
	if got, want := mssqlDSN(opts), "sqlserver://alice:s3cret@db.example.com:5433?database=app"; got != want {
		t.Errorf("mssqlDSN:\n got: %s\nwant: %s", got, want)
	}
}

// Defaults: host localhost and the engine's standard port when omitted.
func TestEngineDSNDefaults(t *testing.T) {
	empty := map[string]any{}
	if got, want := postgresDSN(empty), "postgres://localhost:5432/"; got != want {
		t.Errorf("postgres default DSN: %s (want %s)", got, want)
	}
	if got, want := mysqlDSN(empty), "tcp(localhost:3306)/"; got != want {
		t.Errorf("mysql default DSN: %s (want %s)", got, want)
	}
	if got, want := mssqlDSN(empty), "sqlserver://localhost:1433"; got != want {
		t.Errorf("mssql default DSN: %s (want %s)", got, want)
	}
}

// Credentials with URL-special characters are percent-escaped in the URL-form
// DSNs (postgres / mssql), so a password like "p@ss/word" can't corrupt the DSN.
func TestEngineDSN_EscapesCredentials(t *testing.T) {
	opts := map[string]any{"host": "h", "user": "a b", "password": "p@ss/word", "database": "d"}
	if got := postgresDSN(opts); !contains(got, "p%40ss%2Fword") {
		t.Errorf("postgresDSN should percent-escape password: %s", got)
	}
	if got := mssqlDSN(opts); !contains(got, "p%40ss%2Fword") {
		t.Errorf("mssqlDSN should percent-escape password: %s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// dbConnArg: a missing argument errors; the engine name prefixes it.
func TestDBConnArg_RequiresArgument(t *testing.T) {
	if _, _, err := dbConnArg(goja.FunctionCall{}, "postgres"); err == nil {
		t.Error("expected error when open() called with no argument")
	}
}
