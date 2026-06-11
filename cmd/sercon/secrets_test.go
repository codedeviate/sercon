package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/dop251/goja"
	"github.com/zalando/go-keyring"
)

func TestResolveSecretsPrefix(t *testing.T) {
	// flag wins over env wins over default
	cases := []struct {
		flag, env, want string
	}{
		{"", "", "sercon/"},         // default
		{"", "sws6/", "sws6/"},      // env
		{"team/", "sws6/", "team/"}, // flag beats env
		{"team/", "", "team/"},      // flag only
	}
	for i, c := range cases {
		t.Setenv("SERCON_SECRETS_PREFIX", c.env)
		secretsPrefixOverride = c.flag
		if got := resolveSecretsPrefix(); got != c.want {
			t.Errorf("case %d: flag=%q env=%q -> got %q want %q", i, c.flag, c.env, got, c.want)
		}
	}
	secretsPrefixOverride = "" // reset shared package state
}

func TestLinuxSecretsAvailable(t *testing.T) {
	cases := []struct {
		dbusAddr, runtimeDir string
		makeBusSocket        bool
		want                 bool
	}{
		{"unix:path=/run/user/1000/bus", "", false, true}, // DBUS addr set
		{"", "", false, false},                            // nothing
		{"", "", true, true},                              // bus socket present under runtimeDir
	}
	for i, c := range cases {
		dir := ""
		if c.makeBusSocket {
			dir = t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "bus"), nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if got := linuxSecretsAvailable(c.dbusAddr, dir); got != c.want {
			t.Errorf("case %d: addr=%q -> got %v want %v", i, c.dbusAddr, got, c.want)
		}
	}
}

// callWith builds a goja.FunctionCall with the given string args for invoking
// a work func directly in tests (no event loop needed — work funcs are plain
// Go).
func callWith(args ...string) goja.FunctionCall {
	vals := make([]goja.Value, len(args))
	rt := goja.New()
	for i, a := range args {
		vals[i] = rt.ToValue(a)
	}
	return goja.FunctionCall{Arguments: vals}
}

func TestSecretsRoundTrip(t *testing.T) {
	// MockInit swaps in an in-memory keyring provider PROCESS-WIDE for the rest
	// of this test binary. Harmless here — this is the only test that touches
	// keyring.* directly; any future test exercising a real backend must account
	// for this.
	keyring.MockInit()
	ctx := context.Background()

	get := secretsGet("sercon-test/")
	set := secretsSet("sercon-test/")
	del := secretsDelete("sercon-test/")

	// absent -> nil (JS null)
	if v, err := get(ctx, callWith("devshop", "tess")); err != nil || v != nil {
		t.Fatalf("get absent: v=%v err=%v want nil,nil", v, err)
	}

	// set then get
	if _, err := set(ctx, callWith("devshop", "tess", "hunter2")); err != nil {
		t.Fatalf("set: %v", err)
	}
	if v, err := get(ctx, callWith("devshop", "tess")); err != nil || v != "hunter2" {
		t.Fatalf("get present: v=%v err=%v want hunter2", v, err)
	}

	// the stored item lands under PREFIX+name in the backend
	if raw, err := keyring.Get("sercon-test/devshop", "tess"); err != nil || raw != "hunter2" {
		t.Fatalf("stored service should be PREFIX+name: raw=%q err=%v", raw, err)
	}

	// delete present -> true, then get -> nil, delete again -> false
	if v, err := del(ctx, callWith("devshop", "tess")); err != nil || v != true {
		t.Fatalf("delete present: v=%v err=%v want true", v, err)
	}
	if v, err := get(ctx, callWith("devshop", "tess")); err != nil || v != nil {
		t.Fatalf("get after delete: v=%v err=%v want nil", v, err)
	}
	if v, err := del(ctx, callWith("devshop", "tess")); err != nil || v != false {
		t.Fatalf("delete absent: v=%v err=%v want false", v, err)
	}
}

func TestSecretsArgValidation(t *testing.T) {
	keyring.MockInit()
	ctx := context.Background()
	get := secretsGet("sercon-test/")
	set := secretsSet("sercon-test/")
	del := secretsDelete("sercon-test/")

	// Missing name → error (would otherwise mis-key "sercon-test/undefined").
	if _, err := get(ctx, callWith()); err == nil {
		t.Error("get() with no args should error on missing name")
	}
	if _, err := del(ctx, callWith()); err == nil {
		t.Error("delete() with no args should error on missing name")
	}
	if _, err := set(ctx, callWith()); err == nil {
		t.Error("set() with no args should error on missing name")
	}
	// Empty name → error.
	if _, err := get(ctx, callWith("", "acct")); err == nil {
		t.Error("get() with empty name should error")
	}
	// Missing account → error (only name supplied).
	if _, err := get(ctx, callWith("devshop")); err == nil {
		t.Error("get() with no account should error on missing account")
	}
	if _, err := set(ctx, callWith("devshop")); err == nil {
		t.Error("set() with no account should error on missing account")
	}
	// name + account but missing secret → error.
	if _, err := set(ctx, callWith("devshop", "tess")); err == nil {
		t.Error("set() with no secret should error on missing secret")
	}

	// An EXPLICIT empty account is allowed (single-secret name) and round-trips.
	if _, err := set(ctx, callWith("singleton", "", "v")); err != nil {
		t.Fatalf("set with empty account should be allowed: %v", err)
	}
	if v, err := get(ctx, callWith("singleton", "")); err != nil || v != "v" {
		t.Fatalf("get with empty account: v=%v err=%v want v", v, err)
	}
}
