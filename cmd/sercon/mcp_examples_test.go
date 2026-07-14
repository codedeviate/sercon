package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPExamples drives the four client-DEPENDENT advanced-example scripts
// (mcp-sampling.ts, mcp-elicit.ts, mcp-roots.ts, mcp-oauth.ts). Unlike Task
// 6's client-independent cookbook scripts (mcp-toolbox.ts and friends, which
// are their own client and self-test over raw JSON-RPC — see
// examples/scripts/mcp-server-http.ts's header comment for why that works),
// these four need a real MCP client wielding a capability the script itself
// can't fake: an LLM to sample from, a human to elicit from, a filesystem
// root list, or an OAuth bearer token. So they are NOT added to DEMO_SCRIPTS
// / make demo — this file is their coverage instead, following the same
// subprocess+CommandTransport pattern cmd/sercon/mcp_stdio_test.go's
// TestMCPStdio established for mcp-server-stdio.ts.
//
// The sercon binary is built once here and reused by every subtest below
// (only the Sampling/Elicit/Roots subtests need stdio, so only those are
// skipped on windows — see mcp_stdio_redirect_windows.go — while OAuth,
// which uses the Streamable HTTP transport, still runs there).
func TestMCPExamples(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "sercon-mcp-examples-test")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(build.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build sercon: %v\n%s", err, out)
	}

	t.Run("Sampling", func(t *testing.T) { testMCPExampleSampling(t, bin) })
	t.Run("Elicit", func(t *testing.T) { testMCPExampleElicit(t, bin) })
	t.Run("Roots", func(t *testing.T) { testMCPExampleRoots(t, bin) })
	t.Run("OAuth", func(t *testing.T) { testMCPExampleOAuth(t, bin) })
}

// mcpExampleFixture resolves one of this task's example scripts to an
// absolute path, mirroring TestMCPStdio's fixture lookup.
func mcpExampleFixture(t *testing.T, name string) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "examples", "scripts", name))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// syncBuffer is a mutex-guarded bytes.Buffer safe to use as an exec.Cmd's
// Stderr while the test goroutine concurrently polls String() — exec.Cmd
// starts its own goroutine copying the child's stderr pipe into whatever
// io.Writer is assigned, so a plain bytes.Buffer would race under `-race`
// against the roots subtest's poll loop (see testMCPExampleRoots).
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// testMCPExampleSampling drives mcp-sampling.ts as a subprocess over
// mcp.CommandTransport, connecting an SDK client whose CreateMessageHandler
// stands in for "the client's LLM" (setting it is what makes the client
// advertise the sampling capability — see client.go, and
// mcp_phase3_test.go's TestMCPSample for the in-memory-transport version of
// this same round trip). Asserts the tool's returned JSON reflects the
// canned CreateMessageResult verbatim: content.text -> summary, plus
// model/stopReason/role, proving ctx.sample really reached this handler and
// its result really made it back into JS.
func testMCPExampleSampling(t *testing.T, bin string) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio() is unsupported on windows (see mcp_stdio_redirect_windows.go)")
	}

	fixture := mcpExampleFixture(t, "mcp-sampling.ts")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, fixture)
	var stderr syncBuffer
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "sampling-test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		CreateMessageHandler: func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			return &mcp.CreateMessageResult{
				Content:    &mcp.TextContent{Text: "SUMMARY"},
				Model:      "test-model",
				StopReason: "endTurn",
				Role:       "assistant",
			}, nil
		},
	})
	sess, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect (initialize) failed: %v\nstderr:\n%s", err, stderr.String())
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{
		Name:      "summarize",
		Arguments: map[string]any{"text": "a long story about sercon"},
	})
	if err != nil {
		t.Fatalf("call tool summarize: %v\nstderr:\n%s", err, stderr.String())
	}
	if res.IsError {
		t.Fatalf("summarize returned isError: %#v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d: %#v", len(res.Content), res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}

	var got struct {
		Summary    string `json:"summary"`
		Model      string `json:"model"`
		StopReason string `json:"stopReason"`
		Role       string `json:"role"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &got); err != nil {
		t.Fatalf("unmarshal tool result %q: %v", tc.Text, err)
	}
	if got.Summary != "SUMMARY" {
		t.Errorf("summary = %q, want %q", got.Summary, "SUMMARY")
	}
	if got.Model != "test-model" {
		t.Errorf("model = %q, want %q", got.Model, "test-model")
	}
	if got.StopReason != "endTurn" {
		t.Errorf("stopReason = %q, want %q", got.StopReason, "endTurn")
	}
	if got.Role != "assistant" {
		t.Errorf("role = %q, want %q", got.Role, "assistant")
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("session close: %v", err)
	}
	_ = cmd.Wait()
}

// testMCPExampleElicit drives mcp-elicit.ts as a subprocess, connecting an
// SDK client whose ElicitationHandler stands in for "the human at the
// keyboard" confirming the deploy (setting it is what makes the client
// advertise the elicitation capability — see client.go, and
// mcp_phase3_test.go's TestMCPElicit for the in-memory-transport version).
// Asserts the tool honors the canned accept+confirm=true response by
// actually "deploying" (deployed:true, echoing the target), not just
// completing without error.
func testMCPExampleElicit(t *testing.T, bin string) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio() is unsupported on windows (see mcp_stdio_redirect_windows.go)")
	}

	fixture := mcpExampleFixture(t, "mcp-elicit.ts")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, fixture)
	var stderr syncBuffer
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "elicit-test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{
				Action:  "accept",
				Content: map[string]any{"confirm": true},
			}, nil
		},
	})
	sess, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect (initialize) failed: %v\nstderr:\n%s", err, stderr.String())
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{
		Name:      "deploy",
		Arguments: map[string]any{"target": "prod"},
	})
	if err != nil {
		t.Fatalf("call tool deploy: %v\nstderr:\n%s", err, stderr.String())
	}
	if res.IsError {
		t.Fatalf("deploy returned isError: %#v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d: %#v", len(res.Content), res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}

	var got struct {
		Deployed bool   `json:"deployed"`
		Target   string `json:"target"`
		Action   string `json:"action"`
	}
	if err := json.Unmarshal([]byte(tc.Text), &got); err != nil {
		t.Fatalf("unmarshal tool result %q: %v", tc.Text, err)
	}
	if !got.Deployed {
		t.Errorf("deployed = false, want true (elicit response was accept+confirm=true): %+v", got)
	}
	if got.Target != "prod" {
		t.Errorf("target = %q, want %q", got.Target, "prod")
	}
	if got.Action != "accept" {
		t.Errorf("action = %q, want %q", got.Action, "accept")
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("session close: %v", err)
	}
	_ = cmd.Wait()
}

// testMCPExampleRoots drives mcp-roots.ts as a subprocess: client.AddRoots
// is called BEFORE Connect (pre-seeding roots for the tool's first
// ctx.roots() call), then again AFTER Connect to trigger a
// notifications/roots/list_changed notification that the script's
// srv.onRootsChanged hook turns into a log line on stderr — mirroring
// mcp_phase3_test.go's TestMCPRoots / TestMCPOnRootsChanged, but over a real
// subprocess+stdio transport instead of an in-memory one, and observing the
// onRootsChanged side effect via stderr instead of a Go-side callback
// channel, since stdout must stay pure JSON-RPC (see mcp-roots.ts's header
// comment and mcp-server-stdio.ts's stdout/stderr split).
func testMCPExampleRoots(t *testing.T, bin string) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio() is unsupported on windows (see mcp_stdio_redirect_windows.go)")
	}

	fixture := mcpExampleFixture(t, "mcp-roots.ts")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, fixture)
	var stderr syncBuffer
	cmd.Stderr = &stderr

	client := mcp.NewClient(&mcp.Implementation{Name: "roots-test-client", Version: "0.0.0"}, nil)
	client.AddRoots(&mcp.Root{URI: "file:///b"}, &mcp.Root{URI: "file:///a"})
	sess, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatalf("connect (initialize) failed: %v\nstderr:\n%s", err, stderr.String())
	}

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "listRoots"})
	if err != nil {
		t.Fatalf("call tool listRoots: %v\nstderr:\n%s", err, stderr.String())
	}
	if res.IsError {
		t.Fatalf("listRoots returned isError: %#v", res.Content)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d: %#v", len(res.Content), res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("want TextContent, got %#v", res.Content[0])
	}
	const wantInitial = `["file:///a","file:///b"]`
	if tc.Text != wantInitial {
		t.Errorf("listRoots result = %q, want %q", tc.Text, wantInitial)
	}

	// Adding a third root post-connect fires notifications/roots/list_changed;
	// the script's onRootsChanged hook re-lists and logs the sorted uris.
	client.AddRoots(&mcp.Root{URI: "file:///changed"})

	deadline := time.Now().Add(10 * time.Second)
	for {
		if strings.Contains(stderr.String(), "file:///changed") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for onRootsChanged log line; stderr so far:\n%s", stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !strings.Contains(stderr.String(), `roots changed: ["file:///a","file:///b","file:///changed"]`) {
		t.Errorf("onRootsChanged log line missing expected sorted root set; stderr:\n%s", stderr.String())
	}

	if err := sess.Close(); err != nil {
		t.Fatalf("session close: %v", err)
	}
	_ = cmd.Wait()
}

// testMCPExampleOAuth drives mcp-oauth.ts as a subprocess. Unlike the three
// stdio-based subtests above, this script binds a FIXED port (39050, see
// the script's header comment for why) and never exits on its own — its
// srv.listen(...) HoldRun keeps the process alive so an external client can
// connect at any time, mirroring how a real always-on OAuth-protected MCP
// server stays up until its operator stops it. So this subtest starts the
// process directly (no CommandTransport — that owns stdin/stdout framing
// this HTTP-only script doesn't use), polls the fixed port until the
// listener accepts connections, then kills the process when done.
//
// Assertions mirror cmd/sercon/mcp_auth_test.go's TestMCPAuth (the inline
// test.serve() version of the same auth wiring): no token and a bad token
// both 401 (the no-token case's WWW-Authenticate references the metadata
// URL), the metadata endpoint serves RFC 9728 JSON, and a good token lets a
// real SDK client over StreamableClientTransport complete initialize +
// tools/call.
func testMCPExampleOAuth(t *testing.T, bin string) {
	const (
		baseURL     = "http://127.0.0.1:39050"
		mcpURL      = baseURL + "/mcp"
		metadataURL = baseURL + "/.well-known/oauth-protected-resource"
	)

	fixture := mcpExampleFixture(t, "mcp-oauth.ts")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, fixture)
	var stderr syncBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start mcp-oauth.ts: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	// Poll the fixed port until the listener is accepting connections — the
	// script has no way to hand an OS-assigned port back to this test (see
	// the script's header comment), so a fixed port + poll is the simplest
	// reliable readiness signal.
	deadline := time.Now().Add(10 * time.Second)
	for {
		resp, err := http.Get(metadataURL)
		if err == nil {
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for mcp-oauth.ts to listen on %s: %v\nstderr:\n%s", baseURL, err, stderr.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	// (a) no token -> 401 with WWW-Authenticate referencing the metadata URL.
	{
		resp, err := http.Get(mcpURL)
		if err != nil {
			t.Fatalf("no-token request: %v", err)
		}
		www := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("no-token: want 401, got %d", resp.StatusCode)
		}
		if !strings.HasPrefix(www, "Bearer") || !strings.Contains(www, "resource_metadata=") {
			t.Fatalf("no-token: WWW-Authenticate missing Bearer/resource_metadata: %q", www)
		}
		if !strings.Contains(www, metadataURL) {
			t.Fatalf("no-token: WWW-Authenticate does not reference metadata URL %q: %q", metadataURL, www)
		}
	}

	// (b) bad token -> 401.
	{
		req, _ := http.NewRequest(http.MethodGet, mcpURL, nil)
		req.Header.Set("Authorization", "Bearer nope")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("bad-token request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("bad-token: want 401, got %d", resp.StatusCode)
		}
	}

	// (c) metadata endpoint -> 200 JSON, matching the resourceMetadata the
	// script registered.
	{
		resp, err := http.Get(metadataURL)
		if err != nil {
			t.Fatalf("metadata request: %v", err)
		}
		var meta map[string]any
		derr := json.NewDecoder(resp.Body).Decode(&meta)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("metadata: want 200, got %d", resp.StatusCode)
		}
		if derr != nil {
			t.Fatalf("metadata: decode: %v", derr)
		}
		if _, ok := meta["resource"].(string); !ok || meta["resource"] == "" {
			t.Errorf("metadata: missing resource: %#v", meta)
		}
		as, ok := meta["authorization_servers"].([]any)
		if !ok || len(as) != 1 || as[0] != "https://auth.example.com" {
			t.Errorf("metadata: unexpected authorization_servers: %#v", meta["authorization_servers"])
		}
		scopes, ok := meta["scopes_supported"].([]any)
		if !ok || len(scopes) != 1 || scopes[0] != "mcp" {
			t.Errorf("metadata: unexpected scopes_supported: %#v", meta["scopes_supported"])
		}
	}

	// (d) good token -> MCP initialize + tools/call succeeds, proving verify
	// ran and accepted the demo's hardcoded token.
	{
		hc := &http.Client{Transport: bearerRT{token: "good-token", base: http.DefaultTransport}}
		client := mcp.NewClient(&mcp.Implementation{Name: "oauth-test-client", Version: "0.0.0"}, nil)
		transport := &mcp.StreamableClientTransport{Endpoint: mcpURL, HTTPClient: hc}
		sess, err := client.Connect(ctx, transport, nil)
		if err != nil {
			t.Fatalf("connect (initialize) with good token: %v", err)
		}
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "add", Arguments: map[string]any{"a": 2, "b": 3}})
		if err != nil {
			t.Fatalf("call tool add: %v", err)
		}
		if res.IsError {
			t.Fatalf("add: unexpected isError, content=%v", res.Content)
		}
		tc, ok := res.Content[0].(*mcp.TextContent)
		if !ok || tc.Text != "5" {
			t.Fatalf("add: want TextContent \"5\", got %#v", res.Content[0])
		}
		if err := sess.Close(); err != nil {
			t.Fatalf("session close: %v", err)
		}
	}
}
