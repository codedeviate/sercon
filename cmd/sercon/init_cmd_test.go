package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInit_WritesFiles(t *testing.T) {
	dir := t.TempDir()
	if code := runInit([]string{dir}); code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}
	dts, err := os.ReadFile(filepath.Join(dir, "sercon.d.ts"))
	if err != nil {
		t.Fatalf("sercon.d.ts: %v", err)
	}
	if !strings.Contains(string(dts), "declare const runtime") {
		t.Error("sercon.d.ts missing the reserved-global declarations")
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "jsconfig.json"))
	if err != nil {
		t.Fatalf("jsconfig.json: %v", err)
	}
	if !strings.Contains(string(cfg), "sercon.d.ts") {
		t.Error("jsconfig.json does not reference sercon.d.ts")
	}
}

func TestRunInit_SkipsExistingUnlessForce(t *testing.T) {
	dir := t.TempDir()
	dts := filepath.Join(dir, "sercon.d.ts")
	if err := os.WriteFile(dts, []byte("SENTINEL"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Without --force: existing file is left untouched.
	if code := runInit([]string{dir}); code != exitOK {
		t.Fatalf("got %d", code)
	}
	if b, _ := os.ReadFile(dts); string(b) != "SENTINEL" {
		t.Error("existing sercon.d.ts was overwritten without --force")
	}
	// With --force: overwritten with the real declarations.
	if code := runInit([]string{"-force", dir}); code != exitOK {
		t.Fatalf("force: got %d", code)
	}
	if b, _ := os.ReadFile(dts); string(b) == "SENTINEL" {
		t.Error("--force did not overwrite sercon.d.ts")
	}
}

func TestRunInit_TooManyArgs(t *testing.T) {
	if code := runInit([]string{"a", "b"}); code != exitUsage {
		t.Fatalf("expected exitUsage for two dir args, got %d", code)
	}
}
