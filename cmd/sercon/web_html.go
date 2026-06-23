package main

import (
	"bytes"
	"context"
	"strings"
	"time"

	"github.com/andybalholm/cascadia"
	"github.com/antchfx/htmlquery"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"golang.org/x/net/html"
)

// htmlParse runs the lenient HTML5 tree builder. By design it does not error on
// malformed markup (unclosed tags, implied elements) — it returns a best-effort
// document tree. Errors are reserved for unreadable input.
func htmlParse(source string) (*html.Node, error) {
	return html.Parse(strings.NewReader(source))
}

// cssQueryAll returns all descendants of root matching the CSS selector.
// A malformed selector returns an error (surfaced to the script as a throw).
func cssQueryAll(root *html.Node, selector string) ([]*html.Node, error) {
	sel, err := cascadia.Compile(selector)
	if err != nil {
		return nil, err
	}
	return cascadia.QueryAll(root, sel), nil
}

// cssQueryFirst returns the first descendant matching the selector, or nil.
func cssQueryFirst(root *html.Node, selector string) (*html.Node, error) {
	sel, err := cascadia.Compile(selector)
	if err != nil {
		return nil, err
	}
	return cascadia.Query(root, sel), nil
}

// xpathQueryAll evaluates an XPath expression against root. A leading "//" is
// absolute (whole tree); use ".//" to scope to root's subtree. Attribute and
// text matches (e.g. //a/@href) come back as nodes whose nodeText is the value.
func xpathQueryAll(root *html.Node, expr string) ([]*html.Node, error) {
	return htmlquery.QueryAll(root, expr)
}

// xpathQueryFirst returns the first XPath match, or nil.
func xpathQueryFirst(root *html.Node, expr string) (*html.Node, error) {
	return htmlquery.Query(root, expr)
}

// nodeText returns the concatenated text content of a node (and, for attribute
// nodes from an XPath //x/@attr match, the attribute value).
func nodeText(n *html.Node) string {
	if n == nil {
		return ""
	}
	return htmlquery.InnerText(n)
}

// nodeOuterHTML renders the node and its subtree as HTML.
func nodeOuterHTML(n *html.Node) string {
	if n == nil {
		return ""
	}
	return htmlquery.OutputHTML(n, true)
}

// nodeInnerHTML renders only the node's children as HTML.
func nodeInnerHTML(n *html.Node) string {
	if n == nil {
		return ""
	}
	return htmlquery.OutputHTML(n, false)
}

// nodeTag returns the lower-cased element name, or "" for non-element nodes
// (document, text, comment, attribute results).
func nodeTag(n *html.Node) string {
	if n == nil || n.Type != html.ElementNode {
		return ""
	}
	return strings.ToLower(n.Data) // html.Parse already lowercases element names; defensive for programmatically built nodes
}

// nodeAttr returns an attribute value and whether it was present.
func nodeAttr(n *html.Node, name string) (string, bool) {
	if n == nil {
		return "", false
	}
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

// nodeAttrs returns all attributes of the node as a name→value map. Namespaced
// attributes (e.g. SVG xlink:href) are keyed "namespace:key" to avoid collisions.
func nodeAttrs(n *html.Node) map[string]string {
	if n == nil {
		return map[string]string{}
	}
	m := make(map[string]string, len(n.Attr))
	for _, a := range n.Attr {
		key := a.Key
		if a.Namespace != "" {
			key = a.Namespace + ":" + a.Key
		}
		m[key] = a.Val
	}
	return m
}

// newHTMLNode wraps a *html.Node as a chainable goja handle. Built per call on
// the event loop (goja is single-threaded; never construct off-loop). Mirrors
// the image handle pattern in image.go (newImageHandle).
func newHTMLNode(vm *goja.Runtime, n *html.Node) goja.Value {
	obj := vm.NewObject()

	nodesToArray := func(ns []*html.Node) goja.Value {
		arr := make([]interface{}, len(ns))
		for i, x := range ns {
			arr[i] = newHTMLNode(vm, x)
		}
		return vm.ToValue(arr)
	}
	firstOrNull := func(x *html.Node) goja.Value {
		if x == nil {
			return goja.Null()
		}
		return newHTMLNode(vm, x)
	}
	throwOn := func(err error, kind, expr string) {
		if err != nil {
			panic(vm.NewTypeError("web.html: invalid %s %q: %v", kind, expr, err))
		}
	}

	_ = obj.Set("find", func(call goja.FunctionCall) goja.Value {
		sel := call.Argument(0).String()
		x, err := cssQueryFirst(n, sel)
		throwOn(err, "CSS selector", sel)
		return firstOrNull(x)
	})
	_ = obj.Set("findAll", func(call goja.FunctionCall) goja.Value {
		sel := call.Argument(0).String()
		xs, err := cssQueryAll(n, sel)
		throwOn(err, "CSS selector", sel)
		return nodesToArray(xs)
	})
	_ = obj.Set("xpath", func(call goja.FunctionCall) goja.Value {
		expr := call.Argument(0).String()
		x, err := xpathQueryFirst(n, expr)
		throwOn(err, "XPath expression", expr)
		return firstOrNull(x)
	})
	_ = obj.Set("xpathAll", func(call goja.FunctionCall) goja.Value {
		expr := call.Argument(0).String()
		xs, err := xpathQueryAll(n, expr)
		throwOn(err, "XPath expression", expr)
		return nodesToArray(xs)
	})
	_ = obj.Set("text", func(goja.FunctionCall) goja.Value { return vm.ToValue(nodeText(n)) })
	_ = obj.Set("html", func(goja.FunctionCall) goja.Value { return vm.ToValue(nodeOuterHTML(n)) })
	_ = obj.Set("innerHTML", func(goja.FunctionCall) goja.Value { return vm.ToValue(nodeInnerHTML(n)) })
	_ = obj.Set("tag", func(goja.FunctionCall) goja.Value { return vm.ToValue(nodeTag(n)) })
	_ = obj.Set("attr", func(call goja.FunctionCall) goja.Value {
		v, ok := nodeAttr(n, call.Argument(0).String())
		if !ok {
			return goja.Null()
		}
		return vm.ToValue(v)
	})
	_ = obj.Set("attrs", func(goja.FunctionCall) goja.Value { return vm.ToValue(nodeAttrs(n)) })
	return obj
}

// htmlParseBinding implements web.html.parse(source) — synchronous; runs on the
// loop so building the handle is safe.
func htmlParseBinding(vm *goja.Runtime) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		root, err := htmlParse(call.Argument(0).String())
		if err != nil {
			panic(vm.NewGoError(err))
		}
		return newHTMLNode(vm, root)
	}
}

// htmlLoadBinding implements web.html.load(url, opts?) — async. It cannot use
// PromisifyAsync because the result is a goja handle that must be built on the
// loop; PromisifyAsync would vm.ToValue the off-loop value. So we replicate the
// keepAlive sentinel (eventloop only counts setTimeout/Interval/Immediate as
// live work) and resolve with newHTMLNode inside RunOnLoop.
func htmlLoadBinding(vm *goja.Runtime, loop *eventloop.EventLoop) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		promise, resolve, reject := vm.NewPromise()
		url := call.Argument(0).String()
		var optsMap map[string]any
		if o := call.Argument(1); o != nil && !goja.IsUndefined(o) && !goja.IsNull(o) {
			if m, ok := o.Export().(map[string]any); ok {
				optsMap = m
			}
		}
		keepAlive := loop.SetTimeout(func(*goja.Runtime) {}, 24*time.Hour)
		go func() {
			body, _, err := loadBytes(context.Background(), url, optsMap)
			var root *html.Node
			if err == nil {
				root, err = html.Parse(bytes.NewReader(body))
			}
			loop.RunOnLoop(func(vm *goja.Runtime) {
				if err != nil {
					_ = reject(vm.NewGoError(err))
				} else {
					_ = resolve(newHTMLNode(vm, root))
				}
				loop.ClearTimeout(keepAlive)
			})
		}()
		return vm.ToValue(promise)
	}
}
