package main

import (
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

// decodeStreamResult unmarshals a JSON string captured from a stream test.
func decodeStreamResult(t *testing.T, got any) map[string]any {
	t.Helper()
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected JSON string capture, got %T (%v)", got, got)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("unmarshal %q: %v", s, err)
	}
	return m
}

// TestExecStream_StdoutLines streams two echoed lines and confirms each is
// delivered on "stdout" in order, and the Promise resolves exitCode 0.
func TestExecStream_StdoutLines(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	got := runSocketScript(t, `
		const lines = [];
		const r = await services.exec.stream("echo one; echo two", (line, stream) => {
			lines.push(stream + ":" + line);
		});
		__capture(JSON.stringify({ lines, exitCode: r.exitCode, success: r.success }));
	`)
	m := decodeStreamResult(t, got)
	lines, _ := m["lines"].([]any)
	if len(lines) != 2 || lines[0] != "stdout:one" || lines[1] != "stdout:two" {
		t.Errorf("lines: got %v want [stdout:one stdout:two]", lines)
	}
	if m["exitCode"] != float64(0) || m["success"] != true {
		t.Errorf("result: got exitCode=%v success=%v want 0/true", m["exitCode"], m["success"])
	}
}

// TestExecStream_StderrRouting confirms a line written to fd 2 is tagged
// "stderr".
func TestExecStream_StderrRouting(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	got := runSocketScript(t, `
		const lines = [];
		await services.exec.stream("echo err 1>&2", (line, stream) => {
			lines.push(stream + ":" + line);
		});
		__capture(JSON.stringify({ lines }));
	`)
	m := decodeStreamResult(t, got)
	lines, _ := m["lines"].([]any)
	if len(lines) != 1 || lines[0] != "stderr:err" {
		t.Errorf("lines: got %v want [stderr:err]", lines)
	}
}

// TestExecStream_NonZeroExit confirms a non-zero exit resolves (does not throw)
// with the real exit code and success:false, and still delivers output.
func TestExecStream_NonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh")
	}
	got := runSocketScript(t, `
		const lines = [];
		const r = await services.exec.stream("echo x; exit 3", (line) => { lines.push(line); });
		__capture(JSON.stringify({ lines, exitCode: r.exitCode, success: r.success }));
	`)
	m := decodeStreamResult(t, got)
	lines, _ := m["lines"].([]any)
	if len(lines) != 1 || lines[0] != "x" {
		t.Errorf("lines: got %v want [x]", lines)
	}
	if m["exitCode"] != float64(3) || m["success"] != false {
		t.Errorf("result: got exitCode=%v success=%v want 3/false", m["exitCode"], m["success"])
	}
}

// TestExecStream_PartialFinalLine confirms a final line without a trailing
// newline is still delivered.
func TestExecStream_PartialFinalLine(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh + printf")
	}
	got := runSocketScript(t, `
		const lines = [];
		await services.exec.stream("printf done", (line) => { lines.push(line); });
		__capture(JSON.stringify({ lines }));
	`)
	m := decodeStreamResult(t, got)
	lines, _ := m["lines"].([]any)
	if len(lines) != 1 || lines[0] != "done" {
		t.Errorf("lines: got %v want [done]", lines)
	}
}

// TestExecStream_SpawnFailureRejects confirms a bogus binary (argv form)
// rejects the Promise rather than resolving.
func TestExecStream_SpawnFailureRejects(t *testing.T) {
	got := runSocketScript(t, `
		let outcome;
		try {
			await services.exec.stream(["__sercon_no_such_binary__"], () => {});
			outcome = "resolved";
		} catch (e) {
			outcome = "rejected: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T (%v)", got, got)
	}
	if !strings.HasPrefix(s, "rejected:") {
		t.Errorf("expected rejection, got %q", s)
	}
}

// TestExecStream_OnLineNotFunction confirms a non-function second argument
// throws synchronously.
func TestExecStream_OnLineNotFunction(t *testing.T) {
	got := runSocketScript(t, `
		let outcome;
		try {
			services.exec.stream("echo hi", 123);
			outcome = "no-throw";
		} catch (e) {
			outcome = "threw: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T (%v)", got, got)
	}
	if !strings.Contains(s, "threw:") || !strings.Contains(s, "onLine") {
		t.Errorf("expected throw mentioning onLine, got %q", s)
	}
}

// TestExecStream_TimeoutRejects confirms opts.timeout kills the process tree
// and rejects the Promise (parity with exec.shell's timeout behaviour), and
// that the kill is prompt rather than waiting out the full sleep.
func TestExecStream_TimeoutRejects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sleep")
	}
	start := time.Now()
	got := runSocketScript(t, `
		let outcome;
		try {
			await services.exec.stream("sleep 5", () => {}, { timeout: 150 });
			outcome = "resolved";
		} catch (e) {
			outcome = "rejected: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	elapsed := time.Since(start)
	s, ok := got.(string)
	if !ok {
		t.Fatalf("expected string, got %T (%v)", got, got)
	}
	if !strings.HasPrefix(s, "rejected:") {
		t.Errorf("expected timeout rejection, got %q", s)
	}
	if elapsed > 3*time.Second {
		t.Errorf("timeout took too long: %v", elapsed)
	}
}
