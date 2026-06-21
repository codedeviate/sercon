package main

import (
	"fmt"
	"strings"
)

// cdpExec sends one CDP command through chromedriver's passthrough endpoint
// (POST /session/<id>/goog/cdp/execute) and returns the decoded result value.
// CDP is Chrome-only; firefox is rejected before any request is made. Callers
// must already hold s.do (it issues an s.command).
func (s *wdSession) cdpExec(cmd string, params map[string]any) (any, error) {
	if s.browser == "firefox" {
		return nil, fmt.Errorf("webdriver.cdp: CDP is Chrome-only (chromedriver); current browser is %q", s.browser)
	}
	if params == nil {
		params = map[string]any{}
	}
	v, err := s.command("POST", "/goog/cdp/execute", map[string]any{"cmd": cmd, "params": params})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unknown command") || strings.Contains(msg, "HTTP 404") || strings.Contains(msg, "invalid session") {
			return nil, fmt.Errorf("%w (requires a CDP-capable chromedriver endpoint)", err)
		}
		return nil, err
	}
	return v, nil
}

// cdpQuery maps a webdriver locator strategy to a query for CDP location.
// CSS-family strategies return useXPath=false and a CSS selector; XPath-family
// return useXPath=true and an XPath expression.
func cdpQuery(by, value string) (query string, useXPath bool, err error) {
	switch by {
	case "css":
		return value, false, nil
	case "id":
		return fmt.Sprintf("[id=%q]", value), false, nil
	case "name":
		return fmt.Sprintf("[name=%q]", value), false, nil
	case "tag":
		return value, false, nil
	case "className":
		return "." + value, false, nil
	case "xpath":
		return value, true, nil
	case "linkText":
		return fmt.Sprintf(`//a[normalize-space(.)=%s]`, xpathLiteral(value)), true, nil
	case "partialLinkText":
		return fmt.Sprintf(`//a[contains(normalize-space(.), %s)]`, xpathLiteral(value)), true, nil
	default:
		return "", false, fmt.Errorf("webdriver.cdpClick: unknown locator strategy %q (use css/xpath/id/name/tag/className/linkText/partialLinkText)", by)
	}
}

// xpathLiteral wraps s as a valid XPath 1.0 string literal. XPath has no escape
// mechanism, so it picks single or double quotes by content, and falls back to
// concat() when s contains both kinds of quote.
func xpathLiteral(s string) string {
	if !strings.Contains(s, "'") {
		return "'" + s + "'"
	}
	if !strings.Contains(s, `"`) {
		return `"` + s + `"`
	}
	// Contains both ' and ": concat('part', "'", 'part', …).
	parts := strings.Split(s, "'")
	var b strings.Builder
	b.WriteString("concat(")
	for i, p := range parts {
		if i > 0 {
			b.WriteString(`, "'", `)
		}
		b.WriteString("'" + p + "'")
	}
	b.WriteString(")")
	return b.String()
}

// quadCenter returns the centre of a CDP DOM quad (8-number [x1,y1..x4,y4]),
// offset by dx/dy.
func quadCenter(quad []float64, dx, dy float64) (x, y float64) {
	for i := 0; i+1 < 8; i += 2 {
		x += quad[i] / 4
		y += quad[i+1] / 4
	}
	return x + dx, y + dy
}

// mouseButtonsMask returns the Input.dispatchMouseEvent `buttons` bitmask for a
// button name (left=1, right=2, middle=4; default left).
func mouseButtonsMask(button string) int {
	switch button {
	case "right":
		return 2
	case "middle":
		return 4
	case "left":
		return 1
	default:
		return 1
	}
}

// optFloat reads a numeric option as float64, falling back when absent/non-numeric.
func optFloat(opts map[string]any, key string, fallback float64) float64 {
	if opts == nil {
		return fallback
	}
	switch t := opts[key].(type) {
	case float64:
		return t
	case int64:
		return float64(t)
	case int:
		return float64(t)
	}
	return fallback
}

// asStr returns v as a string, or "" if it is not a string.
func asStr(v any) string { s, _ := v.(string); return s }

// toFloatSlice converts a decoded JSON []any of numbers to []float64.
func toFloatSlice(arr []any) []float64 {
	out := make([]float64, 0, len(arr))
	for _, v := range arr {
		if f, ok := v.(float64); ok {
			out = append(out, f)
		}
	}
	return out
}

// collectDocumentNodeIDs gathers the nodeId of every #document node in a
// DOM.getDocument(pierce) tree — the top document plus each iframe's
// contentDocument (including cross-origin frames, which CDP exposes).
func collectDocumentNodeIDs(node map[string]any, out *[]float64) {
	if strings.EqualFold(asStr(node["nodeName"]), "#document") {
		if id, ok := node["nodeId"].(float64); ok {
			*out = append(*out, id)
		}
	}
	if kids, ok := node["children"].([]any); ok {
		for _, k := range kids {
			if km, ok := k.(map[string]any); ok {
				collectDocumentNodeIDs(km, out)
			}
		}
	}
	if cd, ok := node["contentDocument"].(map[string]any); ok {
		collectDocumentNodeIDs(cd, out)
	}
}

// cdpContentQuad returns the first content quad (8 floats) of a node, or
// ok=false when the node has no layout box (not visible / zero-size / detached).
func (s *wdSession) cdpContentQuad(nodeID float64) ([]float64, bool) {
	res, err := s.cdpExec("DOM.getContentQuads", map[string]any{"nodeId": nodeID})
	if err != nil {
		return nil, false
	}
	m, _ := res.(map[string]any)
	qs, _ := m["quads"].([]any)
	if len(qs) == 0 {
		return nil, false
	}
	q, _ := qs[0].([]any)
	if len(q) < 8 {
		return nil, false
	}
	out := make([]float64, 8)
	for i := 0; i < 8; i++ {
		f, ok := q[i].(float64)
		if !ok {
			return nil, false
		}
		out[i] = f
	}
	return out, true
}

// cdpQueryCSS finds nodes matching a CSS selector in every document of the
// pierced frame tree (top + each contentDocument), flattening the results.
func (s *wdSession) cdpQueryCSS(selector string) ([]float64, error) {
	res, err := s.cdpExec("DOM.getDocument", map[string]any{"depth": -1, "pierce": true})
	if err != nil {
		return nil, err
	}
	m, _ := res.(map[string]any)
	root, _ := m["root"].(map[string]any)
	if root == nil {
		return nil, nil
	}
	var docIDs []float64
	collectDocumentNodeIDs(root, &docIDs)
	var out []float64
	for _, docID := range docIDs {
		qres, qerr := s.cdpExec("DOM.querySelectorAll", map[string]any{"nodeId": docID, "selector": selector})
		if qerr != nil {
			continue // a re-navigated/detached document can error; skip it
		}
		qm, _ := qres.(map[string]any)
		arr, _ := qm["nodeIds"].([]any)
		out = append(out, toFloatSlice(arr)...)
	}
	return out, nil
}

// cdpSearchXPath finds nodes matching an XPath across the whole pierced tree via
// DOM.performSearch (the only frame-piercing XPath primitive in the DOM domain).
func (s *wdSession) cdpSearchXPath(query string) ([]float64, error) {
	res, err := s.cdpExec("DOM.performSearch", map[string]any{"query": query, "includeUserAgentShadowDOM": true})
	if err != nil {
		return nil, err
	}
	m, _ := res.(map[string]any)
	searchID, _ := m["searchId"].(string)
	count, _ := m["resultCount"].(float64)
	if searchID == "" || count <= 0 {
		if searchID != "" {
			_, _ = s.cdpExec("DOM.discardSearchResults", map[string]any{"searchId": searchID})
		}
		return nil, nil
	}
	defer func() { _, _ = s.cdpExec("DOM.discardSearchResults", map[string]any{"searchId": searchID}) }()
	got, err := s.cdpExec("DOM.getSearchResults", map[string]any{"searchId": searchID, "fromIndex": 0, "toIndex": int(count)})
	if err != nil {
		return nil, err
	}
	gm, _ := got.(map[string]any)
	arr, _ := gm["nodeIds"].([]any)
	return toFloatSlice(arr), nil
}

// cdpLocate finds the first laid-out element matching (by,value) anywhere in the
// page including nested cross-origin frames, returning its nodeId + content
// quad. found=false (with nil error) means no visible match yet.
func (s *wdSession) cdpLocate(by, value string) (nodeID float64, quad []float64, found bool, err error) {
	query, useXPath, err := cdpQuery(by, value)
	if err != nil {
		return 0, nil, false, err
	}
	var ids []float64
	if useXPath {
		ids, err = s.cdpSearchXPath(query)
	} else {
		ids, err = s.cdpQueryCSS(query)
	}
	if err != nil {
		return 0, nil, false, err
	}
	for _, id := range ids {
		if q, ok := s.cdpContentQuad(id); ok {
			return id, q, true, nil
		}
	}
	return 0, nil, false, nil
}
