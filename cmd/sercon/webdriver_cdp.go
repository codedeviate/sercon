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
