// Package tui owns the multi-pane terminal UI runtime for sercon scripts.
// The package has four units, each in its own file:
//
//   - layout.go : parsing and validation of the script-supplied layout
//     tree (this file).
//   - ansi.go   : translating subprocess ANSI SGR escapes into tview
//     color tags.
//   - pane.go   : per-pane line buffer with \r overwrite + scrollback
//     capacity.
//   - runtime.go: the tview-driven Controller that realises a LayoutNode
//     as a Flex tree of TextView leaves and runs the
//     application goroutine.
//
// runtime.go also dispatches to fallback.go's writer when stdout is not
// a TTY (CI / pipelines / make demo), so the same scripts run in both
// contexts.
package tui

import (
	"errors"
	"fmt"
)

// LayoutNode is a node in the script-declared layout tree. Exactly one
// of Name, Rows, Cols is set per node; ParseLayout enforces this.
//
//   - Name != "" → leaf pane; the value is its unique identifier and the
//     argument passed to api.tui.pane(name).
//   - Rows != nil → vertical split: children stack top-to-bottom.
//   - Cols != nil → horizontal split: children sit side-by-side.
//
// Weight (default 1) maps to tview.Flex.AddItem's proportion arg —
// children share their parent's space in proportion to their weights.
// Title (leaf only) is the initial pane title; the script can update
// it at runtime via pane.title(...).
type LayoutNode struct {
	Name   string
	Title  string
	Rows   []LayoutNode
	Cols   []LayoutNode
	Weight int
}

// IsLeaf reports whether this node is a named pane.
func (n LayoutNode) IsLeaf() bool { return n.Name != "" }

// IsRows reports whether this node is a vertical split.
func (n LayoutNode) IsRows() bool { return n.Rows != nil }

// IsCols reports whether this node is a horizontal split.
func (n LayoutNode) IsCols() bool { return n.Cols != nil }

// AllNames returns the names of every leaf in declaration order
// (depth-first, left-to-right).
func (n LayoutNode) AllNames() []string {
	var out []string
	n.WalkLeaves(func(leaf LayoutNode) { out = append(out, leaf.Name) })
	return out
}

// WalkLeaves invokes fn for every leaf in depth-first, left-to-right
// declaration order. Used by both AllNames and the runtime when wiring
// TextView widgets to pane state.
func (n LayoutNode) WalkLeaves(fn func(LayoutNode)) {
	if n.IsLeaf() {
		fn(n)
		return
	}
	children := n.Rows
	if n.IsCols() {
		children = n.Cols
	}
	for _, c := range children {
		c.WalkLeaves(fn)
	}
}

// ParseLayout validates the script-supplied tree (passed in as the Go
// representation produced by goja's Value.Export() — map[string]any /
// []any for objects/arrays, numbers as int64 or float64, strings as
// string). It enforces:
//
//   - every node has exactly one of name / rows / cols
//   - rows/cols arrays are non-empty
//   - every node has only the allowed keys (name, title, rows, cols,
//     weight); unknown keys error
//   - leaf names are non-empty strings, unique across the whole tree
//   - title is optional and only allowed on leaves
//   - weight is a positive integer (defaults to 1)
//
// Errors include the path through the tree (e.g. "rows[1].cols[0]") so
// scripts can locate the bad node quickly.
func ParseLayout(v any) (LayoutNode, error) {
	seen := map[string]bool{}
	return parseNode(v, "", seen)
}

var allowedKeys = map[string]bool{
	"name": true, "title": true, "rows": true, "cols": true, "weight": true,
}

func parseNode(v any, path string, seen map[string]bool) (LayoutNode, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return LayoutNode{}, fmt.Errorf("%s: layout node must be an object, got %T", pathLabel(path), v)
	}
	for k := range m {
		if !allowedKeys[k] {
			return LayoutNode{}, fmt.Errorf("%s: unknown key %q (allowed: name, title, rows, cols, weight)", pathLabel(path), k)
		}
	}
	hasName := m["name"] != nil
	hasRows := m["rows"] != nil
	hasCols := m["cols"] != nil
	switch {
	case hasName && (hasRows || hasCols):
		return LayoutNode{}, fmt.Errorf("%s: node may have only one of name/rows/cols", pathLabel(path))
	case hasRows && hasCols:
		return LayoutNode{}, fmt.Errorf("%s: node may have only one of name/rows/cols", pathLabel(path))
	case !hasName && !hasRows && !hasCols:
		return LayoutNode{}, fmt.Errorf("%s: node must have name|rows|cols", pathLabel(path))
	}

	weight := 1
	if w, ok := m["weight"]; ok {
		iw, ok := asInt(w)
		if !ok || iw <= 0 {
			return LayoutNode{}, fmt.Errorf("%s: weight must be > 0 integer, got %v", pathLabel(path), w)
		}
		weight = iw
	}

	if hasName {
		name, ok := m["name"].(string)
		if !ok {
			return LayoutNode{}, fmt.Errorf("%s: name must be a string, got %T", pathLabel(path), m["name"])
		}
		if name == "" {
			return LayoutNode{}, fmt.Errorf("%s: name is empty", pathLabel(path))
		}
		if seen[name] {
			return LayoutNode{}, fmt.Errorf("%s: duplicate pane name %q", pathLabel(path), name)
		}
		seen[name] = true
		title := ""
		if tv, ok := m["title"]; ok {
			ts, ok := tv.(string)
			if !ok {
				return LayoutNode{}, fmt.Errorf("%s: title must be a string, got %T", pathLabel(path), tv)
			}
			title = ts
		}
		return LayoutNode{Name: name, Title: title, Weight: weight}, nil
	}

	if _, hasTitle := m["title"]; hasTitle {
		return LayoutNode{}, fmt.Errorf("%s: title is only allowed on leaf (name) nodes", pathLabel(path))
	}

	key := "rows"
	if hasCols {
		key = "cols"
	}
	rawChildren, ok := m[key].([]any)
	if !ok {
		return LayoutNode{}, fmt.Errorf("%s: %s must be an array, got %T", pathLabel(path), key, m[key])
	}
	if len(rawChildren) == 0 {
		return LayoutNode{}, fmt.Errorf("%s: empty %s", pathLabel(path), key)
	}
	children := make([]LayoutNode, 0, len(rawChildren))
	for i, c := range rawChildren {
		child, err := parseNode(c, fmt.Sprintf("%s%s[%d]", pathPrefix(path), key, i), seen)
		if err != nil {
			return LayoutNode{}, err
		}
		children = append(children, child)
	}
	if hasRows {
		return LayoutNode{Rows: children, Weight: weight}, nil
	}
	return LayoutNode{Cols: children, Weight: weight}, nil
}

// asInt accepts the integer representations goja can produce
// (int64 from integer literals, float64 from non-integer numeric values
// that nevertheless have integer values).
func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int64:
		return int(x), true
	case int:
		return x, true
	case float64:
		if x == float64(int(x)) {
			return int(x), true
		}
	}
	return 0, false
}

// pathLabel returns "<root>" for an empty path, the path itself otherwise.
func pathLabel(p string) string {
	if p == "" {
		return "<root>"
	}
	return p
}

// pathPrefix returns p + "." if p is non-empty (so children can be
// formatted as "p.rows[0]"), or "" so children at top level read like
// "rows[0]" without a leading dot.
func pathPrefix(p string) string {
	if p == "" {
		return ""
	}
	return p + "."
}

// ErrEmpty is returned when ParseLayout is called with nil. Exported so
// the binding can produce a friendly message.
var ErrEmpty = errors.New("layout is empty")

// ParseLayoutOrEmpty is like ParseLayout but returns a sentinel error
// for nil input. Callers that already handle nil before calling ParseLayout
// can ignore this.
func ParseLayoutOrEmpty(v any) (LayoutNode, error) {
	if v == nil {
		return LayoutNode{}, ErrEmpty
	}
	return ParseLayout(v)
}
