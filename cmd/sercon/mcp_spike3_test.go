package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// TestMCPPhase3Spike pins the exact go-sdk v1.6.1 API surface Phase-3
// features will use: sampling (server->client CreateMessage), elicitation,
// roots, and the auth.RequireBearerToken HTTP middleware + oauthex protected
// resource metadata. Pure-Go, in-memory transport for the MCP subtests and a
// plain httptest server for the auth subtest — no goja involved.
//
// The critical subtest is "sampling (no-deadlock)": a server tool handler
// calls req.Session.CreateMessage, which blocks the tool's goroutine on a
// server->client round trip while the client's CreateMessageHandler answers.
// This proves the SDK's transport keeps servicing the connection (reads +
// dispatches the client's response) even while a request handler is
// mid-flight and blocked — see task-1-report.md for the internal mechanism
// (jsonrpc2.Async release) that makes this safe.
//
// Every signature below was checked against the real source under
// $(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.6.1
// and matched the brief with two corrections: the client-side elicitation
// handler field is ElicitationHandler (not ElicitHandler), and roots are
// registered via (*mcp.Client).AddRoots(...*mcp.Root), not a ClientOptions
// field. See task-1-report.md for the full trace.
func TestMCPPhase3Spike(t *testing.T) {
	ctx := context.Background()

	schema := map[string]any{"type": "object"}

	srv := mcp.NewServer(&mcp.Implementation{Name: "spike3", Version: "0.0.0"}, nil)

	// --- tool 1: sampling round trip (the no-deadlock proof) ---
	srv.AddTool(&mcp.Tool{
		Name:        "askModel",
		Description: "ask the client's model to answer via sampling",
		InputSchema: schema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// This call blocks the tool handler's goroutine on a server->client
		// request while the client's CreateMessageHandler (below) computes
		// and returns an answer. If the SDK's connection loop couldn't
		// service the transport concurrently with a blocked handler, the
		// client's response would never be read and this call would hang
		// forever.
		res, err := req.Session.CreateMessage(ctx, &mcp.CreateMessageParams{
			Messages: []*mcp.SamplingMessage{
				{Role: "user", Content: &mcp.TextContent{Text: "what is 6*7?"}},
			},
			MaxTokens:        100,
			SystemPrompt:     "You are a calculator.",
			ModelPreferences: &mcp.ModelPreferences{IntelligencePriority: 0.8},
			StopSequences:    []string{"STOP"},
			Temperature:      0.5,
			IncludeContext:   "none",
		})
		if err != nil {
			return nil, err
		}
		tc, ok := res.Content.(*mcp.TextContent)
		if !ok {
			return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{
				Text: "sampling result content is not TextContent",
			}}}, nil
		}
		payload, err := json.Marshal(map[string]string{
			"text":       tc.Text,
			"model":      res.Model,
			"stopReason": res.StopReason,
			"role":       string(res.Role),
		})
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}}}, nil
	})

	// --- tool 2: elicitation round trip ---
	srv.AddTool(&mcp.Tool{
		Name:        "confirm",
		Description: "ask the client's user to confirm an action via elicitation",
		InputSchema: schema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := req.Session.Elicit(ctx, &mcp.ElicitParams{
			Message: "Proceed with the operation?",
			RequestedSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"confirmed": map[string]any{"type": "boolean"}},
			},
			Mode: "form",
		})
		if err != nil {
			return nil, err
		}
		payload, err := json.Marshal(map[string]any{"action": res.Action, "content": res.Content})
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}}}, nil
	})

	// --- tool 3: roots round trip ---
	srv.AddTool(&mcp.Tool{
		Name:        "listRoots",
		Description: "list the client's configured roots",
		InputSchema: schema,
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		res, err := req.Session.ListRoots(ctx, &mcp.ListRootsParams{})
		if err != nil {
			return nil, err
		}
		var names []string
		for _, r := range res.Roots {
			names = append(names, r.Name+"|"+r.URI)
		}
		payload, err := json.Marshal(names)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(payload)}}}, nil
	})

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		// Setting CreateMessageHandler automatically advertises the sampling
		// capability (client.go: "Setting CreateMessageHandler to a non-nil
		// value automatically causes the client to advertise the sampling
		// capability").
		CreateMessageHandler: func(_ context.Context, req *mcp.CreateMessageRequest) (*mcp.CreateMessageResult, error) {
			if req.Params.SystemPrompt != "You are a calculator." {
				t.Errorf("CreateMessageHandler: SystemPrompt = %q, want %q", req.Params.SystemPrompt, "You are a calculator.")
			}
			if len(req.Params.Messages) != 1 {
				t.Errorf("CreateMessageHandler: got %d messages, want 1", len(req.Params.Messages))
			}
			return &mcp.CreateMessageResult{
				Content:    &mcp.TextContent{Text: "42"},
				Model:      "spike-model",
				StopReason: "endTurn",
				Role:       "assistant",
			}, nil
		},
		// Likewise ElicitationHandler automatically advertises elicitation.
		ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			if req.Params.Message != "Proceed with the operation?" {
				t.Errorf("ElicitationHandler: Message = %q, want %q", req.Params.Message, "Proceed with the operation?")
			}
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"confirmed": true}}, nil
		},
	})
	// Roots are registered on the client via AddRoots (there is no
	// ClientOptions field for a static root list); the default client
	// capabilities already advertise "roots" without any extra opt-in.
	client.AddRoots(
		&mcp.Root{URI: "file:///workspace", Name: "workspace"},
		&mcp.Root{URI: "file:///tmp", Name: "tmp"},
	)

	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer sess.Close()

	t.Run("sampling (no-deadlock)", func(t *testing.T) {
		type callResult struct {
			res *mcp.CallToolResult
			err error
		}
		done := make(chan callResult, 1)
		go func() {
			res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "askModel"})
			done <- callResult{res, err}
		}()

		var cr callResult
		select {
		case cr = <-done:
			// proceed with assertions below
		case <-time.After(5 * time.Second):
			t.Fatal("deadlock: CallTool(askModel) did not return within 5s while the tool handler awaited a server->client CreateMessage round trip")
		}

		if cr.err != nil {
			t.Fatalf("call tool: %v", cr.err)
		}
		if cr.res.IsError {
			t.Fatalf("askModel reported an error result: %+v", cr.res)
		}
		if len(cr.res.Content) != 1 {
			t.Fatalf("want 1 content item, got %d", len(cr.res.Content))
		}
		tc, ok := cr.res.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("want TextContent, got %#v", cr.res.Content[0])
		}
		var got struct {
			Text       string `json:"text"`
			Model      string `json:"model"`
			StopReason string `json:"stopReason"`
			Role       string `json:"role"`
		}
		if err := json.Unmarshal([]byte(tc.Text), &got); err != nil {
			t.Fatalf("unmarshal tool result payload: %v", err)
		}
		if got.Text != "42" || got.Model != "spike-model" || got.StopReason != "endTurn" || got.Role != "assistant" {
			t.Errorf("sampling round trip = %+v, want text=42 model=spike-model stopReason=endTurn role=assistant", got)
		}
	})

	t.Run("elicitation", func(t *testing.T) {
		done := make(chan struct {
			res *mcp.CallToolResult
			err error
		}, 1)
		go func() {
			res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "confirm"})
			done <- struct {
				res *mcp.CallToolResult
				err error
			}{res, err}
		}()

		var cr struct {
			res *mcp.CallToolResult
			err error
		}
		select {
		case cr = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for CallTool(confirm) — elicitation round trip did not complete")
		}
		if cr.err != nil {
			t.Fatalf("call tool: %v", cr.err)
		}
		if cr.res.IsError {
			t.Fatalf("confirm reported an error result: %+v", cr.res)
		}
		tc, ok := cr.res.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("want TextContent, got %#v", cr.res.Content[0])
		}
		var got struct {
			Action  string         `json:"action"`
			Content map[string]any `json:"content"`
		}
		if err := json.Unmarshal([]byte(tc.Text), &got); err != nil {
			t.Fatalf("unmarshal tool result payload: %v", err)
		}
		if got.Action != "accept" {
			t.Errorf("elicit action = %q, want %q", got.Action, "accept")
		}
		if confirmed, _ := got.Content["confirmed"].(bool); !confirmed {
			t.Errorf("elicit content = %+v, want confirmed=true", got.Content)
		}
	})

	t.Run("roots", func(t *testing.T) {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "listRoots"})
		if err != nil {
			t.Fatalf("call tool: %v", err)
		}
		if res.IsError {
			t.Fatalf("listRoots reported an error result: %+v", res)
		}
		tc, ok := res.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("want TextContent, got %#v", res.Content[0])
		}
		var names []string
		if err := json.Unmarshal([]byte(tc.Text), &names); err != nil {
			t.Fatalf("unmarshal tool result payload: %v", err)
		}
		// ListRoots doesn't guarantee order (it's backed by an internal
		// featureSet), so compare as sets rather than a fixed sequence.
		want := map[string]bool{"workspace|file:///workspace": false, "tmp|file:///tmp": false}
		if len(names) != len(want) {
			t.Fatalf("roots = %v, want (in any order) %v", names, want)
		}
		for _, n := range names {
			if _, ok := want[n]; !ok {
				t.Errorf("unexpected root %q, want one of %v", n, want)
				continue
			}
			want[n] = true
		}
		for n, seen := range want {
			if !seen {
				t.Errorf("missing expected root %q in %v", n, names)
			}
		}
	})

	t.Run("oauth bearer + protected resource metadata", func(t *testing.T) {
		const goodToken = "good-token"

		verifier := auth.TokenVerifier(func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
			if token != goodToken {
				return nil, auth.ErrInvalidToken
			}
			return &auth.TokenInfo{
				Scopes:     []string{"mcp:read"},
				Expiration: time.Now().Add(time.Hour),
				UserID:     "user-123",
			}, nil
		})

		const resourceMetadataURL = "https://example.com/.well-known/oauth-protected-resource"

		protected := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ti := auth.TokenInfoFromContext(r.Context())
			if ti == nil || ti.UserID != "user-123" {
				t.Errorf("protected handler: TokenInfo = %+v, want UserID=user-123", ti)
			}
			w.WriteHeader(http.StatusOK)
		})

		metadata := &oauthex.ProtectedResourceMetadata{
			Resource:             "https://example.com/mcp",
			AuthorizationServers: []string{"https://auth.example.com"},
			ScopesSupported:      []string{"mcp:read"},
		}

		mux := http.NewServeMux()
		mux.Handle("/mcp", auth.RequireBearerToken(verifier, &auth.RequireBearerTokenOptions{
			ResourceMetadataURL: resourceMetadataURL,
			Scopes:              []string{"mcp:read"},
		})(protected))
		mux.Handle("/.well-known/oauth-protected-resource", auth.ProtectedResourceMetadataHandler(metadata))

		ts := httptest.NewServer(mux)
		defer ts.Close()

		// (a) no token -> 401 with a WWW-Authenticate header.
		resp, err := http.Get(ts.URL + "/mcp")
		if err != nil {
			t.Fatalf("GET /mcp (no token): %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("no token: status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
		if wa := resp.Header.Get("WWW-Authenticate"); !strings.Contains(wa, "resource_metadata=") {
			t.Errorf("no token: WWW-Authenticate = %q, want it to contain resource_metadata=", wa)
		}

		// (a2) bad token -> also 401 with a WWW-Authenticate header.
		req, err := http.NewRequest(http.MethodGet, ts.URL+"/mcp", nil)
		if err != nil {
			t.Fatalf("build bad-token request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer wrong-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /mcp (bad token): %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("bad token: status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
		}
		if wa := resp.Header.Get("WWW-Authenticate"); !strings.Contains(wa, "resource_metadata=") {
			t.Errorf("bad token: WWW-Authenticate = %q, want it to contain resource_metadata=", wa)
		}

		// (b) good token -> 200, and the verifier's TokenInfo reached the handler.
		req, err = http.NewRequest(http.MethodGet, ts.URL+"/mcp", nil)
		if err != nil {
			t.Fatalf("build good-token request: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+goodToken)
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /mcp (good token): %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("good token: status = %d, want %d", resp.StatusCode, http.StatusOK)
		}

		// (c) serve + fetch oauthex.ProtectedResourceMetadata as JSON.
		resp, err = http.Get(ts.URL + "/.well-known/oauth-protected-resource")
		if err != nil {
			t.Fatalf("GET /.well-known/oauth-protected-resource: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("metadata endpoint: status = %d, want %d", resp.StatusCode, http.StatusOK)
		}
		var got oauthex.ProtectedResourceMetadata
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
			t.Fatalf("decode metadata: %v", err)
		}
		if got.Resource != metadata.Resource {
			t.Errorf("metadata.Resource = %q, want %q", got.Resource, metadata.Resource)
		}
		if len(got.AuthorizationServers) != 1 || got.AuthorizationServers[0] != "https://auth.example.com" {
			t.Errorf("metadata.AuthorizationServers = %v, want [https://auth.example.com]", got.AuthorizationServers)
		}
		if len(got.ScopesSupported) != 1 || got.ScopesSupported[0] != "mcp:read" {
			t.Errorf("metadata.ScopesSupported = %v, want [mcp:read]", got.ScopesSupported)
		}
	})
}
