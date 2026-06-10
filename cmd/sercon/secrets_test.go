package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSecretsPrefix(t *testing.T) {
	// flag wins over env wins over default
	cases := []struct {
		flag, env, want string
	}{
		{"", "", "sercon/"},        // default
		{"", "sws6/", "sws6/"},    // env
		{"team/", "sws6/", "team/"}, // flag beats env
		{"team/", "", "team/"},    // flag only
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
