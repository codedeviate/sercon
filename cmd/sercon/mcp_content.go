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
// Each item is dispatched by toContentItem on its `type` field.
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
		c, err := toContentItem(m)
		if err != nil {
			return nil, fmt.Errorf("mcp content[%d]: %w", i, err)
		}
		out = append(out, c)
	}
	return out, nil
}

// toContentItem converts a single already-exported JS content object
// ({type, ...}) into an mcp.Content, dispatched on its `type` field:
//
//	"text"     -> &mcp.TextContent{Text}
//	"image"    -> &mcp.ImageContent{Data, MIMEType}
//	"audio"    -> &mcp.AudioContent{Data, MIMEType}
//	"resource" -> &mcp.EmbeddedResource{Resource}
//
// Shared by toContentList (each element of a tool/embedded-resource content
// array) and toGetPromptResult (a prompt message's `content` is a single
// object, not an array) — deliberately not duplicated between them.
func toContentItem(m map[string]any) (mcp.Content, error) {
	typ, _ := m["type"].(string)
	switch typ {
	case "text":
		text, _ := m["text"].(string)
		return &mcp.TextContent{Text: text}, nil

	case "image":
		data, err := decodeContentData(m["data"])
		if err != nil {
			return nil, fmt.Errorf("(image): %w", err)
		}
		mimeType, _ := m["mimeType"].(string)
		return &mcp.ImageContent{Data: data, MIMEType: mimeType}, nil

	case "audio":
		data, err := decodeContentData(m["data"])
		if err != nil {
			return nil, fmt.Errorf("(audio): %w", err)
		}
		mimeType, _ := m["mimeType"].(string)
		return &mcp.AudioContent{Data: data, MIMEType: mimeType}, nil

	case "resource":
		res, err := toEmbeddedResource(m["resource"])
		if err != nil {
			return nil, fmt.Errorf("(resource): %w", err)
		}
		return res, nil

	default:
		return nil, fmt.Errorf("unknown content type %q", typ)
	}
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

// toCompleteResult converts a JS `srv.completion` handler's settled return
// value into an SDK *mcp.CompleteResult, mirroring toReadResourceResult/
// toGetPromptResult's on-the-loop conversion contract (v is exported
// immediately; no goja.Value is retained past this call).
//
// Two accepted shapes:
//   - a plain string[] -> Completion.Values, Total/HasMore left zero.
//   - an object { values?: string[], total?: number, hasMore?: boolean } ->
//     each field mapped directly; `values` defaults to an empty slice when
//     omitted.
//
// undefined/null/nil (a handler that declines to complete) is not an error
// here — it converts to an empty CompleteResult{}, the same "no matches"
// shape the mcp.serve dispatcher already returns when no JS completion
// handler is registered at all (see CompletionHandler in mcp.go). A
// malformed non-nil return (wrong element/field types, or a shape that's
// neither an array nor an object) IS an error: unlike toToolResult, there's
// no isError-equivalent field on CompleteResult, so it propagates as a
// protocol error via the CompletionHandler dispatcher's convert path.
func toCompleteResult(_ *goja.Runtime, v goja.Value) (*mcp.CompleteResult, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return &mcp.CompleteResult{}, nil
	}

	toStringSlice := func(raw any) ([]string, error) {
		list, ok := raw.([]any)
		if !ok {
			return nil, fmt.Errorf("mcp completion result: `values` must be an array of strings, got %T", raw)
		}
		values := make([]string, 0, len(list))
		for i, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("mcp completion result: values[%d] must be a string, got %T", i, item)
			}
			values = append(values, s)
		}
		return values, nil
	}

	if arr, ok := v.Export().([]any); ok {
		values, err := toStringSlice(arr)
		if err != nil {
			return nil, err
		}
		return &mcp.CompleteResult{Completion: mcp.CompletionResultDetails{Values: values}}, nil
	}

	m, ok := v.Export().(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp completion result: want a string array or an object with `values`, got %T", v.Export())
	}

	details := mcp.CompletionResultDetails{}
	if rawValues, has := m["values"]; has {
		values, err := toStringSlice(rawValues)
		if err != nil {
			return nil, err
		}
		details.Values = values
	}
	if rawTotal, has := m["total"]; has {
		switch t := rawTotal.(type) {
		case int64:
			details.Total = int(t)
		case float64:
			details.Total = int(t)
		default:
			return nil, fmt.Errorf("mcp completion result: `total` must be a number, got %T", rawTotal)
		}
	}
	if rawHasMore, has := m["hasMore"]; has {
		b, ok := rawHasMore.(bool)
		if !ok {
			return nil, fmt.Errorf("mcp completion result: `hasMore` must be a boolean, got %T", rawHasMore)
		}
		details.HasMore = b
	}

	return &mcp.CompleteResult{Completion: details}, nil
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

// toGetPromptResult converts a JS prompt `get` handler's settled return
// value into an SDK *mcp.GetPromptResult, mirroring toReadResourceResult's
// on-the-loop conversion contract (v is exported immediately; no goja.Value
// is retained past this call).
//
// Expected shape: { description?: string, messages: [{ role, content }] }.
// Each message's `content` is a single MCP content object (e.g.
// {type:"text", text}), converted via toContentItem — the same per-item
// dispatch toContentList uses for tool/embedded-resource content arrays,
// deliberately not duplicated here.
//
// Unlike toToolResult, an unrecognised shape is a Go error (not an isError
// result): there's no isError-equivalent field on GetPromptResult, so a
// malformed handler return is indistinguishable from any other prompt-get
// failure and propagates as a protocol error via jsPrompt's convert path.
func toGetPromptResult(_ *goja.Runtime, v goja.Value) (*mcp.GetPromptResult, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, fmt.Errorf("mcp prompt result: get handler returned no value")
	}

	m, ok := v.Export().(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp prompt result: want an object with `messages`, got %T", v.Export())
	}

	result := &mcp.GetPromptResult{}
	if desc, has := m["description"]; has {
		s, ok := desc.(string)
		if !ok {
			return nil, fmt.Errorf("mcp prompt result: `description` must be a string, got %T", desc)
		}
		result.Description = s
	}

	rawMessages, has := m["messages"]
	if !has {
		return nil, fmt.Errorf("mcp prompt result: object must have `messages`")
	}
	items, ok := rawMessages.([]any)
	if !ok {
		return nil, fmt.Errorf("mcp prompt result: `messages` must be an array, got %T", rawMessages)
	}

	messages := make([]*mcp.PromptMessage, 0, len(items))
	for i, item := range items {
		mm, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("mcp prompt result: messages[%d]: expected an object, got %T", i, item)
		}
		role, _ := mm["role"].(string)
		if role == "" {
			return nil, fmt.Errorf("mcp prompt result: messages[%d]: `role` is required", i)
		}
		contentVal, has := mm["content"]
		if !has {
			return nil, fmt.Errorf("mcp prompt result: messages[%d]: `content` is required", i)
		}
		cm, ok := contentVal.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("mcp prompt result: messages[%d]: `content` must be an object, got %T", i, contentVal)
		}
		content, err := toContentItem(cm)
		if err != nil {
			return nil, fmt.Errorf("mcp prompt result: messages[%d]: %w", i, err)
		}
		messages = append(messages, &mcp.PromptMessage{Role: mcp.Role(role), Content: content})
	}
	result.Messages = messages

	return result, nil
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
