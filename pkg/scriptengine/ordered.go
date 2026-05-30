package scriptengine

import (
	"bytes"
	"encoding/json"

	"github.com/dop251/goja"
)

// Ordered is an insertion-ordered map a binding returns when the resulting JS
// object's key order must be deterministic. goja derives a JS object's
// property enumeration order from Go map iteration, which Go randomizes per
// process — so a binding that returns a map[string]any shuffles
// JSON.stringify output run-to-run, breaking canonical-serialization hashing
// (payment signing, webhook signatures). Build an Ordered instead: the engine
// converts it to a goja object preserving insertion order, and it is safe to
// construct on a background goroutine (it holds only plain Go data; the goja
// object is built later, on the loop, by PromisifyAsync's resolve step).
//
// Values may be primitives, []any (arrays, whose elements may themselves be
// *Ordered), or nested *Ordered.
type Ordered struct {
	keys []string
	vals []any
}

// NewOrdered returns an empty ordered map.
func NewOrdered() *Ordered { return &Ordered{} }

// Set appends key→val (insertion order is the emitted JS key order) and returns
// the receiver for chaining. Callers control order by call order; duplicate
// keys are emitted as written (goja keeps the last on object build).
func (o *Ordered) Set(key string, val any) *Ordered {
	o.keys = append(o.keys, key)
	o.vals = append(o.vals, val)
	return o
}

// Len reports the number of entries.
func (o *Ordered) Len() int { return len(o.keys) }

// OrderedToValue converts a result into a goja.Value, building any *Ordered
// (including nested ones and slices of them) into ordered goja objects.
// Anything that isn't Ordered-shaped falls through to vm.ToValue, so existing
// map/struct/primitive results are unaffected. PromisifyAsync calls it on the
// loop for async results; synchronous, on-loop bindings (those returning
// goja.Value directly) call it to convert an *Ordered they built or decoded.
func OrderedToValue(vm *goja.Runtime, val any) goja.Value {
	return vm.ToValue(convertOrdered(vm, val))
}

func convertOrdered(vm *goja.Runtime, v any) any {
	switch x := v.(type) {
	case *Ordered:
		obj := vm.NewObject()
		for i, k := range x.keys {
			_ = obj.Set(k, convertOrdered(vm, x.vals[i]))
		}
		return obj
	case []*Ordered:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = convertOrdered(vm, e)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = convertOrdered(vm, e)
		}
		return out
	default:
		return v
	}
}

// DecodeOrderedJSON parses JSON preserving object key order: objects become
// *Ordered (keys in source order), arrays become []any, and primitives become
// string / float64 / bool / nil. Use it for bindings that pass through
// arbitrary external JSON (decoded tokens, command output, HTTP bodies) so the
// re-serialized key order matches the source rather than being shuffled by a
// map. The returned value is ready to hand back through a binding (an *Ordered
// resolves to an ordered goja object).
func DecodeOrderedJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	v, err := decodeOrderedValue(dec)
	if err != nil {
		return nil, err
	}
	return v, nil
}

func decodeOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); ok {
		switch delim {
		case '{':
			o := NewOrdered()
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				key, _ := keyTok.(string)
				val, err := decodeOrderedValue(dec)
				if err != nil {
					return nil, err
				}
				o.Set(key, val)
			}
			if _, err := dec.Token(); err != nil { // consume '}'
				return nil, err
			}
			return o, nil
		case '[':
			arr := []any{}
			for dec.More() {
				val, err := decodeOrderedValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, val)
			}
			if _, err := dec.Token(); err != nil { // consume ']'
				return nil, err
			}
			return arr, nil
		}
	}
	// primitive: string, float64, bool, or nil
	return tok, nil
}
