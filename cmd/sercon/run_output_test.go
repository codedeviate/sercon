// cmd/sercon/run_output_test.go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns what
// was written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var b strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				b.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		done <- b.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = orig
	return <-done
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
