package main

import (
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/antchfx/htmlquery"
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
func nodeText(n *html.Node) string { return htmlquery.InnerText(n) }

// nodeOuterHTML renders the node and its subtree as HTML.
func nodeOuterHTML(n *html.Node) string { return htmlquery.OutputHTML(n, true) }

// nodeInnerHTML renders only the node's children as HTML.
func nodeInnerHTML(n *html.Node) string { return htmlquery.OutputHTML(n, false) }

// nodeTag returns the lower-cased element name, or "" for non-element nodes
// (document, text, comment, attribute results).
func nodeTag(n *html.Node) string {
	if n.Type == html.ElementNode {
		return strings.ToLower(n.Data)
	}
	return ""
}

// nodeAttr returns an attribute value and whether it was present.
func nodeAttr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val, true
		}
	}
	return "", false
}

// nodeAttrs returns all attributes of the node as a name→value map.
func nodeAttrs(n *html.Node) map[string]string {
	m := make(map[string]string, len(n.Attr))
	for _, a := range n.Attr {
		m[a.Key] = a.Val
	}
	return m
}
