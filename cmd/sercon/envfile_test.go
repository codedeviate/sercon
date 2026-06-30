package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvFile(t *testing.T) {
	in := []byte("# a comment\n\nFOO=bar\nBAZ = \"qu ux\"\nexport Q='x y'\nEMPTY=\nHASHVAL=a#b\n")
	kvs, err := parseEnvFile(in)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"FOO": "bar", "BAZ": "qu ux", "Q": "x y", "EMPTY": "", "HASHVAL": "a#b"}
	if len(kvs) != len(want) {
		t.Fatalf("got %d pairs, want %d: %#v", len(kvs), len(want), kvs)
	}
	for _, kv := range kvs {
		if w, ok := want[kv.key]; !ok || w != kv.val {
			t.Fatalf("kv %q=%q not expected (want %q)", kv.key, kv.val, w)
		}
	}
}

func TestParseEnvFile_Malformed(t *testing.T) {
	if _, err := parseEnvFile([]byte("FOO=ok\nnoequals\n")); err == nil {
		t.Fatal("expected error on a line without '='")
	}
	if _, err := parseEnvFile([]byte("=novalue\n")); err == nil {
		t.Fatal("expected error on an empty key")
	}
}

func TestApplyEnvFiles_RealEnvWins(t *testing.T) {
	t.Setenv("SERCON_TEST_REAL", "fromenv")
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("SERCON_TEST_REAL=fromfile\nSERCON_TEST_NEW=fromfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// Make the "new" key absent and auto-restored after the test.
	t.Setenv("SERCON_TEST_NEW", "")
	if err := os.Unsetenv("SERCON_TEST_NEW"); err != nil {
		t.Fatal(err)
	}
	if err := applyEnvFiles([]string{p}); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("SERCON_TEST_REAL"); got != "fromenv" {
		t.Fatalf("real env should win: got %q want fromenv", got)
	}
	if got := os.Getenv("SERCON_TEST_NEW"); got != "fromfile" {
		t.Fatalf("new var should load from file: got %q want fromfile", got)
	}
}

func TestApplyEnvFiles_MissingFile(t *testing.T) {
	if err := applyEnvFiles([]string{filepath.Join(t.TempDir(), "nope.env")}); err == nil {
		t.Fatal("expected error for a missing env file")
	}
}

func TestEnvLoad_AppliesAndOverride(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/.env"
	if err := os.WriteFile(path, []byte("LOAD_NEW=fromfile\nLOAD_PRESET=fromfile\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOAD_PRESET", "preset") // already set → wins unless override

	m, err := envLoad(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if m["LOAD_NEW"] != "fromfile" || m["LOAD_PRESET"] != "fromfile" {
		t.Fatalf("returned map should hold all file pairs: %v", m)
	}
	if got := os.Getenv("LOAD_NEW"); got != "fromfile" {
		t.Fatalf("LOAD_NEW env = %q, want fromfile", got)
	}
	if got := os.Getenv("LOAD_PRESET"); got != "preset" {
		t.Fatalf("LOAD_PRESET env = %q, want preset (already-set wins)", got)
	}

	if _, err := envLoad(path, true); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("LOAD_PRESET"); got != "fromfile" {
		t.Fatalf("LOAD_PRESET after override = %q, want fromfile", got)
	}
}

func TestEnvLoad_Errors(t *testing.T) {
	if _, err := envLoad(t.TempDir()+"/missing.env", false); err == nil {
		t.Fatal("expected error for missing file")
	}
	bad := t.TempDir() + "/bad.env"
	if err := os.WriteFile(bad, []byte("nokey\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := envLoad(bad, false); err == nil {
		t.Fatal("expected error for malformed file")
	}
}
