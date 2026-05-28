package tui_test

import (
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

// ParseLayout accepts the map representation Goja produces for a JS object
// argument (vm.Value.Export() yields map[string]any / []any). The tests
// drive it with maps directly so they don't need a goja runtime.

func TestParseLayout_Leaf(t *testing.T) {
	n, err := tui.ParseLayout(map[string]any{"name": "log"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !n.IsLeaf() || n.Name != "log" || n.Weight != 1 {
		t.Fatalf("got %+v", n)
	}
}

func TestParseLayout_LeafWithTitleAndWeight(t *testing.T) {
	n, err := tui.ParseLayout(map[string]any{
		"name":   "log",
		"title":  "Orchestrator",
		"weight": int64(3),
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if n.Title != "Orchestrator" || n.Weight != 3 {
		t.Fatalf("got %+v", n)
	}
}

func TestParseLayout_Rows(t *testing.T) {
	n, err := tui.ParseLayout(map[string]any{
		"rows": []any{
			map[string]any{"name": "a"},
			map[string]any{"name": "b"},
		},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !n.IsRows() || len(n.Rows) != 2 || n.Rows[0].Name != "a" || n.Rows[1].Name != "b" {
		t.Fatalf("got %+v", n)
	}
}

func TestParseLayout_Cols(t *testing.T) {
	n, err := tui.ParseLayout(map[string]any{
		"cols": []any{
			map[string]any{"name": "a"},
			map[string]any{"name": "b"},
		},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !n.IsCols() || len(n.Cols) != 2 {
		t.Fatalf("got %+v", n)
	}
}

func TestParseLayout_Nested(t *testing.T) {
	n, err := tui.ParseLayout(map[string]any{
		"rows": []any{
			map[string]any{"name": "log", "weight": int64(1)},
			map[string]any{
				"cols":   []any{map[string]any{"name": "brew"}, map[string]any{"name": "npm"}},
				"weight": int64(2),
			},
		},
	})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !n.IsRows() || len(n.Rows) != 2 {
		t.Fatalf("top: %+v", n)
	}
	if n.Rows[1].Weight != 2 || !n.Rows[1].IsCols() || len(n.Rows[1].Cols) != 2 {
		t.Fatalf("nested cols: %+v", n.Rows[1])
	}
	// AllNames walks the tree in declaration order.
	names := n.AllNames()
	want := []string{"log", "brew", "npm"}
	if len(names) != 3 || names[0] != want[0] || names[1] != want[1] || names[2] != want[2] {
		t.Fatalf("AllNames: got %v, want %v", names, want)
	}
}

func TestParseLayout_DuplicateName(t *testing.T) {
	_, err := tui.ParseLayout(map[string]any{
		"rows": []any{
			map[string]any{"name": "x"},
			map[string]any{"cols": []any{map[string]any{"name": "x"}, map[string]any{"name": "y"}}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), `duplicate pane name "x"`) {
		t.Fatalf("expected duplicate-name error, got %v", err)
	}
}

func TestParseLayout_UnknownKey(t *testing.T) {
	_, err := tui.ParseLayout(map[string]any{"name": "x", "color": "red"})
	if err == nil || !strings.Contains(err.Error(), `unknown key "color"`) {
		t.Fatalf("expected unknown-key error, got %v", err)
	}
}

func TestParseLayout_EmptyRows(t *testing.T) {
	_, err := tui.ParseLayout(map[string]any{"rows": []any{}})
	if err == nil || !strings.Contains(err.Error(), "empty rows") {
		t.Fatalf("expected empty-rows error, got %v", err)
	}
}

func TestParseLayout_EmptyCols(t *testing.T) {
	_, err := tui.ParseLayout(map[string]any{"cols": []any{}})
	if err == nil || !strings.Contains(err.Error(), "empty cols") {
		t.Fatalf("expected empty-cols error, got %v", err)
	}
}

func TestParseLayout_MissingName(t *testing.T) {
	_, err := tui.ParseLayout(map[string]any{"title": "x"})
	if err == nil || !strings.Contains(err.Error(), "must have name|rows|cols") {
		t.Fatalf("expected missing-shape error, got %v", err)
	}
}

func TestParseLayout_BothNameAndRows(t *testing.T) {
	_, err := tui.ParseLayout(map[string]any{"name": "x", "rows": []any{map[string]any{"name": "y"}}})
	if err == nil || !strings.Contains(err.Error(), "node may have only one of name/rows/cols") {
		t.Fatalf("expected exclusive-shape error, got %v", err)
	}
}

func TestParseLayout_NegativeWeight(t *testing.T) {
	_, err := tui.ParseLayout(map[string]any{"name": "x", "weight": int64(-1)})
	if err == nil || !strings.Contains(err.Error(), "weight must be > 0") {
		t.Fatalf("expected weight error, got %v", err)
	}
}

func TestParseLayout_EmptyName(t *testing.T) {
	_, err := tui.ParseLayout(map[string]any{"name": ""})
	if err == nil || !strings.Contains(err.Error(), "name is empty") {
		t.Fatalf("expected empty-name error, got %v", err)
	}
}
