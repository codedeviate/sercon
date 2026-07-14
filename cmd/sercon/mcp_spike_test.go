package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCPSDKSpike pins the exact SDK API the bridge relies on: dynamic-schema
// tool registration, the ToolHandler signature, CallToolResult/TextContent, and
// the in-memory client/server transport pair. Pure-Go — no goja involved.
func TestMCPSDKSpike(t *testing.T) {
	ctx := context.Background()
	srv := mcp.NewServer(&mcp.Implementation{Name: "spike", Version: "0.0.0"}, nil)

	srv.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "echo the text argument",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"text": map[string]any{"type": "string"}},
			"required":   []any{"text"},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// req.Params.Arguments is a *CallToolParamsRaw json.RawMessage (not
		// `any`), unlike the client-side CallToolParams.Arguments used below —
		// the brief's map[string]any type assertion doesn't compile against
		// v1.6.1. Unmarshal it as the raw-schema example in the SDK does.
		var args map[string]any
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		text, _ := args["text"].(string)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
	})

	ct, st := mcp.NewInMemoryTransports()
	go func() { _ = srv.Run(ctx, st) }()

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	sess, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer sess.Close()

	res, err := sess.CallTool(ctx, &mcp.CallToolParams{Name: "echo", Arguments: map[string]any{"text": "hi"}})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "hi" {
		t.Fatalf("want TextContent 'hi', got %#v", res.Content[0])
	}
}
