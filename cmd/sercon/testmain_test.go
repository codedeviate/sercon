package main

import (
	"os"
	"testing"
)

// TestMain points the stuffr wazero compilation cache at a throwaway dir so
// the test binary never writes the user's real cache, and removes it (about
// 10 MB per run) once the tests finish.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sercon-test-wazero-")
	if err != nil {
		panic(err)
	}
	orig := stuffrCacheDir
	stuffrCacheDir = func() string { return dir }
	code := m.Run()
	stuffrCacheDir = orig
	_ = os.RemoveAll(dir)
	os.Exit(code)
}
