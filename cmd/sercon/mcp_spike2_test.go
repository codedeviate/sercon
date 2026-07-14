package main

import (
	"context"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPPhase2Spike pins the exact go-sdk v1.6.1 API surface Phase-2
// features will use: progress notifications + logging from a tool handler,
// ServerOptions{PageSize, CompletionHandler, SubscribeHandler,
// UnsubscribeHandler}, resource templates + ResourceUpdated, and
// RemoveTools. Pure-Go, in-memory transport — no goja involved.
//
// Every signature below was checked against the real source under
// $(go env GOMODCACHE)/github.com/modelcontextprotocol/go-sdk@v1.6.1/mcp
// and matched the brief exactly; see task-1-report.md for the trace.
func TestMCPPhase2Spike(t *testing.T) {
	ctx := context.Background()

	completionValues := []string{"python", "pytorch", "pyside"}

	subscribeCh := make(chan string, 1)
	unsubscribeCh := make(chan string, 1)

	srv := mcp.NewServer(&mcp.Implementation{Name: "spike2", Version: "0.0.0"}, &mcp.ServerOptions{
		PageSize: 2,
		CompletionHandler: func(_ context.Context, req *mcp.CompleteRequest) (*mcp.CompleteResult, error) {
			if req.Params.Argument.Name != "lang" {
				t.Errorf("CompletionHandler: argument name = %q, want %q", req.Params.Argument.Name, "lang")
			}
			return &mcp.CompleteResult{
				Completion: mcp.CompletionResultDetails{Values: completionValues},
			}, nil
		},
		SubscribeHandler: func(_ context.Context, req *mcp.SubscribeRequest) error {
			subscribeCh <- req.Params.URI
			return nil
		},
		UnsubscribeHandler: func(_ context.Context, req *mcp.UnsubscribeRequest) error {
			unsubscribeCh <- req.Params.URI
			return nil
		},
	})

	// --- tool 1: progress + logging ---
	progressCh := make(chan *mcp.ProgressNotificationParams, 4)
	loggingCh := make(chan *mcp.LoggingMessageParams, 4)

	srv.AddTool(&mcp.Tool{
		Name:        "progressLog",
		Description: "emit progress + a log line",
		InputSchema: map[string]any{"type": "object"},
	},
		func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			token := req.Params.GetProgressToken()
			if token == nil {
				t.Error("progressLog handler: GetProgressToken() returned nil")
			}
			if err := req.Session.NotifyProgress(ctx, &mcp.ProgressNotificationParams{
				ProgressToken: token,
				Progress:      1,
				Total:         2,
				Message:       "halfway",
			}); err != nil {
				t.Errorf("NotifyProgress: %v", err)
			}
			if err := req.Session.Log(ctx, &mcp.LoggingMessageParams{
				Level: "warning",
				Data:  "progressLog ran",
			}); err != nil {
				t.Errorf("Log: %v", err)
			}
			return &mcp.CallToolResult{}, nil
		})

	// --- tools 2-4: pagination + RemoveTools fodder ---
	noop := func(_ context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{}, nil
	}
	schema := map[string]any{"type": "object"}
	srv.AddTool(&mcp.Tool{Name: "removeMe", Description: "will be removed", InputSchema: schema}, noop)
	srv.AddTool(&mcp.Tool{Name: "keep1", Description: "stays", InputSchema: schema}, noop)
	srv.AddTool(&mcp.Tool{Name: "keep2", Description: "stays", InputSchema: schema}, noop)

	// --- resource template ---
	srv.AddResourceTemplate(&mcp.ResourceTemplate{
		Name:        "user-file",
		URITemplate: "file:///users/{id}",
		MIMEType:    "text/plain",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: "file:///users/1", Text: "stub"}}}, nil
	})

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()

	updatedCh := make(chan string, 4)
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, &mcp.ClientOptions{
		ProgressNotificationHandler: func(_ context.Context, req *mcp.ProgressNotificationClientRequest) {
			progressCh <- req.Params
		},
		LoggingMessageHandler: func(_ context.Context, req *mcp.LoggingMessageRequest) {
			loggingCh <- req.Params
		},
		ResourceUpdatedHandler: func(_ context.Context, req *mcp.ResourceUpdatedNotificationRequest) {
			updatedCh <- req.Params.URI
		},
	})
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer sess.Close()

	// Log() no-ops until the client has set a logging level (server.go:
	// "The message is not sent if the client has not called SetLevel").
	if err := sess.SetLoggingLevel(ctx, &mcp.SetLoggingLevelParams{Level: "info"}); err != nil {
		t.Fatalf("SetLoggingLevel: %v", err)
	}

	t.Run("progress and logging", func(t *testing.T) {
		res, err := sess.CallTool(ctx, &mcp.CallToolParams{
			Name: "progressLog",
			Meta: mcp.Meta{"progressToken": "tok-123"},
		})
		if err != nil {
			t.Fatalf("call tool: %v", err)
		}
		if res.IsError {
			t.Fatalf("progressLog reported an error result: %+v", res)
		}

		select {
		case p := <-progressCh:
			if p.ProgressToken != "tok-123" {
				t.Errorf("progress token = %v, want %q", p.ProgressToken, "tok-123")
			}
			if p.Progress != 1 || p.Total != 2 || p.Message != "halfway" {
				t.Errorf("progress params = %+v, want Progress=1 Total=2 Message=halfway", p)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for progress notification")
		}

		select {
		case l := <-loggingCh:
			if l.Level != "warning" || l.Data != "progressLog ran" {
				t.Errorf("logging params = %+v, want Level=warning Data=%q", l, "progressLog ran")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for logging message")
		}
	})

	t.Run("pagination via PageSize", func(t *testing.T) {
		// 4 tools registered (progressLog, removeMe, keep1, keep2); PageSize:2
		// must cap the first page at 2 and hand back a NextCursor.
		first, err := sess.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("ListTools (page 1): %v", err)
		}
		if len(first.Tools) != 2 {
			t.Fatalf("page 1: got %d tools, want 2 (PageSize)", len(first.Tools))
		}
		if first.NextCursor == "" {
			t.Fatal("page 1: NextCursor is empty, want non-empty (more pages remain)")
		}
	})

	t.Run("completion handler", func(t *testing.T) {
		res, err := sess.Complete(ctx, &mcp.CompleteParams{
			Argument: mcp.CompleteParamsArgument{Name: "lang", Value: "py"},
			Ref:      &mcp.CompleteReference{Type: "ref/prompt", Name: "whatever"},
		})
		if err != nil {
			t.Fatalf("Complete: %v", err)
		}
		if len(res.Completion.Values) != len(completionValues) {
			t.Fatalf("completion values = %v, want %v", res.Completion.Values, completionValues)
		}
		for i, v := range completionValues {
			if res.Completion.Values[i] != v {
				t.Fatalf("completion values = %v, want %v", res.Completion.Values, completionValues)
			}
		}
	})

	t.Run("subscribe, ResourceUpdated, unsubscribe", func(t *testing.T) {
		if err := sess.Subscribe(ctx, &mcp.SubscribeParams{URI: "file:///users/1"}); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		select {
		case uri := <-subscribeCh:
			if uri != "file:///users/1" {
				t.Errorf("SubscribeHandler saw URI %q, want %q", uri, "file:///users/1")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for SubscribeHandler")
		}

		if err := srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: "file:///users/1"}); err != nil {
			t.Fatalf("ResourceUpdated: %v", err)
		}
		select {
		case uri := <-updatedCh:
			if uri != "file:///users/1" {
				t.Errorf("ResourceUpdatedHandler saw URI %q, want %q", uri, "file:///users/1")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for ResourceUpdatedHandler")
		}

		if err := sess.Unsubscribe(ctx, &mcp.UnsubscribeParams{URI: "file:///users/1"}); err != nil {
			t.Fatalf("Unsubscribe: %v", err)
		}
		select {
		case uri := <-unsubscribeCh:
			if uri != "file:///users/1" {
				t.Errorf("UnsubscribeHandler saw URI %q, want %q", uri, "file:///users/1")
			}
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for UnsubscribeHandler")
		}

		// After unsubscribing, a further ResourceUpdated must NOT reach the client.
		if err := srv.ResourceUpdated(ctx, &mcp.ResourceUpdatedNotificationParams{URI: "file:///users/1"}); err != nil {
			t.Fatalf("ResourceUpdated (post-unsubscribe): %v", err)
		}
		select {
		case uri := <-updatedCh:
			t.Fatalf("received ResourceUpdated after unsubscribe: %q", uri)
		case <-time.After(300 * time.Millisecond):
			// expected: nothing arrives
		}
	})

	t.Run("resource templates list", func(t *testing.T) {
		res, err := sess.ListResourceTemplates(ctx, nil)
		if err != nil {
			t.Fatalf("ListResourceTemplates: %v", err)
		}
		var found *mcp.ResourceTemplate
		for _, rt := range res.ResourceTemplates {
			if rt.Name == "user-file" {
				found = rt
				break
			}
		}
		if found == nil {
			t.Fatalf("ListResourceTemplates: %q not found in %+v", "user-file", res.ResourceTemplates)
		}
		if found.URITemplate != "file:///users/{id}" || found.MIMEType != "text/plain" {
			t.Errorf("template = %+v, want URITemplate=file:///users/{id} MIMEType=text/plain", found)
		}
	})

	t.Run("RemoveTools", func(t *testing.T) {
		srv.RemoveTools("removeMe")

		var names []string
		cursor := ""
		for {
			page, err := sess.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
			if err != nil {
				t.Fatalf("ListTools: %v", err)
			}
			for _, tl := range page.Tools {
				names = append(names, tl.Name)
			}
			if page.NextCursor == "" {
				break
			}
			cursor = page.NextCursor
		}
		for _, n := range names {
			if n == "removeMe" {
				t.Fatalf("RemoveTools(\"removeMe\") did not remove it; tools = %v", names)
			}
		}
		if len(names) != 3 {
			t.Fatalf("expected 3 remaining tools after RemoveTools, got %d: %v", len(names), names)
		}
	})
}
