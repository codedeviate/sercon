package main

import (
	"encoding/base64"
	"fmt"

	"github.com/dop251/goja"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toToolResult converts a JS tool handler's settled return value (the goja
// result callJSHandler hands back) into an SDK *mcp.CallToolResult. It
// exports v to native Go data immediately — no goja.Value is retained past
// this call, matching the Task 3 rule that conversion happens on the loop
// before the value can escape to another goroutine.
//
// Supported shapes:
//   - a plain string -> one TextContent.
//   - undefined/null/nil -> an empty, non-error result (a handler that
//     resolves with no value).
//   - an object with a `content` array -> each item converted by its `type`
//     field via toContentList; `structuredContent` and `isError` are carried
//     through as-is.
//
// Per the brief, toToolResult itself never returns an error: an unknown
// content `type` (or any other conversion failure encountered while walking
// `content`) is surfaced as an IsError result carrying the failure message
// as a single TextContent, rather than panicking or propagating a Go error.
// Any other unrecognised top-level shape gets the same isError treatment.
func toToolResult(vm *goja.Runtime, v goja.Value) *mcp.CallToolResult {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return &mcp.CallToolResult{}
	}

	switch exported := v.Export().(type) {
	case string:
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: exported}}}

	case map[string]any:
		result := &mcp.CallToolResult{}

		if _, has := exported["content"]; has {
			contentVal := v.ToObject(vm).Get("content")
			list, err := toContentList(vm, contentVal)
			if err != nil {
				return errorResult(err)
			}
			result.Content = list
		}

		if sc, has := exported["structuredContent"]; has {
			result.StructuredContent = sc
		}

		if isErr, ok := exported["isError"].(bool); ok {
			result.IsError = isErr
		}

		return result

	default:
		return errorResult(fmt.Errorf("mcp tool result: unsupported result type %T", exported))
	}
}

// errorResult builds the isError result toToolResult falls back to when
// content conversion fails, per the brief's "unknown type -> error, turned
// into an isError result by the caller" contract.
func errorResult(err error) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
	}
}

// toContentList converts a goja value representing a JS array of content
// items (e.g. the `content` field of a tool result) into []mcp.Content.
// Each item is dispatched on its `type` field:
//
//	"text"     -> &mcp.TextContent{Text}
//	"image"    -> &mcp.ImageContent{Data, MIMEType}
//	"audio"    -> &mcp.AudioContent{Data, MIMEType}
//	"resource" -> &mcp.EmbeddedResource{Resource}
//
// An unrecognised `type` (or a malformed item) returns an error; the caller
// (toToolResult here, the Task 5 tool binding for handler-level results)
// decides how to surface it.
func toContentList(vm *goja.Runtime, v goja.Value) ([]mcp.Content, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, nil
	}

	items, ok := v.Export().([]any)
	if !ok {
		return nil, fmt.Errorf("mcp content: expected an array, got %T", v.Export())
	}

	out := make([]mcp.Content, 0, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("mcp content[%d]: expected an object, got %T", i, item)
		}

		typ, _ := m["type"].(string)
		switch typ {
		case "text":
			text, _ := m["text"].(string)
			out = append(out, &mcp.TextContent{Text: text})

		case "image":
			data, err := decodeContentData(m["data"])
			if err != nil {
				return nil, fmt.Errorf("mcp content[%d] (image): %w", i, err)
			}
			mimeType, _ := m["mimeType"].(string)
			out = append(out, &mcp.ImageContent{Data: data, MIMEType: mimeType})

		case "audio":
			data, err := decodeContentData(m["data"])
			if err != nil {
				return nil, fmt.Errorf("mcp content[%d] (audio): %w", i, err)
			}
			mimeType, _ := m["mimeType"].(string)
			out = append(out, &mcp.AudioContent{Data: data, MIMEType: mimeType})

		case "resource":
			res, err := toEmbeddedResource(m["resource"])
			if err != nil {
				return nil, fmt.Errorf("mcp content[%d] (resource): %w", i, err)
			}
			out = append(out, res)

		default:
			return nil, fmt.Errorf("mcp content[%d]: unknown content type %q", i, typ)
		}
	}
	return out, nil
}

// toEmbeddedResource converts an already-exported JS `resource` object
// ({uri, mimeType?, text?, blob?}) into an *mcp.EmbeddedResource.
func toEmbeddedResource(v any) (*mcp.EmbeddedResource, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("resource: expected an object, got %T", v)
	}

	rc := &mcp.ResourceContents{}
	rc.URI, _ = m["uri"].(string)
	rc.MIMEType, _ = m["mimeType"].(string)
	rc.Text, _ = m["text"].(string)

	if blob, has := m["blob"]; has {
		b, err := decodeContentData(blob)
		if err != nil {
			return nil, fmt.Errorf("resource blob: %w", err)
		}
		rc.Blob = b
	}

	return &mcp.EmbeddedResource{Resource: rc}, nil
}

// toReadResourceResult converts a JS resource `read` handler's settled return
// value into an SDK *mcp.ReadResourceResult, mirroring toToolResult's
// on-the-loop conversion contract (v is exported immediately; no goja.Value
// is retained past this call). uri is the requested URI (from
// ReadResourceRequest.Params.URI, not the handler's return value — the
// result always echoes back what was asked for) and mimeType is the
// resource's registered MIMEType; both are stamped onto the single
// ResourceContents produced.
//
// Supported shapes:
//   - {text: string} -> one ResourceContents with Text set.
//   - {blob: <base64 string|Uint8Array|ArrayBuffer>} -> one ResourceContents
//     with Blob set, decoded via decodeContentData (the same helper
//     toContentList/toEmbeddedResource use for image/audio/resource blobs —
//     deliberately not duplicated here).
//
// Unlike toToolResult, an unrecognised shape is a Go error (not an isError
// result): there's no isError-equivalent field on ReadResourceResult, so a
// malformed handler return is indistinguishable from any other resource-read
// failure and propagates as a protocol error via jsResource's convert path.
func toReadResourceResult(_ *goja.Runtime, uri, mimeType string, v goja.Value) (*mcp.ReadResourceResult, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, fmt.Errorf("mcp resource result: read handler for %q returned no value", uri)
	}

	m, ok := v.Export().(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp resource result: want an object with `text` or `blob`, got %T", v.Export())
	}

	rc := &mcp.ResourceContents{URI: uri, MIMEType: mimeType}

	if text, has := m["text"]; has {
		s, ok := text.(string)
		if !ok {
			return nil, fmt.Errorf("mcp resource result: `text` must be a string, got %T", text)
		}
		rc.Text = s
	} else if blob, has := m["blob"]; has {
		b, err := decodeContentData(blob)
		if err != nil {
			return nil, fmt.Errorf("mcp resource result: blob: %w", err)
		}
		rc.Blob = b
	} else {
		return nil, fmt.Errorf("mcp resource result: object must have `text` or `blob`")
	}

	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{rc}}, nil
}

// decodeContentData coerces an already-exported JS `data` value into raw
// bytes for Image/AudioContent.Data (which the SDK marshals as base64 on
// the wire, per the `// base64-encoded` comment in the SDK's content.go).
// A string is treated as base64 text and decoded; a Uint8Array (exported
// as []byte by goja) or ArrayBuffer is used as-is. This deliberately does
// NOT reuse bytesFromExported (see http.go): that helper treats a string
// as raw UTF-8 bytes, which is the right behaviour for request/response
// bodies but wrong here — a JS content item's string `data` is base64 text
// that must be decoded to the actual binary payload.
func decodeContentData(v any) ([]byte, error) {
	switch e := v.(type) {
	case string:
		b, err := base64.StdEncoding.DecodeString(e)
		if err != nil {
			return nil, fmt.Errorf("invalid base64 data: %w", err)
		}
		return b, nil
	case []byte:
		return e, nil
	case goja.ArrayBuffer:
		return e.Bytes(), nil
	default:
		return nil, fmt.Errorf("want a base64 string, Uint8Array, or ArrayBuffer, got %T", e)
	}
}
