package main

import (
	"bytes"
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/codedeviate/sercon/pkg/scriptengine"
	"github.com/dop251/goja"
)

// shellArg packages a JS-side cmd into a goja.Value so we can drive
// execShell through its real signature without spinning up a runtime
// per test. We hand it the export-shaped value directly.
type shellCall struct {
	cmd  any
	opts map[string]any
}

func runShell(t *testing.T, c shellCall) (map[string]any, error) {
	t.Helper()
	vm := goja.New()
	args := []goja.Value{vm.ToValue(c.cmd)}
	if c.opts != nil {
		args = append(args, vm.ToValue(c.opts))
	}
	call := goja.FunctionCall{Arguments: args}
	return execShell(context.Background(), call)
}

// String cmd routes through the host shell so shell metacharacters work.
// Returns exit 0 and stdout containing the echoed text.
func TestExecShell_StringCmdViaShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("string-cmd test assumes /bin/sh")
	}
	out, err := runShell(t, shellCall{cmd: "echo hi-from-shell"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out["exitCode"].(int) != 0 {
		t.Errorf("exitCode: %v", out["exitCode"])
	}
	if !strings.Contains(out["stdout"].(string), "hi-from-shell") {
		t.Errorf("stdout: %q", out["stdout"])
	}
	if out["success"].(bool) != true {
		t.Errorf("success: %v", out["success"])
	}
}

// Array cmd bypasses the shell — pass argv directly. Useful when args
// might contain glob characters that you don't want re-expanded.
func TestExecShell_ArgvCmd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("argv test assumes /bin/echo")
	}
	out, err := runShell(t, shellCall{cmd: []any{"/bin/echo", "literal *"}})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := strings.TrimSpace(out["stdout"].(string)); got != "literal *" {
		t.Errorf("stdout: %q (want literal *)", got)
	}
}

// Non-zero exit must not throw — it surfaces via exitCode + success:false.
// Distinct from spawn failures, which do throw.
func TestExecShell_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh -c")
	}
	out, err := runShell(t, shellCall{cmd: "exit 7"})
	if err != nil {
		t.Fatalf("non-zero exit should not throw: %v", err)
	}
	if out["exitCode"].(int) != 7 {
		t.Errorf("exitCode: %v (want 7)", out["exitCode"])
	}
	if out["success"].(bool) != false {
		t.Errorf("success should be false on non-zero exit")
	}
}

// stdin is piped to the subprocess. `cat` echoes whatever we send.
func TestExecShell_StdinPipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/cat")
	}
	out, err := runShell(t, shellCall{
		cmd:  []any{"/bin/cat"},
		opts: map[string]any{"stdin": "piped-content\n"},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := out["stdout"].(string); got != "piped-content\n" {
		t.Errorf("stdout: %q", got)
	}
}

// Custom env vars get merged on top of os.Environ. The subprocess sees
// both the parent env and the overrides.
func TestExecShell_EnvOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	out, err := runShell(t, shellCall{
		cmd:  "echo $SERCON_TEST_VAR",
		opts: map[string]any{"env": map[string]any{"SERCON_TEST_VAR": "merged-ok"}},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(out["stdout"].(string), "merged-ok") {
		t.Errorf("stdout: %q (want merged-ok)", out["stdout"])
	}
}

// cwd directs the subprocess to a specific directory.
func TestExecShell_CwdRespected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/pwd")
	}
	dir := t.TempDir()
	out, err := runShell(t, shellCall{
		cmd:  []any{"/bin/pwd"},
		opts: map[string]any{"cwd": dir},
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := strings.TrimSpace(out["stdout"].(string)); got != dir {
		// macOS resolves /tmp to /private/tmp; allow EvalSymlinks-style
		// suffix match in either direction.
		if !strings.HasSuffix(got, dir) && !strings.HasSuffix(dir, got) {
			t.Errorf("stdout: %q (want %q)", got, dir)
		}
	}
}

// Timeout kills the subprocess and the binding throws (the context
// deadline counts as a spawn-side failure, not a graceful exit).
func TestExecShell_TimeoutKills(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sleep")
	}
	start := time.Now()
	_, err := runShell(t, shellCall{
		cmd:  "sleep 5",
		opts: map[string]any{"timeout": int64(150)}, // ms
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}

// Empty / undefined cmd is a clear caller error.
func TestExecShell_InputValidation(t *testing.T) {
	if _, err := runShell(t, shellCall{cmd: ""}); err == nil {
		t.Error("empty string cmd should error")
	}
	if _, err := runShell(t, shellCall{cmd: []any{}}); err == nil {
		t.Error("empty argv should error")
	}
}

// With opts.pane, stdout streams into the pane (verified via the
// fallback writer). The returned shell result's stdout/stderr are
// empty because the data was streamed, not captured.
func TestExecShell_PaneStreamsStdout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/echo")
	}
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerExampleAPI(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
api.tui.layout({name: "out"});
const p = api.tui.pane("out");
const r = await api.exec.shell(["/bin/echo", "hello-from-shell"], { pane: p });
api.assert.equal(r.exitCode, 0);
api.assert.equal(r.stdout, "");
api.assert.equal(r.stderr, "");
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	got := captured.String()
	if !strings.Contains(got, "[out] hello-from-shell\n") {
		t.Errorf("expected streamed line in pane; got:\n%s", got)
	}
}

// pane: also accepts a string (pane name) for ergonomics.
func TestExecShell_PaneAcceptsStringName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/echo")
	}
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerExampleAPI(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		_, err := eng.Run(context.Background(), "run.ts", `
api.tui.layout({name: "out"});
const r = await api.exec.shell(["/bin/echo", "via-name"], { pane: "out" });
api.assert.equal(r.exitCode, 0);
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	if !strings.Contains(captured.String(), "[out] via-name\n") {
		t.Errorf("got: %q", captured.String())
	}
}

// With opts.pane, exec.Cmd's stdout and stderr copy goroutines both
// write to the SAME io.Writer (pane.AsWriter()). The adapter must
// serialise concurrent Write calls or a race will fire under -race.
// /bin/sh's printf to stderr below ensures both streams produce
// output to drive the concurrency.
func TestExecShell_PaneSerialisesStdoutAndStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := registerExampleAPI(eng); err != nil {
		t.Fatal(err)
	}
	var captured bytes.Buffer
	withTestStdout(&captured, func() {
		// Loop 50 times alternating stdout / stderr to get repeated
		// concurrent Write calls from the two copy goroutines.
		_, err := eng.Run(context.Background(), "run.ts", `
api.tui.layout({name: "out"});
const p = api.tui.pane("out");
const r = await api.exec.shell(
  ["/bin/sh", "-c", "for i in $(seq 1 50); do printf 'out %s\\n' $i; printf 'err %s\\n' $i 1>&2; done"],
  { pane: p },
);
api.assert.equal(r.exitCode, 0);
`)
		if err != nil {
			t.Fatalf("run: %v", err)
		}
	})
	got := captured.String()
	// We don't assert exact line ordering — stdout and stderr
	// interleave non-deterministically. Just confirm we got the
	// expected line count, which proves no Write was dropped.
	// 50 stdout lines + 50 stderr lines = 100 lines total.
	lines := 0
	for _, c := range got {
		if c == '\n' {
			lines++
		}
	}
	if lines != 100 {
		t.Errorf("expected 100 lines (50 out + 50 err); got %d. Output:\n%s", lines, got)
	}
}
