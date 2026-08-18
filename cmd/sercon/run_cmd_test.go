package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunRun_NoScript surfaces the usage-error path.
func TestRunRun_NoScript(t *testing.T) {
	if code := runRun([]string{}); code != exitUsage {
		t.Fatalf("expected exitUsage, got %d", code)
	}
}

// TestRunRun_PassesArgs confirms that, unlike the default multi-script mode,
// `run` treats the first positional as the script and the rest as the
// script's runtime.argv[2:] — the behavior a `#!/usr/bin/env -S sercon run`
// shebang relies on. A leading shebang line on the script is also stripped.
func TestRunRun_PassesArgs(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "argcheck.ts")
	src := "#!/usr/bin/env -S sercon run\n" +
		`const got = runtime.argv.slice(2).join(",");` + "\n" +
		`if (got !== "hello,world") throw new Error("argv[2:]=" + got);` + "\n"
	if err := os.WriteFile(script, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}

	if code := runRun([]string{script, "hello", "world"}); code != exitOK {
		t.Fatalf("expected exitOK with correct args, got %d", code)
	}
	// Wrong args → the script throws → exitThrow, proving the args really
	// reached the script rather than being ignored.
	if code := runRun([]string{script, "nope"}); code != exitThrow {
		t.Fatalf("expected exitThrow with wrong args, got %d", code)
	}
}

// TestRunRun_DoubleDashIsLiteralArg confirms `run` does NOT treat a "--"
// after the script as a separator (the default multi-script mode does) — it's
// passed through as a literal runtime.argv entry.
func TestRunRun_DoubleDashIsLiteralArg(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "args.ts")
	src := `const a = runtime.argv.slice(2).join(",");` + "\n" +
		`if (a !== "--,x") throw new Error("argv[2:]=" + a);` + "\n"
	if err := os.WriteFile(script, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runRun([]string{script, "--", "x"}); code != exitOK {
		t.Fatalf(`expected exitOK ("--" should be a literal arg), got %d`, code)
	}
}

// TestRunRun_FlagsBeforeScript confirms sercon flags are parsed when they
// precede the script, while tokens after the script are script args (not
// flags).
func TestRunRun_FlagsBeforeScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "noop.ts")
	if err := os.WriteFile(script, []byte(`runtime.log("ok");`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runRun([]string{"-timeout", "5s", script}); code != exitOK {
		t.Fatalf("expected exitOK with a leading flag, got %d", code)
	}
}

// -v wires engOpts.Verbose to the stderr registry stream (not a bare
// os.Stderr), so a script's own runtime.stderr redirect also catches the
// engine's transpile trace.
func TestRunRun_VerboseTraceRoutesToStderrRegistry(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "ok.ts")
	if err := os.WriteFile(script, []byte(`runtime.log("x");`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errb, restore := withCapturedStdio(t)
	defer restore()
	if code := runRun([]string{"-v", script}); code != exitOK {
		t.Fatalf("expected exitOK, got %d", code)
	}
	if !strings.Contains(errb.String(), "transpile entry") {
		t.Fatalf("expected transpile trace on stderr registry, got %q", errb.String())
	}
}

// The FAIL line stays on stdout (mirroring main.go's default mode) but must
// go through the registry rather than a bare fmt.Printf to the real stdout.
func TestRunRun_FailRoutesToStdoutRegistry(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "bad.ts")
	if err := os.WriteFile(script, []byte(`throw new Error("boom");`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, restore := withCapturedStdio(t)
	defer restore()
	if code := runRun([]string{script}); code == exitOK {
		t.Fatalf("expected non-OK exit code")
	}
	if !strings.Contains(out.String(), "FAIL ") {
		t.Fatalf("expected FAIL line on stdout registry, got %q", out.String())
	}
}
