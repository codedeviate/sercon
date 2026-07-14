package main

import (
	"encoding/base64"
	"reflect"
	"testing"

	"github.com/dop251/goja"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// evalMCP runs src on a fresh goja runtime and returns the resulting value,
// failing the test on a compile/runtime error. toToolResult/toContentList
// only need a *goja.Runtime (no eventloop), so these tests use goja.New()
// directly rather than spinning up a scriptengine.Engine.
func evalMCP(t *testing.T, vm *goja.Runtime, src string) goja.Value {
	t.Helper()
	v, err := vm.RunString(src)
	if err != nil {
		t.Fatalf("eval %q: %v", src, err)
	}
	return v
}

func TestMCPContent_StringToText(t *testing.T) {
	vm := goja.New()
	v := evalMCP(t, vm, `"hi"`)

	res := toToolResult(vm, v)

	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "hi" {
		t.Fatalf("want TextContent{Text: \"hi\"}, got %#v", res.Content[0])
	}
	if res.IsError {
		t.Fatal("want IsError false")
	}
}

func TestMCPContent_MultiItemWithImage(t *testing.T) {
	vm := goja.New()
	imgBytes := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	b64 := base64.StdEncoding.EncodeToString(imgBytes)

	v := evalMCP(t, vm, `({
		content: [
			{ type: "text", text: "a" },
			{ type: "image", data: "`+b64+`", mimeType: "image/png" },
		],
	})`)

	res := toToolResult(vm, v)

	if len(res.Content) != 2 {
		t.Fatalf("want 2 content items, got %d: %#v", len(res.Content), res.Content)
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "a" {
		t.Fatalf("content[0]: want TextContent{Text:\"a\"}, got %#v", res.Content[0])
	}
	ic, ok := res.Content[1].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("content[1]: want *mcp.ImageContent, got %#v", res.Content[1])
	}
	if ic.MIMEType != "image/png" {
		t.Fatalf("MIMEType = %q, want image/png", ic.MIMEType)
	}
	if !reflect.DeepEqual(ic.Data, imgBytes) {
		t.Fatalf("Data = %v, want %v (base64 round-trip broken)", ic.Data, imgBytes)
	}
	if res.IsError {
		t.Fatal("want IsError false")
	}
}

// TestMCPContent_ImageUint8ArrayData pins the other half of the binary-data
// nuance: when `data` arrives as a Uint8Array (not a base64 string), the
// bytes must be used directly with no base64 decode pass.
func TestMCPContent_ImageUint8ArrayData(t *testing.T) {
	vm := goja.New()
	v := evalMCP(t, vm, `({
		content: [
			{ type: "image", data: new Uint8Array([137, 80, 78, 71]), mimeType: "image/png" },
		],
	})`)

	res := toToolResult(vm, v)

	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	ic, ok := res.Content[0].(*mcp.ImageContent)
	if !ok {
		t.Fatalf("want *mcp.ImageContent, got %#v", res.Content[0])
	}
	want := []byte{137, 80, 78, 71}
	if !reflect.DeepEqual(ic.Data, want) {
		t.Fatalf("Data = %v, want %v", ic.Data, want)
	}
}

func TestMCPContent_Audio(t *testing.T) {
	vm := goja.New()
	audioBytes := []byte{0, 1, 2, 3, 4, 5}
	b64 := base64.StdEncoding.EncodeToString(audioBytes)

	v := evalMCP(t, vm, `({
		content: [{ type: "audio", data: "`+b64+`", mimeType: "audio/wav" }],
	})`)

	res := toToolResult(vm, v)

	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	ac, ok := res.Content[0].(*mcp.AudioContent)
	if !ok {
		t.Fatalf("want *mcp.AudioContent, got %#v", res.Content[0])
	}
	if ac.MIMEType != "audio/wav" {
		t.Fatalf("MIMEType = %q, want audio/wav", ac.MIMEType)
	}
	if !reflect.DeepEqual(ac.Data, audioBytes) {
		t.Fatalf("Data = %v, want %v", ac.Data, audioBytes)
	}
}

func TestMCPContent_Resource(t *testing.T) {
	vm := goja.New()
	v := evalMCP(t, vm, `({
		content: [{ type: "resource", resource: { uri: "file:///a.txt", mimeType: "text/plain", text: "hello" } }],
	})`)

	res := toToolResult(vm, v)

	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	er, ok := res.Content[0].(*mcp.EmbeddedResource)
	if !ok {
		t.Fatalf("want *mcp.EmbeddedResource, got %#v", res.Content[0])
	}
	if er.Resource == nil || er.Resource.URI != "file:///a.txt" || er.Resource.Text != "hello" {
		t.Fatalf("Resource = %#v, want {URI: file:///a.txt, Text: hello}", er.Resource)
	}
}

func TestMCPContent_StructuredContent(t *testing.T) {
	vm := goja.New()
	v := evalMCP(t, vm, `({ structuredContent: { ok: true } })`)

	res := toToolResult(vm, v)

	sc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("StructuredContent type = %T, want map[string]any", res.StructuredContent)
	}
	if sc["ok"] != true {
		t.Fatalf("StructuredContent = %#v, want {ok:true}", sc)
	}
}

func TestMCPContent_IsError(t *testing.T) {
	vm := goja.New()
	v := evalMCP(t, vm, `({ content: [{ type: "text", text: "e" }], isError: true })`)

	res := toToolResult(vm, v)

	if !res.IsError {
		t.Fatal("want IsError true")
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 content item, got %d", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok || tc.Text != "e" {
		t.Fatalf("want TextContent{Text:\"e\"}, got %#v", res.Content[0])
	}
}

func TestMCPContent_UnknownTypeList(t *testing.T) {
	vm := goja.New()
	v := evalMCP(t, vm, `[{ type: "bogus", text: "x" }]`)

	if _, err := toContentList(vm, v); err == nil {
		t.Fatal("want error for unknown content type, got nil")
	}
}

// TestMCPContent_UnknownTypeToolResultIsError verifies toToolResult's
// non-error signature still surfaces an unknown content type as an isError
// result (per the brief: unknown type -> error, and the caller — here
// toToolResult itself, standing in for the Task 5 tool binding — turns it
// into an isError result) rather than panicking.
func TestMCPContent_UnknownTypeToolResultIsError(t *testing.T) {
	vm := goja.New()
	v := evalMCP(t, vm, `({ content: [{ type: "bogus", text: "x" }] })`)

	res := toToolResult(vm, v)

	if !res.IsError {
		t.Fatal("want IsError true for unknown content type")
	}
	if len(res.Content) != 1 {
		t.Fatalf("want 1 (error) content item, got %d", len(res.Content))
	}
	if _, ok := res.Content[0].(*mcp.TextContent); !ok {
		t.Fatalf("want error surfaced as TextContent, got %#v", res.Content[0])
	}
}

// TestMCPContent_UndefinedResult verifies a handler that returns nothing
// (undefined) — e.g. resolved with no value — produces an empty, non-error
// result rather than panicking on a nil/undefined Export().
func TestMCPContent_UndefinedResult(t *testing.T) {
	vm := goja.New()
	v := evalMCP(t, vm, `undefined`)

	res := toToolResult(vm, v)

	if res == nil {
		t.Fatal("want non-nil CallToolResult")
	}
	if res.IsError {
		t.Fatal("want IsError false for undefined result")
	}
	if len(res.Content) != 0 {
		t.Fatalf("want 0 content items, got %d", len(res.Content))
	}
}
