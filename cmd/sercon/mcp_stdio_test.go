package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPStdio is the end-to-end gate for srv.stdio(): it builds the sercon
// binary, runs the mcp-server-stdio.ts fixture as a real subprocess, and drives
// it with the SDK client over a CommandTransport (which speaks JSON-RPC over the
// child's stdin/stdout).
//
// The fixture emits a console.log and a runtime.log BEFORE serving. If either
// leaked onto stdout it would land in the middle of the JSON-RPC stream and the
// client's initialize/tools-call handshake below would fail to parse — so a
// green initialize + a correct "5" result is itself proof that stdout stayed a
// pure JSON-RPC channel. As an extra assertion we capture the child's stderr and
// confirm both log lines were redirected there (not dropped).
func TestMCPStdio(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio() is unsupported on windows (see mcp_stdio_redirect_windows.go)")
	}

	bin := filepath.Join(t.TempDir(), "sercon-mcp-stdio-test")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(build.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sercon: %v\n%s", err, out)
	}

	fixture, err := filepath.Abs(filepath.Join("..", "..", "examples", "scripts", "mcp-server-stdio.ts"))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, fixture)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-test-client", Version: "0.0.0"}, nil)
	sess, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("client connect (initialize) failed — stdout may not be pure JSON-RPC: %v\nstderr:\n%s", err, stderr.String())
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{
		Name:      "add",
		Arguments: map[string]any{"a": 2, "b": 3},
	})
	if err != nil {
		t.Fatalf("call tool add: %v\nstderr:\n%s", err, stderr.String())
	}
	if res.IsError {
		t.Fatalf("tool add returned isError: %#v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d: %#v", len(res.Content), res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "5" {
		t.Fatalf("want TextContent \"5\", got %#v", res.Content[0])
	}

	// Closing the session closes the child's stdin, which unblocks srv.stdio()
	// and lets the process exit.
	if err := sess.Close(); err != nil {
		t.Fatalf("session close: %v", err)
	}
	_ = cmd.Wait()

	// The two log lines must have been redirected to stderr, proving they were
	// moved off stdout rather than dropped.
	errOut := stderr.String()
	if !strings.Contains(errOut, "debug line") {
		t.Errorf("expected console.log %q on stderr, got:\n%s", "debug line", errOut)
	}
	if !strings.Contains(errOut, "runtime line") {
		t.Errorf("expected runtime.log %q on stderr, got:\n%s", "runtime line", errOut)
	}
}
