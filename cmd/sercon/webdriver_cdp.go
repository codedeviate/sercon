package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
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

// cdpExecFn runs one CDP command and returns its decoded result. Implementations
// either go through chromedriver's page-session passthrough (s.cdpExec) or a
// browser-level cdpConn session (cdpConn.callMap).
type cdpExecFn func(method string, params map[string]any) (map[string]any, error)

// passExec adapts the page-session passthrough (s.cdpExec) to a cdpExecFn.
// Callers must hold s.do (cdpExec issues an s.command).
func (s *wdSession) passExec(method string, params map[string]any) (map[string]any, error) {
	r, err := s.cdpExec(method, params)
	if err != nil {
		return nil, err
	}
	m, _ := r.(map[string]any)
	return m, nil
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
	if asStr(node["nodeName"]) == "#document" {
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

// cdpContentQuadX returns the first content quad (8 floats) of a node, or
// ok=false when it has no layout box. Transport-agnostic.
func cdpContentQuadX(exec cdpExecFn, nodeID float64) ([]float64, bool) {
	m, err := exec("DOM.getContentQuads", map[string]any{"nodeId": nodeID})
	if err != nil || m == nil {
		return nil, false
	}
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

// cdpQueryCSSX finds CSS matches in every document of the pierced frame tree.
func cdpQueryCSSX(exec cdpExecFn, selector string) ([]float64, error) {
	m, err := exec("DOM.getDocument", map[string]any{"depth": -1, "pierce": true})
	if err != nil {
		return nil, err
	}
	root, _ := m["root"].(map[string]any)
	if root == nil {
		return nil, nil
	}
	var docIDs []float64
	collectDocumentNodeIDs(root, &docIDs)
	var out []float64
	var lastErr error
	for _, docID := range docIDs {
		qm, qerr := exec("DOM.querySelectorAll", map[string]any{"nodeId": docID, "selector": selector})
		if qerr != nil {
			lastErr = qerr
			continue
		}
		lastErr = nil
		arr, _ := qm["nodeIds"].([]any)
		out = append(out, toFloatSlice(arr)...)
	}
	if len(out) == 0 && lastErr != nil {
		return nil, lastErr
	}
	return out, nil
}

// cdpSearchXPathX finds XPath matches across the pierced tree. performSearch
// needs DOM.enable + a pierced getDocument first so cross-origin frame matches
// get real (non-zero) frontend nodeIds.
func cdpSearchXPathX(exec cdpExecFn, query string) ([]float64, error) {
	if _, err := exec("DOM.enable", map[string]any{}); err != nil {
		return nil, err
	}
	if _, err := exec("DOM.getDocument", map[string]any{"depth": -1, "pierce": true}); err != nil {
		return nil, err
	}
	m, err := exec("DOM.performSearch", map[string]any{"query": query, "includeUserAgentShadowDOM": true})
	if err != nil {
		return nil, err
	}
	searchID, _ := m["searchId"].(string)
	count, _ := m["resultCount"].(float64)
	if searchID == "" || count <= 0 {
		if searchID != "" {
			_, _ = exec("DOM.discardSearchResults", map[string]any{"searchId": searchID})
		}
		return nil, nil
	}
	defer func() { _, _ = exec("DOM.discardSearchResults", map[string]any{"searchId": searchID}) }()
	gm, err := exec("DOM.getSearchResults", map[string]any{"searchId": searchID, "fromIndex": 0, "toIndex": int(count)})
	if err != nil {
		return nil, err
	}
	arr, _ := gm["nodeIds"].([]any)
	return toFloatSlice(arr), nil
}

// cdpLocateX finds the first laid-out element matching (by,value), returning its
// nodeId + content quad (coordinates local to exec's session/widget).
func cdpLocateX(exec cdpExecFn, by, value string) (nodeID float64, quad []float64, found bool, err error) {
	query, useXPath, err := cdpQuery(by, value)
	if err != nil {
		return 0, nil, false, err
	}
	var ids []float64
	if useXPath {
		ids, err = cdpSearchXPathX(exec, query)
	} else {
		ids, err = cdpQueryCSSX(exec, query)
	}
	if err != nil {
		return 0, nil, false, err
	}
	for _, id := range ids {
		if q, ok := cdpContentQuadX(exec, id); ok {
			return id, q, true, nil
		}
	}
	return 0, nil, false, nil
}

// cdpClickImpl waits for an element matching (by,value) to appear and lay out
// anywhere in the frame tree, scrolls it into view, and dispatches a trusted
// mouse press/release at its centre. Manages its own per-step s.do locking, so
// callers must NOT wrap it in s.do.
func (s *wdSession) cdpClickImpl(by, value string, opts map[string]any) (any, error) {
	if s.browser == "firefox" {
		return nil, fmt.Errorf("webdriver.cdpClick: CDP is Chrome-only (chromedriver); current browser is %q", s.browser)
	}
	// Validate the strategy up front for a clean error before any waiting.
	if _, _, err := cdpQuery(by, value); err != nil {
		return nil, err
	}

	timeout := optInt(opts, "timeout", 10000)
	poll := optInt(opts, "poll", 50)
	if poll <= 0 {
		poll = 50
	}
	button := "left"
	if b, ok := opts["button"].(string); ok && b != "" {
		button = b
	}
	scroll := optBool(opts, "scrollIntoView", true)
	dx := optFloat(opts, "offsetX", 0)
	dy := optFloat(opts, "offsetY", 0)

	var nodeID float64
	var quad []float64
	deadline := time.Now().Add(time.Duration(timeout) * time.Millisecond)
	for {
		var found bool
		if _, err := s.do(func() (any, error) {
			id, q, f, e := cdpLocateX(s.passExec, by, value)
			nodeID, quad, found = id, q, f
			return nil, e
		}); err != nil {
			return nil, fmt.Errorf("webdriver.cdpClick: %w", err)
		}
		if found {
			break
		}
		if !time.Now().Before(deadline) {
			return nil, fmt.Errorf("webdriver.cdpClick: no element matching %s=%q within %dms", by, value, timeout)
		}
		time.Sleep(time.Duration(poll) * time.Millisecond)
	}

	res, err := s.do(func() (any, error) {
		if scroll {
			_, _ = s.cdpExec("DOM.scrollIntoViewIfNeeded", map[string]any{"nodeId": nodeID})
			if q, ok := cdpContentQuadX(s.passExec, nodeID); ok {
				quad = q
			}
		}
		x, y := quadCenter(quad, dx, dy)
		mask := mouseButtonsMask(button)
		events := []map[string]any{
			{"type": "mouseMoved", "x": x, "y": y, "button": "none", "buttons": 0},
			{"type": "mousePressed", "x": x, "y": y, "button": button, "buttons": mask, "clickCount": 1},
			{"type": "mouseReleased", "x": x, "y": y, "button": button, "buttons": 0, "clickCount": 1},
		}
		for _, p := range events {
			if _, e := s.cdpExec("Input.dispatchMouseEvent", p); e != nil {
				return nil, e
			}
		}
		o := scriptengine.NewOrdered()
		o.Set("clicked", true)
		o.Set("x", x)
		o.Set("y", y)
		return o, nil
	})
	if err != nil {
		return nil, fmt.Errorf("webdriver.cdpClick: %w", err)
	}
	return res, nil
}

// targetIDFromExport extracts a targetId from an exported JS arg: a string id,
// or an object with a targetId field.
func targetIDFromExport(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if m, ok := v.(map[string]any); ok {
		return asStr(m["targetId"])
	}
	return ""
}

// projectTargets reduces a Target.getTargets targetInfos array to the fields the
// script API exposes.
func projectTargets(infos []any) []map[string]any {
	out := make([]map[string]any, 0, len(infos))
	for _, ti := range infos {
		m, _ := ti.(map[string]any)
		if m == nil {
			continue
		}
		out = append(out, map[string]any{
			"targetId": asStr(m["targetId"]),
			"type":     asStr(m["type"]),
			"url":      asStr(m["url"]),
			"title":    asStr(m["title"]),
		})
	}
	return out
}

// targetSessionObject builds the goja handle for an attached target session.
func (s *wdSession) targetSessionObject(c *cdpConn, targetID, sessionID string, vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"targetId":  targetID,
		"sessionId": sessionID,
		"cdp": wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			method := strArg(call, 0)
			if method == "" {
				return nil, errors.New("webdriver: cdp method must be a non-empty string")
			}
			return c.callMap(sessionID, method, optsArgMap(call, 1))
		}),
		"detach": wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
			if _, err := c.callMap("", "Target.detachFromTarget", map[string]any{"sessionId": sessionID}); err != nil {
				return nil, err
			}
			o := scriptengine.NewOrdered()
			o.Set("detached", true)
			return o, nil
		}),
	}
}

// addCDP wires the Chrome-only CDP methods onto the session handle object.
func (s *wdSession) addCDP(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["cdp"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		cmd := strArg(call, 0)
		if cmd == "" {
			return nil, errors.New("webdriver.cdp: command must be a non-empty string")
		}
		params := optsArgMap(call, 1)
		return s.do(func() (any, error) { return s.cdpExec(cmd, params) })
	})
	obj["cdpClick"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		by := strArg(call, 0)
		value := strArg(call, 1)
		if by == "" || value == "" {
			return nil, errors.New("webdriver.cdpClick: (by, value) are required")
		}
		return s.cdpClickImpl(by, value, optsArgMap(call, 2))
	})
	obj["targets"] = wdAsync(vm, loop, func(_ context.Context, _ goja.FunctionCall) (any, error) {
		c, err := s.cdpConnect()
		if err != nil {
			return nil, err
		}
		_, _ = c.callMap("", "Target.setDiscoverTargets", map[string]any{"discover": true})
		res, err := c.callMap("", "Target.getTargets", nil)
		if err != nil {
			return nil, err
		}
		infos, _ := res["targetInfos"].([]any)
		return projectTargets(infos), nil
	})
	obj["attach"] = wdAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
		targetID := targetIDFromExport(call.Argument(0).Export())
		if targetID == "" {
			return nil, errors.New("webdriver.attach: a target id or target object is required")
		}
		c, err := s.cdpConnect()
		if err != nil {
			return nil, err
		}
		res, err := c.callMap("", "Target.attachToTarget", map[string]any{"targetId": targetID, "flatten": true})
		if err != nil {
			return nil, fmt.Errorf("webdriver.attach: %w", err)
		}
		sessionID := asStr(res["sessionId"])
		if sessionID == "" {
			return nil, errors.New("webdriver.attach: no sessionId returned")
		}
		return s.targetSessionObject(c, targetID, sessionID, vm, loop), nil
	})
}
