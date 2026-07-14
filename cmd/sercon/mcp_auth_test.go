package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// bearerRT is a test RoundTripper that injects a fixed bearer token on every
// request, so the SDK streamable client authenticates against the OAuth
// resource-server middleware.
type bearerRT struct {
	token string
	base  http.RoundTripper
}

func (b bearerRT) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.base.RoundTrip(r)
}

// TestMCPAuth drives srv.listen({ auth }) end to end: a script binds a
// Streamable HTTP transport guarded by an OAuth bearer verifier that accepts
// only the token "good", advertising protected-resource metadata. The Go side
// then asserts:
//   - no token          -> 401 + WWW-Authenticate: Bearer referencing the
//     resource_metadata URL;
//   - bad token         -> 401;
//   - good token        -> the MCP initialize + tools/call succeeds (proving
//     verify ran and its identity/scopes were honored);
//   - GET metadata URL  -> 200 JSON with resource / authorization_servers /
//     scopes_supported.
func TestMCPAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})

	var ms *mcpServer
	urlCh := make(chan string, 1)
	done := make(chan struct{})

	if err := eng.RegisterNamespaceFactory("test", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return map[string]any{
			"serve": func(call goja.FunctionCall) goja.Value {
				ms = &mcpServer{
					eng: eng, vm: vm, loop: loop,
					srv: mcp.NewServer(&mcp.Implementation{Name: "t", Version: "1.0.0"}, nil),
				}
				return ms.handle(vm)
			},
			"notifyURL": func(u string) goja.Value {
				urlCh <- u
				return goja.Undefined()
			},
			"waitClose": func() goja.Value {
				p, resolve, _ := vm.NewPromise()
				go func() {
					<-done
					loop.RunOnLoop(func(*goja.Runtime) { _ = resolve(goja.Undefined()) })
				}()
				return vm.ToValue(p)
			},
		}
	}); err != nil {
		t.Fatal(err)
	}

	runErr := make(chan error, 1)
	go func() {
		_, err := eng.Run(ctx, "auth.ts", `
const srv = test.serve();
srv.tool({
	name: "add",
	inputSchema: { type: "object" },
	handler: (args) => String(args.a + args.b),
});
const h = await srv.listen({
	port: 0,
	auth: {
		verify: (token, req) => token === "good" ? { subject: "u1", scopes: ["mcp"] } : null,
		resourceMetadata: {
			authorizationServers: ["https://auth.example.com"],
			scopesSupported: ["mcp"],
			resourceName: "Test MCP",
		},
		scopes: ["mcp"],
	},
});
test.notifyURL(h.url);
await test.waitClose();
await h.close();
`)
		runErr <- err
	}()

	var url string
	select {
	case url = <-urlCh:
	case <-ctx.Done():
		close(done)
		t.Fatal("timed out waiting for listen() URL")
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:") || !strings.HasSuffix(url, "/mcp") {
		close(done)
		t.Fatalf("unexpected handle url: %q", url)
	}
	base := strings.TrimSuffix(url, "/mcp")
	metadataURL := base + "/.well-known/oauth-protected-resource"

	fail := func(format string, args ...any) {
		close(done)
		t.Fatalf(format, args...)
	}

	// (a) no token -> 401 with WWW-Authenticate referencing the metadata URL.
	{
		resp, err := http.Get(url)
		if err != nil {
			fail("no-token request: %v", err)
		}
		body := resp.Header.Get("WWW-Authenticate")
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			fail("no-token: want 401, got %d", resp.StatusCode)
		}
		if !strings.HasPrefix(body, "Bearer") || !strings.Contains(body, "resource_metadata=") {
			fail("no-token: WWW-Authenticate missing Bearer/resource_metadata: %q", body)
		}
		if !strings.Contains(body, metadataURL) {
			fail("no-token: WWW-Authenticate does not reference metadata URL %q: %q", metadataURL, body)
		}
	}

	// (b) bad token -> 401.
	{
		req, _ := http.NewRequest(http.MethodGet, url, nil)
		req.Header.Set("Authorization", "Bearer nope")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			fail("bad-token request: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			fail("bad-token: want 401, got %d", resp.StatusCode)
		}
	}

	// (d) metadata endpoint -> 200 JSON.
	{
		resp, err := http.Get(metadataURL)
		if err != nil {
			fail("metadata request: %v", err)
		}
		var meta map[string]any
		dec := json.NewDecoder(resp.Body)
		derr := dec.Decode(&meta)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			fail("metadata: want 200, got %d", resp.StatusCode)
		}
		if derr != nil {
			fail("metadata: decode: %v", derr)
		}
		if _, ok := meta["resource"].(string); !ok || meta["resource"] == "" {
			fail("metadata: missing resource: %#v", meta)
		}
		as, ok := meta["authorization_servers"].([]any)
		if !ok || len(as) == 0 {
			fail("metadata: missing authorization_servers: %#v", meta)
		}
		if _, ok := meta["scopes_supported"].([]any); !ok {
			fail("metadata: missing scopes_supported: %#v", meta)
		}
	}

	// (c) good token -> MCP initialize + tools/call succeeds.
	{
		hc := &http.Client{Transport: bearerRT{token: "good", base: http.DefaultTransport}}
		client := mcp.NewClient(&mcp.Implementation{Name: "auth-test-client", Version: "0.0.0"}, nil)
		transport := &mcp.StreamableClientTransport{Endpoint: url, HTTPClient: hc}
		sess, err := client.Connect(ctx, transport, nil)
		if err != nil {
			fail("connect (initialize) with good token: %v", err)
		}
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "add", Arguments: map[string]any{"a": 2, "b": 3}})
		if err != nil {
			fail("call tool add: %v", err)
		}
		if res.IsError {
			fail("add: unexpected isError, content=%v", res.Content)
		}
		tc, ok := res.Content[0].(*mcp.TextContent)
		if !ok || tc.Text != "5" {
			fail("add: want TextContent \"5\", got %#v", res.Content[0])
		}
		if err := sess.Close(); err != nil {
			fail("session close: %v", err)
		}
	}

	close(done)
	if err := <-runErr; err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestMCPAuth_NonFunctionVerifyThrows asserts the auth config is validated:
// an `auth` block whose `verify` is not a function throws synchronously.
func TestMCPAuth_NonFunctionVerifyThrows(t *testing.T) {
	if _, err := runScript(t, `
		const srv = mcp.serve({ name: "t", version: "1.0.0" });
		srv.listen({ port: 0, auth: { verify: "not-a-function" } });
	`); err == nil {
		t.Fatal("expected throw for non-function verify")
	}
}
