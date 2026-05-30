package main

import (
	"fmt"
	"strconv"
	"strings"
)

// phpVarExportEncode renders an IR node as PHP var_export() output: valid PHP
// code. Strings are single-quoted (only ' and \ escaped), arrays use the
// "array ( … )" form with a trailing comma on every entry, and classed nodes
// use \Cls::__set_state(array( … )). The pretty-print unit is two spaces per
// level (or opts.indent if non-empty).
func phpVarExportEncode(n *irNode, opts dumpOpts) (string, error) {
	unit := opts.indent
	if unit == "" {
		unit = "  "
	}
	var b strings.Builder
	if err := phpExpWrite(&b, n, unit, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// phpExpWrite writes n at the given nesting depth. depth is the indentation
// level of the value's own opening token; composite members sit at depth+1.
func phpExpWrite(b *strings.Builder, n *irNode, unit string, depth int) error {
	switch n.kind {
	case dumpNull:
		b.WriteString("NULL")
	case dumpBool:
		if n.b {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case dumpInt:
		b.WriteString(strconv.FormatInt(n.i, 10))
	case dumpFloat:
		b.WriteString(strconv.FormatFloat(n.f, 'G', -1, 64))
	case dumpString:
		b.WriteString(phpExpQuote(n.s))
	case dumpArray:
		b.WriteString("array (\n")
		inner := strings.Repeat(unit, depth+1)
		for i, it := range n.items {
			b.WriteString(inner)
			fmt.Fprintf(b, "%d => ", i)
			if err := phpExpWrite(b, it, unit, depth+1); err != nil {
				return err
			}
			b.WriteString(",\n")
		}
		b.WriteString(strings.Repeat(unit, depth))
		b.WriteByte(')')
	case dumpMap:
		b.WriteString("array (\n")
		inner := strings.Repeat(unit, depth+1)
		for _, p := range n.pairs {
			b.WriteString(inner)
			b.WriteString(phpExpKey(p.key))
			b.WriteString(" => ")
			if err := phpExpWrite(b, p.val, unit, depth+1); err != nil {
				return err
			}
			b.WriteString(",\n")
		}
		b.WriteString(strings.Repeat(unit, depth))
		b.WriteByte(')')
	case dumpClass:
		// PHP emits \Cls::__set_state(array( … )) with members indented three
		// spaces deeper than the opening line. We mirror that with depth+1 plus
		// a constant three-space lead so our own output round-trips.
		fmt.Fprintf(b, "\\%s::__set_state(array(\n", n.class)
		inner := strings.Repeat(unit, depth) + "   "
		for _, p := range n.pairs {
			b.WriteString(inner)
			b.WriteString(phpExpKey(p.key))
			b.WriteString(" => ")
			if err := phpExpWrite(b, p.val, unit, depth+1); err != nil {
				return err
			}
			b.WriteString(",\n")
		}
		b.WriteString(strings.Repeat(unit, depth))
		b.WriteString("))")
	default:
		return fmt.Errorf("php.varExport: unsupported node kind %d", n.kind)
	}
	return nil
}

// phpExpKey renders a map/object key: canonical ints unquoted, everything else
// single-quoted.
func phpExpKey(k string) string {
	if isCanonicalInt(k) {
		return k
	}
	return phpExpQuote(k)
}

// phpExpQuote single-quotes s, escaping only backslash and single-quote, as
// var_export does (newlines and other bytes appear literally).
func phpExpQuote(s string) string {
	var b strings.Builder
	b.WriteByte('\'')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('\'')
	return b.String()
}

// phpVarExportDecode parses the literal subset that var_export emits back into
// an IR node. It is whitespace-tolerant between tokens, panic-free on malformed
// input, and rejects trailing garbage after the top-level value.
func phpVarExportDecode(text string, opts dumpOpts) (*irNode, error) {
	c := &phpExpCursor{s: text}
	n, err := c.parseValue()
	if err != nil {
		return nil, err
	}
	c.skipSpace()
	if c.pos != len(c.s) {
		return nil, c.errf("trailing data")
	}
	return n, nil
}

type phpExpCursor struct {
	s   string
	pos int
}

func (c *phpExpCursor) errf(format string, args ...any) error {
	args = append(args, c.pos)
	return fmt.Errorf("php.parseVarExport: "+format+" at offset %d", args...)
}

func (c *phpExpCursor) skipSpace() {
	for c.pos < len(c.s) {
		switch c.s[c.pos] {
		case ' ', '\t', '\r', '\n':
			c.pos++
		default:
			return
		}
	}
}

// expect consumes the literal prefix at the cursor (after skipping leading
// whitespace) or errors.
func (c *phpExpCursor) expect(lit string) error {
	c.skipSpace()
	if !strings.HasPrefix(c.s[c.pos:], lit) {
		return c.errf("expected %q", lit)
	}
	c.pos += len(lit)
	return nil
}

func (c *phpExpCursor) parseValue() (*irNode, error) {
	c.skipSpace()
	if c.pos >= len(c.s) {
		return nil, c.errf("unexpected end of input")
	}
	ch := c.s[c.pos]
	switch {
	case ch == '\'':
		s, err := c.parseString()
		if err != nil {
			return nil, err
		}
		return nodeString(s), nil
	case ch == '\\':
		return c.parseClass()
	case strings.HasPrefix(c.s[c.pos:], "array"):
		return c.parseArray()
	case strings.HasPrefix(c.s[c.pos:], "NULL"):
		c.pos += len("NULL")
		return nodeNull(), nil
	case strings.HasPrefix(c.s[c.pos:], "true"):
		c.pos += len("true")
		return nodeBool(true), nil
	case strings.HasPrefix(c.s[c.pos:], "false"):
		c.pos += len("false")
		return nodeBool(false), nil
	case ch == '-' || ch == '+' || (ch >= '0' && ch <= '9'):
		return c.parseNumber()
	default:
		return nil, c.errf("unexpected token %q", ch)
	}
}

// parseString reads a single-quoted string, unescaping \' and \\ only.
func (c *phpExpCursor) parseString() (string, error) {
	if c.pos >= len(c.s) || c.s[c.pos] != '\'' {
		return "", c.errf("expected string")
	}
	c.pos++ // opening quote
	var b strings.Builder
	for c.pos < len(c.s) {
		ch := c.s[c.pos]
		switch ch {
		case '\\':
			if c.pos+1 >= len(c.s) {
				return "", c.errf("unterminated string escape")
			}
			next := c.s[c.pos+1]
			switch next {
			case '\\':
				b.WriteByte('\\')
			case '\'':
				b.WriteByte('\'')
			default:
				// var_export only escapes \ and '; any other backslash is a
				// literal backslash followed by the next char.
				b.WriteByte('\\')
				b.WriteByte(next)
			}
			c.pos += 2
		case '\'':
			c.pos++ // closing quote
			return b.String(), nil
		default:
			b.WriteByte(ch)
			c.pos++
		}
	}
	return "", c.errf("unterminated string")
}

// parseNumber reads an int or float literal. A literal containing '.', 'e',
// 'E', or matching INF/NAN is a float; otherwise an int.
func (c *phpExpCursor) parseNumber() (*irNode, error) {
	start := c.pos
	if c.pos < len(c.s) && (c.s[c.pos] == '-' || c.s[c.pos] == '+') {
		c.pos++
	}
	isFloat := false
	for c.pos < len(c.s) {
		ch := c.s[c.pos]
		switch {
		case ch >= '0' && ch <= '9':
			c.pos++
		case ch == '.' || ch == 'e' || ch == 'E' || ch == '+' || ch == '-':
			isFloat = true
			c.pos++
		default:
			goto done
		}
	}
done:
	tok := c.s[start:c.pos]
	if tok == "" || tok == "-" || tok == "+" {
		return nil, c.errf("invalid number %q", tok)
	}
	if isFloat {
		f, err := strconv.ParseFloat(tok, 64)
		if err != nil {
			return nil, c.errf("invalid float %q", tok)
		}
		return nodeFloat(f), nil
	}
	i, err := strconv.ParseInt(tok, 10, 64)
	if err != nil {
		return nil, c.errf("invalid int %q", tok)
	}
	return nodeInt(i), nil
}

// parseArray reads "array ( <k> => <v>, … )" and applies the list/assoc
// heuristic on the keys.
func (c *phpExpCursor) parseArray() (*irNode, error) {
	if err := c.expect("array"); err != nil {
		return nil, err
	}
	if err := c.expect("("); err != nil {
		return nil, err
	}
	keys, vals, err := c.parseEntries(")")
	if err != nil {
		return nil, err
	}
	return buildArrayNode(keys, vals), nil
}

// parseClass reads "\<Cls>::__set_state(array( … ))" into a dumpClass node.
func (c *phpExpCursor) parseClass() (*irNode, error) {
	if err := c.expect("\\"); err != nil {
		return nil, err
	}
	// Read the class name: a run of identifier / namespace-separator chars.
	start := c.pos
	for c.pos < len(c.s) {
		ch := c.s[c.pos]
		if ch == ':' || ch == '(' {
			break
		}
		c.pos++
	}
	class := c.s[start:c.pos]
	if class == "" {
		return nil, c.errf("expected class name")
	}
	if err := c.expect("::__set_state(array("); err != nil {
		return nil, err
	}
	keys, vals, err := c.parseEntries("))")
	if err != nil {
		return nil, err
	}
	n := &irNode{kind: dumpClass, class: class, pairs: make([]irPair, len(keys))}
	for i := range keys {
		n.pairs[i] = irPair{key: keys[i], val: vals[i]}
	}
	return n, nil
}

// parseEntries reads "<key> => <value>," repeated until the closing token
// (")" for arrays, "))" for __set_state). Keys are int literals or
// single-quoted strings. Whitespace between tokens is insignificant.
func (c *phpExpCursor) parseEntries(closeTok string) ([]string, []*irNode, error) {
	var keys []string
	var vals []*irNode
	for {
		c.skipSpace()
		if c.pos >= len(c.s) {
			return nil, nil, c.errf("unterminated array")
		}
		if strings.HasPrefix(c.s[c.pos:], closeTok) {
			c.pos += len(closeTok)
			return keys, vals, nil
		}
		key, err := c.parseKey()
		if err != nil {
			return nil, nil, err
		}
		if err := c.expect("=>"); err != nil {
			return nil, nil, err
		}
		val, err := c.parseValue()
		if err != nil {
			return nil, nil, err
		}
		if err := c.expect(","); err != nil {
			return nil, nil, err
		}
		keys = append(keys, key)
		vals = append(vals, val)
	}
}

// parseKey reads an array/object key: an int literal or a single-quoted string.
func (c *phpExpCursor) parseKey() (string, error) {
	c.skipSpace()
	if c.pos >= len(c.s) {
		return "", c.errf("unexpected end of input (expected key)")
	}
	ch := c.s[c.pos]
	switch {
	case ch == '\'':
		return c.parseString()
	case ch == '-' || (ch >= '0' && ch <= '9'):
		n, err := c.parseNumber()
		if err != nil {
			return "", err
		}
		if n.kind != dumpInt {
			return "", c.errf("non-integer array key")
		}
		return strconv.FormatInt(n.i, 10), nil
	default:
		return "", c.errf("invalid key token %q", ch)
	}
}

// buildArrayNode applies the JSON-style heuristic: keys exactly 0..n-1 in
// order (as canonical ints) → dumpArray, else dumpMap.
func buildArrayNode(keys []string, vals []*irNode) *irNode {
	isList := true
	for i, k := range keys {
		if k != strconv.Itoa(i) {
			isList = false
			break
		}
	}
	if isList {
		return &irNode{kind: dumpArray, items: vals}
	}
	n := &irNode{kind: dumpMap, pairs: make([]irPair, len(keys))}
	for i := range keys {
		n.pairs[i] = irPair{key: keys[i], val: vals[i]}
	}
	return n
}
