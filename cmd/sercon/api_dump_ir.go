package main

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/dop251/goja"
)

// dumpKind tags an irNode.
type dumpKind uint8

const (
	dumpNull dumpKind = iota
	dumpBool
	dumpInt
	dumpFloat
	dumpString
	dumpArray // ordered list (JS array / PHP list-array / Perl array ref)
	dumpMap   // ordered key->value (JS object / PHP assoc-array / Perl hash ref)
	dumpClass // ordered key->value + class name (PHP O:, Perl blessed hash/array ref)
)

// irNode is the shared intermediate representation that every format
// encodes from and decodes to. Composite nodes are pointer-shared: the
// same *irNode at two positions denotes a shared reference, which irToJS
// rebuilds as the same JS object. A well-formed IR is acyclic — jsToIR
// throws before producing a cycle, and each decoder throws when an input
// reference would close one. Encoders therefore never need cycle tracking.
type irNode struct {
	kind  dumpKind
	b     bool
	i     int64
	f     float64
	s     string
	items []*irNode // dumpArray
	pairs []irPair  // dumpMap, dumpClass (insertion order preserved)
	class string    // dumpClass
}

type irPair struct {
	key string
	val *irNode
}

// dumpOpts carries per-call options shared by the bridges and the format
// codecs. The zero value resolves to documented defaults via withDumpDefaults.
type dumpOpts struct {
	classKey      string // sentinel property name for classed values (default "__class")
	perlBoolClass string // class emitted for Perl booleans (default "JSON::XS::Boolean")
	indent        string // pretty-print unit for varExport/varDump/dumper ("" = each format's default)
}

func withDumpDefaults(o dumpOpts) dumpOpts {
	if o.classKey == "" {
		o.classKey = "__class"
	}
	if o.perlBoolClass == "" {
		o.perlBoolClass = "JSON::XS::Boolean"
	}
	return o
}

// perlBoolClasses is the allowlist parseDumper treats as JS booleans.
var perlBoolClasses = map[string]bool{
	"JSON::XS::Boolean":          true,
	"JSON::PP::Boolean":          true,
	"Types::Serialiser::Boolean": true,
}

// errCircular is the shared circular-reference error; each codec wraps it
// with its own prefix.
func errCircular(prefix string) error {
	return fmt.Errorf("%s: circular reference (cycles are not representable; same as JSON.stringify)", prefix)
}

func nodeNull() *irNode           { return &irNode{kind: dumpNull} }
func nodeBool(b bool) *irNode     { return &irNode{kind: dumpBool, b: b} }
func nodeInt(i int64) *irNode     { return &irNode{kind: dumpInt, i: i} }
func nodeFloat(f float64) *irNode { return &irNode{kind: dumpFloat, f: f} }
func nodeString(s string) *irNode { return &irNode{kind: dumpString, s: s} }

// jsToIR walks a goja value graph once. It tracks object identity to reuse
// the IR node for any value reached more than once off the current path
// (shared ref), and throws when a value is reached again while still on the
// current path (cycle). classKey-bearing objects become dumpClass.
func jsToIR(vm *goja.Runtime, v goja.Value, opts dumpOpts) (*irNode, error) {
	w := &jsWalker{vm: vm, opts: opts, done: map[*goja.Object]*irNode{}, path: map[*goja.Object]bool{}}
	return w.walk(v)
}

type jsWalker struct {
	vm   *goja.Runtime
	opts dumpOpts
	done map[*goja.Object]*irNode
	path map[*goja.Object]bool
}

func (w *jsWalker) walk(v goja.Value) (*irNode, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nodeNull(), nil
	}
	// A Symbol exports as its description string, so it must be rejected
	// before the Export() type switch would mistake it for a string.
	if _, isSym := v.(*goja.Symbol); isSym {
		return nil, errors.New("codec dump: unsupported type Symbol")
	}
	switch x := v.Export().(type) {
	case bool:
		return nodeBool(x), nil
	case int64:
		return nodeInt(x), nil
	case float64:
		if x == math.Trunc(x) && !math.IsInf(x, 0) && !math.IsNaN(x) {
			return nodeInt(int64(x)), nil
		}
		return nodeFloat(x), nil
	case string:
		return nodeString(x), nil
	case *big.Int:
		return nil, errors.New("codec dump: unsupported type BigInt")
	}

	obj := v.ToObject(w.vm)
	if _, callable := goja.AssertFunction(obj); callable {
		return nil, errors.New("codec dump: unsupported type function")
	}
	if w.path[obj] {
		return nil, errCircular("codec dump")
	}
	if n, seen := w.done[obj]; seen {
		return n, nil
	}
	w.path[obj] = true
	defer delete(w.path, obj)

	var n *irNode
	if obj.ClassName() == "Array" {
		n = &irNode{kind: dumpArray}
		length := int(obj.Get("length").ToInteger())
		n.items = make([]*irNode, 0, length)
		for i := 0; i < length; i++ {
			child, err := w.walk(obj.Get(fmt.Sprintf("%d", i)))
			if err != nil {
				return nil, err
			}
			n.items = append(n.items, child)
		}
	} else {
		keys := obj.Keys()
		class := ""
		if cv := obj.Get(w.opts.classKey); cv != nil && !goja.IsUndefined(cv) {
			class = cv.String()
		}
		if class != "" {
			n = &irNode{kind: dumpClass, class: class}
		} else {
			n = &irNode{kind: dumpMap}
		}
		for _, k := range keys {
			if class != "" && k == w.opts.classKey {
				continue
			}
			child, err := w.walk(obj.Get(k))
			if err != nil {
				return nil, err
			}
			n.pairs = append(n.pairs, irPair{key: k, val: child})
		}
	}
	w.done[obj] = n
	return n, nil
}

// irToJS builds goja values from an IR DAG. Maps and classed nodes become
// vm.NewObject() with ordered .Set() so JSON.stringify key order is stable.
// Shared IR nodes rebuild as the same JS object via the memo.
func irToJS(vm *goja.Runtime, n *irNode, opts dumpOpts) goja.Value {
	return (&irBuilder{vm: vm, opts: opts, memo: map[*irNode]goja.Value{}}).build(n)
}

type irBuilder struct {
	vm   *goja.Runtime
	opts dumpOpts
	memo map[*irNode]goja.Value
}

func (b *irBuilder) build(n *irNode) goja.Value {
	switch n.kind {
	case dumpNull:
		return goja.Null()
	case dumpBool:
		return b.vm.ToValue(n.b)
	case dumpInt:
		return b.vm.ToValue(n.i)
	case dumpFloat:
		return b.vm.ToValue(n.f)
	case dumpString:
		return b.vm.ToValue(n.s)
	}
	if v, ok := b.memo[n]; ok {
		return v
	}
	switch n.kind {
	case dumpArray:
		arr := b.vm.NewArray()
		b.memo[n] = arr
		for i, it := range n.items {
			_ = arr.Set(fmt.Sprintf("%d", i), b.build(it))
		}
		return arr
	default: // dumpMap, dumpClass
		o := b.vm.NewObject()
		b.memo[n] = o
		if n.kind == dumpClass {
			_ = o.Set(b.opts.classKey, n.class)
		}
		for _, p := range n.pairs {
			_ = o.Set(p.key, b.build(p.val))
		}
		return o
	}
}
