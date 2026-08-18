// cmd/sercon/run_output_test.go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with the stdout registry stream redirected to a
// buffer and returns what was written. Predates the registry (it used to
// swap the os.Stdout package var behind a pipe); everything script-facing
// that used to write to a bare os.Stdout now goes through stdioOutStream.
//
// Swaps the stdioOutStream package var itself (buffer as the new stream's
// BASE) rather than pushing a destination onto the existing stream's stack:
// fn() here is run(...)/runRun(...), which reaches runOne, which now calls
// resetStdio() at the start of every run. That drops every entry on the
// stream's stack — a pushed capture would be gone before the script under
// test wrote a byte. reset() never touches base, so a swapped-in stream's
// capture buffer survives it.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	oldOut := stdioOutStream
	stdioOutStream = newStream("stdout", &buf)
	fn()
	stdioOutStream = oldOut
	return buf.String()
}

func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRunOutput_DefaultQuietOnSuccess(t *testing.T) {
	ok := writeScript(t, "ok.ts", `runtime.log("hello-from-script");`)
	out := captureStdout(t, func() { run([]string{ok}) })
	if strings.Contains(out, "PASS ") {
		t.Errorf("default mode should not print PASS; got:\n%s", out)
	}
	if !strings.Contains(out, "hello-from-script") {
		t.Errorf("script console output must still print; got:\n%s", out)
	}
}

func TestRunOutput_VerbosePrintsPass(t *testing.T) {
	ok := writeScript(t, "ok.ts", `runtime.log("x");`)
	out := captureStdout(t, func() { run([]string{"--verbose", ok}) })
	if !strings.Contains(out, "PASS ") {
		t.Errorf("--verbose should print PASS; got:\n%s", out)
	}
}

func TestRunOutput_DefaultPrintsFail(t *testing.T) {
	bad := writeScript(t, "bad.ts", `throw new Error("boom");`)
	var code int
	out := captureStdout(t, func() { code = run([]string{bad}) })
	if !strings.Contains(out, "FAIL ") {
		t.Errorf("default mode should print FAIL on error; got:\n%s", out)
	}
	if code == exitOK {
		t.Errorf("failing script should yield non-OK exit code")
	}
}

func TestRunOutput_SilentSuppressesStatusButKeepsExit(t *testing.T) {
	bad := writeScript(t, "bad.ts", `throw new Error("boom");`)
	var code int
	out := captureStdout(t, func() { code = run([]string{"--silent", bad}) })
	if strings.Contains(out, "FAIL ") || strings.Contains(out, "PASS ") {
		t.Errorf("--silent should print no status lines; got:\n%s", out)
	}
	if code == exitOK {
		t.Errorf("--silent must still return the failing exit code")
	}
}
