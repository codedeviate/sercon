package scriptengine

import (
	"testing"

	"github.com/dop251/goja"
)

func stringifyOrdered(t *testing.T, v any) string {
	t.Helper()
	vm := goja.New()
	vm.Set("__o", OrderedToValue(vm, v))
	res, err := vm.RunString(`JSON.stringify(__o)`)
	if err != nil {
		t.Fatalf("stringify: %v", err)
	}
	return res.String()
}

// OrderedToValue must build a goja object whose JSON.stringify key order is the
// insertion order (not alphabetical, not Go-map-randomized), recursing through
// nested Ordered and arrays.
func TestOrderedToValue_InsertionOrder(t *testing.T) {
	o := NewOrdered().
		Set("zebra", 1).
		Set("alpha", 2).
		Set("nested", NewOrdered().Set("y", []any{1, 2}).Set("x", 3))
	if got, want := stringifyOrdered(t, o), `{"zebra":1,"alpha":2,"nested":{"y":[1,2],"x":3}}`; got != want {
		t.Fatalf("ordered key order:\n got: %s\nwant: %s", got, want)
	}

	// A slice of *Ordered (e.g. SQL query rows) keeps each row's column order.
	rows := []*Ordered{
		NewOrdered().Set("id", 1).Set("name", "a"),
		NewOrdered().Set("id", 2).Set("name", "b"),
	}
	if got, want := stringifyOrdered(t, rows), `[{"id":1,"name":"a"},{"id":2,"name":"b"}]`; got != want {
		t.Fatalf("ordered rows:\n got: %s\nwant: %s", got, want)
	}
}

// DecodeOrderedJSON preserves object key order from the source bytes (objects →
// *Ordered), through nesting and arrays — so a re-stringify is byte-identical
// to the input's structure/order.
func TestDecodeOrderedJSON_PreservesOrder(t *testing.T) {
	src := `{"z":1,"a":[{"k":true},2],"m":"x","n":null}`
	v, err := DecodeOrderedJSON([]byte(src))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := stringifyOrdered(t, v); got != src {
		t.Fatalf("decoded order:\n got: %s\nwant: %s", got, src)
	}
}
