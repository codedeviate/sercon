package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunServe_NoScript surfaces the usage-error path.
func TestRunServe_NoScript(t *testing.T) {
	code := runServe([]string{})
	if code != exitUsage {
		t.Fatalf("expected exitUsage, got %d", code)
	}
}

// stderrAccessLogger must go through the stderr registry stream, not a bare
// os.Stderr, so a served script's own runtime.stderr redirect also catches
// its access log. Supersedes the old format-shape-only sanity check now that
// the registry gives us an easy way to assert the actual routed content.
func TestStderrAccessLogger_RoutesThroughRegistry(t *testing.T) {
	var buf bytes.Buffer
	restore := stdioErrStream.push(destination{kind: destBuffer, w: &buf})
	stderrAccessLogger("127.0.0.1:54321", "GET", "/health", 200, 1234*time.Microsecond)
	restore()

	got := buf.String()
	for _, want := range []string{"127.0.0.1:54321", "GET", "/health", "200", "1234µs"} {
		if !strings.Contains(got, want) {
			t.Errorf("access log missing %q; got %q", want, got)
		}
	}
}

// smtpStderrLogger must also go through the stderr registry stream, in both
// its no-detail and with-detail forms.
func TestSMTPStderrLogger_RoutesThroughRegistry(t *testing.T) {
	var buf bytes.Buffer
	restore := stdioErrStream.push(destination{kind: destBuffer, w: &buf})
	smtpStderrLogger("10.0.0.1:2525", "MAIL", "", true, 500*time.Microsecond)
	smtpStderrLogger("10.0.0.1:2525", "RCPT", "user@example.com", false, 250*time.Microsecond)
	restore()

	got := buf.String()
	if !strings.Contains(got, "MAIL") || !strings.Contains(got, "ACCEPTED") {
		t.Errorf("expected accepted MAIL stage; got %q", got)
	}
	if !strings.Contains(got, "RCPT") || !strings.Contains(got, "user@example.com") || !strings.Contains(got, "REJECTED") {
		t.Errorf("expected rejected RCPT stage with detail; got %q", got)
	}
}

// runServe's FAIL line is on stderr today (unlike default mode's stdout
// FAIL) and must stay there, just routed through the registry.
func TestRunServe_FailRoutesToStderrRegistry(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bad.ts")
	if err := os.WriteFile(script, []byte(`throw new Error("boom");`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, restore := withCapturedStdio(t)
	defer restore()
	if code := runServe([]string{script}); code == exitOK {
		t.Fatalf("expected non-OK exit code")
	}
	if !strings.Contains(errb.String(), "FAIL ") {
		t.Fatalf("expected FAIL line on stderr registry, got %q", errb.String())
	}
}

// serveReadyWriter is set to stdioOut() for the run's duration; the READY
// line a listener binding writes through it must land on the stdout
// registry stream, so a served script's own runtime.stdout redirect also
// catches its own readiness line.
func TestRunServe_ReadyLineRoutesToStdoutRegistry(t *testing.T) {
	port := freePort(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "srv.ts")
	src := fmt.Sprintf(`
const srv = await server.http.listen({ port: %d, routes: { "GET /": (req, res) => res.text("ok") } });
await srv.close();
`, port)
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, restore := withCapturedStdio(t)
	defer restore()
	if code := runServe([]string{script}); code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}
	if !strings.Contains(out.String(), "READY listening on tcp/") {
		t.Fatalf("expected READY line on stdout registry, got %q", out.String())
	}
}
