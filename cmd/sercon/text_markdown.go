// cmd/sercon/text_markdown.go
package main

import (
	"bytes"
	"fmt"

	"github.com/dop251/goja"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
)

// textMarkdownNamespace wires text.markdown.* — currently just toHtml, a
// string-in/string-out CommonMark→HTML renderer backed by the pure-Go
// goldmark. GFM extensions (tables, strikethrough, task lists, autolinks) are
// on by default; hardBreaks maps a source newline to <br>. Raw HTML in the
// source is escaped (goldmark's safe default).
func textMarkdownNamespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value { panic(vm.NewGoError(err)) }
	return map[string]any{
		"toHtml": func(call goja.FunctionCall) goja.Value {
			src := call.Argument(0).String()

			gfm, hardBreaks := true, false
			if opts, ok := call.Argument(1).Export().(map[string]any); ok {
				if v, ok := opts["gfm"].(bool); ok {
					gfm = v
				}
				if v, ok := opts["hardBreaks"].(bool); ok {
					hardBreaks = v
				}
			}

			var mdOpts []goldmark.Option
			if gfm {
				mdOpts = append(mdOpts, goldmark.WithExtensions(extension.GFM))
			}
			if hardBreaks {
				mdOpts = append(mdOpts, goldmark.WithRendererOptions(gmhtml.WithHardWraps()))
			}

			var buf bytes.Buffer
			if err := goldmark.New(mdOpts...).Convert([]byte(src), &buf); err != nil {
				return throw(fmt.Errorf("text.markdown.toHtml: %w", err))
			}
			return vm.ToValue(buf.String())
		},
	}
}
