package main

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// xmlOpts are the codec.xml.encode options.
type xmlOpts struct {
	rootName    string
	indent      string
	declaration bool
}

// xmlNamespace wires codec.xml.* — value ↔ XML via the shared dump IR
// (jsToIR / irToJS), using the @-prefix (attributes) + #text (text content)
// convention. encode/decode are synchronous and throw on error.
func xmlNamespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value { panic(vm.NewGoError(err)) }
	return map[string]any{
		"encode": func(call goja.FunctionCall) goja.Value {
			n, err := jsToIR(vm, call.Argument(0), dumpOpts{})
			if err != nil {
				return throw(err)
			}
			s, err := irToXMLDoc(n, xmlOptsFromArg(call.Argument(1)))
			if err != nil {
				return throw(err)
			}
			return vm.ToValue(s)
		},
		"decode": func(call goja.FunctionCall) goja.Value {
			n, err := xmlToIR(call.Argument(0).String())
			if err != nil {
				return throw(err)
			}
			return irToJS(vm, n, dumpOpts{})
		},
	}
}

// xmlOptsFromArg reads { rootName?, indent?, declaration? } from a JS argument.
func xmlOptsFromArg(arg goja.Value) xmlOpts {
	var o xmlOpts
	if arg == nil || goja.IsUndefined(arg) || goja.IsNull(arg) {
		return o
	}
	obj, ok := arg.(*goja.Object)
	if !ok {
		return o
	}
	if v := obj.Get("rootName"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		o.rootName = v.String()
	}
	if v := obj.Get("indent"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		o.indent = v.String()
	}
	if v := obj.Get("declaration"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
		o.declaration = v.ToBoolean()
	}
	return o
}

// irToXMLDoc renders an IR node as a full XML document: it resolves the root
// element (a single-key map, or opts.rootName wrapping the value) and
// optionally prepends an XML declaration.
func irToXMLDoc(n *irNode, opts xmlOpts) (string, error) {
	var name string
	var content *irNode
	if opts.rootName != "" {
		name, content = opts.rootName, n
	} else {
		if (n.kind != dumpMap && n.kind != dumpClass) || len(n.pairs) != 1 {
			return "", fmt.Errorf("codec.xml.encode: top-level value must be a single-key object naming the root element (or pass opts.rootName)")
		}
		name, content = n.pairs[0].key, n.pairs[0].val
	}
	if content.kind == dumpArray {
		return "", fmt.Errorf("codec.xml.encode: root element content cannot be an array; nest it under a key")
	}
	var b strings.Builder
	if opts.declaration {
		b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	}
	if err := irToXML(&b, name, content, opts, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// irToXML writes a single element named `name` whose content is node n.
// @-prefixed map keys are attributes, #text is text content, and any other key
// is a child element (an array value emits repeated siblings). Scalars become
// text-only elements; null becomes a self-closing empty element.
func irToXML(b *strings.Builder, name string, n *irNode, opts xmlOpts, depth int) error {
	if opts.indent != "" {
		b.WriteString(strings.Repeat(opts.indent, depth))
	}
	switch n.kind {
	case dumpMap, dumpClass:
		var attrs, children []irPair
		var text *irNode
		for _, p := range n.pairs {
			switch {
			case strings.HasPrefix(p.key, "@"):
				attrs = append(attrs, p)
			case p.key == "#text":
				text = p.val
			default:
				children = append(children, p)
			}
		}
		b.WriteByte('<')
		b.WriteString(name)
		for _, a := range attrs {
			s, err := scalarToXMLString(a.val)
			if err != nil {
				return fmt.Errorf("codec.xml.encode: attribute %s: %w", a.key, err)
			}
			b.WriteByte(' ')
			b.WriteString(a.key[1:])
			b.WriteString("=\"")
			b.WriteString(attrEscape(s))
			b.WriteByte('"')
		}
		if text == nil && len(children) == 0 {
			b.WriteString("/>")
			if opts.indent != "" {
				b.WriteByte('\n')
			}
			return nil
		}
		b.WriteByte('>')
		if text != nil {
			s, err := scalarToXMLString(text)
			if err != nil {
				return fmt.Errorf("codec.xml.encode: #text: %w", err)
			}
			if err := xml.EscapeText(b, []byte(s)); err != nil {
				return err
			}
		}
		if len(children) > 0 && opts.indent != "" {
			b.WriteByte('\n')
		}
		for _, c := range children {
			if c.val.kind == dumpArray {
				for _, item := range c.val.items {
					if err := irToXML(b, c.key, item, opts, depth+1); err != nil {
						return err
					}
				}
			} else if err := irToXML(b, c.key, c.val, opts, depth+1); err != nil {
				return err
			}
		}
		if len(children) > 0 && opts.indent != "" {
			b.WriteString(strings.Repeat(opts.indent, depth))
		}
		b.WriteString("</")
		b.WriteString(name)
		b.WriteByte('>')
	case dumpNull:
		b.WriteByte('<')
		b.WriteString(name)
		b.WriteString("/>")
	case dumpArray:
		return fmt.Errorf("codec.xml.encode: unexpected array at element %q (arrays must be a keyed value)", name)
	default: // scalar → text-only element
		s, err := scalarToXMLString(n)
		if err != nil {
			return err
		}
		b.WriteByte('<')
		b.WriteString(name)
		b.WriteByte('>')
		if err := xml.EscapeText(b, []byte(s)); err != nil {
			return err
		}
		b.WriteString("</")
		b.WriteString(name)
		b.WriteByte('>')
	}
	if opts.indent != "" {
		b.WriteByte('\n')
	}
	return nil
}

// scalarToXMLString renders a scalar IR node as its XML string form.
func scalarToXMLString(n *irNode) (string, error) {
	switch n.kind {
	case dumpString:
		return n.s, nil
	case dumpBool:
		if n.b {
			return "true", nil
		}
		return "false", nil
	case dumpInt:
		return strconv.FormatInt(n.i, 10), nil
	case dumpFloat:
		return strconv.FormatFloat(n.f, 'g', -1, 64), nil
	case dumpNull:
		return "", nil
	default:
		return "", fmt.Errorf("expected a scalar value (string/number/bool/null) for an attribute or text node")
	}
}

// attrEscape escapes a value for use inside a double-quoted XML attribute.
// Tab/newline/CR are escaped as numeric entities because conformant parsers
// normalise literal whitespace in attributes to spaces on parse.
func attrEscape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"\t", "&#x9;",
		"\n", "&#xA;",
		"\r", "&#xD;",
	).Replace(s)
}

// rawName reconstructs a literal element/attribute name from an xml.Name read
// via RawToken, which puts the prefix in Space and the local part in Local
// (no namespace-URL resolution). e.g. {Space:"ns", Local:"tag"} -> "ns:tag".
func rawName(n xml.Name) string {
	if n.Space != "" {
		return n.Space + ":" + n.Local
	}
	return n.Local
}

// xmlToIR parses an XML string into the shared IR, using the @-prefix + #text
// convention. RawToken keeps namespace prefixes literal. A text-only element
// (no attrs, no children) becomes a bare string; repeated same-name children
// become an array. Returns a single-pair dumpMap { rootName: rootContent }.
func xmlToIR(src string) (*irNode, error) {
	type frame struct {
		name     string
		attrs    []irPair
		children []irPair
		idx      map[string]int
		text     strings.Builder
	}
	addChild := func(f *frame, name string, node *irNode) {
		if f.idx == nil {
			f.idx = map[string]int{}
		}
		if i, ok := f.idx[name]; ok {
			ex := f.children[i].val
			if ex.kind == dumpArray {
				ex.items = append(ex.items, node)
			} else {
				f.children[i].val = &irNode{kind: dumpArray, items: []*irNode{ex, node}}
			}
			return
		}
		f.idx[name] = len(f.children)
		f.children = append(f.children, irPair{key: name, val: node})
	}
	finalize := func(f *frame) *irNode {
		text := strings.TrimSpace(f.text.String())
		if len(f.attrs) == 0 && len(f.children) == 0 {
			if text == "" {
				return &irNode{kind: dumpNull}
			}
			return nodeString(text)
		}
		pairs := make([]irPair, 0, len(f.attrs)+1+len(f.children))
		pairs = append(pairs, f.attrs...)
		if text != "" {
			pairs = append(pairs, irPair{key: "#text", val: nodeString(text)})
		}
		pairs = append(pairs, f.children...)
		return &irNode{kind: dumpMap, pairs: pairs}
	}

	dec := xml.NewDecoder(strings.NewReader(src))
	var stack []*frame
	var root *irNode
	rootName := ""
	for {
		tok, err := dec.RawToken()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("codec.xml.decode: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			f := &frame{name: rawName(t.Name)}
			for _, a := range t.Attr {
				f.attrs = append(f.attrs, irPair{key: "@" + rawName(a.Name), val: nodeString(a.Value)})
			}
			stack = append(stack, f)
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(t)
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("codec.xml.decode: unexpected end element </%s>", rawName(t.Name))
			}
			f := stack[len(stack)-1]
			if got := rawName(t.Name); got != f.name {
				return nil, fmt.Errorf("codec.xml.decode: mismatched end element </%s> (expected </%s>)", got, f.name)
			}
			stack = stack[:len(stack)-1]
			node := finalize(f)
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("codec.xml.decode: multiple root elements (<%s> after <%s>)", f.name, rootName)
				}
				root, rootName = node, f.name
			} else {
				addChild(stack[len(stack)-1], f.name, node)
			}
		}
	}
	if root == nil {
		return nil, fmt.Errorf("codec.xml.decode: no root element")
	}
	return &irNode{kind: dumpMap, pairs: []irPair{{key: rootName, val: root}}}, nil
}
