package main

import (
	"fmt"
	"strconv"
	"strings"
)

// perlDumperEncode renders an IR node as Data::Dumper-style output with
// normalized 2-space-per-level indentation. The whole value is wrapped as
// `$VAR1 = <value>;`. Booleans use the blessed-scalar-ref convention
// (`bless( do{\(my $o = 1)}, '<class>' )`); the class comes from
// opts.perlBoolClass. Strings are single-quoted with Perl single-quote
// escaping (only ' and \ escaped). Classed nodes emit a blessed HASH ref.
func perlDumperEncode(n *irNode, opts dumpOpts) (string, error) {
	unit := opts.indent
	if unit == "" {
		unit = "  "
	}
	var b strings.Builder
	b.WriteString("$VAR1 = ")
	if err := perlDumpWrite(&b, n, opts, unit, 0); err != nil {
		return "", err
	}
	b.WriteByte(';')
	return b.String(), nil
}

// perlDumpWrite writes n at the given nesting depth. depth is the indentation
// level of the value's own opening token; composite members sit at depth+1.
func perlDumpWrite(b *strings.Builder, n *irNode, opts dumpOpts, unit string, depth int) error {
	switch n.kind {
	case dumpNull:
		b.WriteString("undef")
	case dumpBool:
		v := "0"
		if n.b {
			v = "1"
		}
		fmt.Fprintf(b, "bless( do{\\(my $o = %s)}, %s )", v, perlDumpQuote(opts.perlBoolClass))
	case dumpInt:
		b.WriteString(strconv.FormatInt(n.i, 10))
	case dumpFloat:
		b.WriteString(strconv.FormatFloat(n.f, 'G', -1, 64))
	case dumpString:
		b.WriteString(perlDumpQuote(n.s))
	case dumpArray:
		return perlDumpArray(b, n.items, opts, unit, depth)
	case dumpMap:
		return perlDumpHash(b, n.pairs, opts, unit, depth)
	case dumpClass:
		b.WriteString("bless( ")
		if err := perlDumpHash(b, n.pairs, opts, unit, depth); err != nil {
			return err
		}
		fmt.Fprintf(b, ", %s )", perlDumpQuote(n.class))
	default:
		return fmt.Errorf("perl.dumper: unsupported node kind %d", n.kind)
	}
	return nil
}

func perlDumpArray(b *strings.Builder, items []*irNode, opts dumpOpts, unit string, depth int) error {
	if len(items) == 0 {
		b.WriteString("[]")
		return nil
	}
	b.WriteString("[\n")
	inner := strings.Repeat(unit, depth+1)
	for i, it := range items {
		b.WriteString(inner)
		if err := perlDumpWrite(b, it, opts, unit, depth+1); err != nil {
			return err
		}
		if i < len(items)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat(unit, depth))
	b.WriteByte(']')
	return nil
}

func perlDumpHash(b *strings.Builder, pairs []irPair, opts dumpOpts, unit string, depth int) error {
	if len(pairs) == 0 {
		b.WriteString("{}")
		return nil
	}
	b.WriteString("{\n")
	inner := strings.Repeat(unit, depth+1)
	for i, p := range pairs {
		b.WriteString(inner)
		b.WriteString(perlDumpQuote(p.key))
		b.WriteString(" => ")
		if err := perlDumpWrite(b, p.val, opts, unit, depth+1); err != nil {
			return err
		}
		if i < len(pairs)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString(strings.Repeat(unit, depth))
	b.WriteByte('}')
	return nil
}

// perlDumpQuote single-quotes s with Perl single-quote escaping: only the
// backslash and single-quote characters are escaped; everything else is
// literal.
func perlDumpQuote(s string) string {
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

// perlDumperDecode parses the default-settings subset of Data::Dumper output
// into an IR node. It is whitespace-insensitive between tokens, panic-free on
// malformed/truncated input, rejects trailing garbage, recognizes the JSON
// bool family, and errors on blessed scalar refs and self-referential cycles.
func perlDumperDecode(text string, opts dumpOpts) (*irNode, error) {
	c := &perlCursor{s: text}
	c.skipWS()
	if err := c.expect("$VAR1"); err != nil {
		return nil, err
	}
	c.skipWS()
	if err := c.expect("="); err != nil {
		return nil, err
	}
	c.skipWS()
	n, err := c.parseValue()
	if err != nil {
		return nil, err
	}
	c.skipWS()
	if err := c.expect(";"); err != nil {
		return nil, err
	}
	c.skipWS()
	// Any further `$VARn` assignment or `$VAR1->…` continuation is rejected.
	// A trailing self-referential alias (`$VAR1->{self} = $VAR1;`) implies a
	// cycle, so it is reported as a circular reference. A second top-level
	// `$VARn = …` is the multi-value case.
	if c.pos < len(c.s) {
		rest := c.s[c.pos:]
		if strings.HasPrefix(rest, "$VAR1->") {
			return nil, errCircular("perl.parseDumper")
		}
		if strings.HasPrefix(rest, "$VAR") {
			return nil, c.errf("multiple top-level values not supported")
		}
		return nil, c.errf("trailing data")
	}
	return n, nil
}

type perlCursor struct {
	s     string
	pos   int
	depth int
}

func (c *perlCursor) errf(format string, args ...any) error {
	args = append(args, c.pos)
	return fmt.Errorf("perl.parseDumper: "+format+" at offset %d", args...)
}

func (c *perlCursor) skipWS() {
	for c.pos < len(c.s) {
		switch c.s[c.pos] {
		case ' ', '\t', '\r', '\n':
			c.pos++
		default:
			return
		}
	}
}

// expect consumes the literal at the cursor or errors. It does not skip
// whitespace; callers skipWS as needed.
func (c *perlCursor) expect(lit string) error {
	if !strings.HasPrefix(c.s[c.pos:], lit) {
		return c.errf("expected %q", lit)
	}
	c.pos += len(lit)
	return nil
}

func (c *perlCursor) parseValue() (*irNode, error) {
	c.depth++
	if c.depth > MaxDecodeDepth {
		return nil, fmt.Errorf("perl.parseDumper: max nesting depth %d exceeded", MaxDecodeDepth)
	}
	defer func() { c.depth-- }()
	if c.pos >= len(c.s) {
		return nil, c.errf("unexpected end of input")
	}
	switch c.s[c.pos] {
	case 'u': // undef
		return c.parseUndef()
	case '\'':
		return c.parseString()
	case '[':
		return c.parseArray()
	case '{':
		return c.parseHash()
	case 'b': // bless( … )
		return c.parseBless()
	}
	if ch := c.s[c.pos]; ch == '-' || ch == '+' || (ch >= '0' && ch <= '9') {
		return c.parseNumber()
	}
	return nil, c.errf("unexpected token %q", c.s[c.pos])
}

func (c *perlCursor) parseUndef() (*irNode, error) {
	if err := c.expect("undef"); err != nil {
		return nil, err
	}
	return nodeNull(), nil
}

// parseString reads a single-quoted Perl string, unescaping \' and \\.
func (c *perlCursor) parseString() (*irNode, error) {
	s, err := c.parseStringRaw()
	if err != nil {
		return nil, err
	}
	return nodeString(s), nil
}

func (c *perlCursor) parseStringRaw() (string, error) {
	if c.pos >= len(c.s) || c.s[c.pos] != '\'' {
		return "", c.errf("expected single-quoted string")
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
			case '\'', '\\':
				b.WriteByte(next)
			default:
				// Perl single quotes only special-case \' and \\; any other
				// backslash is literal.
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

func (c *perlCursor) parseNumber() (*irNode, error) {
	start := c.pos
	if c.s[c.pos] == '-' || c.s[c.pos] == '+' {
		c.pos++
	}
	isFloat := false
	for c.pos < len(c.s) {
		ch := c.s[c.pos]
		if ch >= '0' && ch <= '9' {
			c.pos++
		} else if ch == '.' || ch == 'e' || ch == 'E' || ch == '+' || ch == '-' {
			isFloat = true
			c.pos++
		} else {
			break
		}
	}
	tok := c.s[start:c.pos]
	if tok == "" || tok == "-" || tok == "+" {
		return nil, c.errf("invalid number %q", tok)
	}
	if !isFloat {
		if i, err := strconv.ParseInt(tok, 10, 64); err == nil {
			return nodeInt(i), nil
		}
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return nil, c.errf("invalid number %q", tok)
	}
	return nodeFloat(f), nil
}

func (c *perlCursor) parseArray() (*irNode, error) {
	if err := c.expect("["); err != nil {
		return nil, err
	}
	// Append into a nil slice — never size from input.
	n := &irNode{kind: dumpArray}
	c.skipWS()
	if c.pos < len(c.s) && c.s[c.pos] == ']' {
		c.pos++
		return n, nil
	}
	for {
		c.skipWS()
		v, err := c.parseValue()
		if err != nil {
			return nil, err
		}
		n.items = append(n.items, v)
		c.skipWS()
		if c.pos >= len(c.s) {
			return nil, c.errf("unterminated array")
		}
		switch c.s[c.pos] {
		case ',':
			c.pos++
			c.skipWS()
			// Allow a trailing comma before the close bracket.
			if c.pos < len(c.s) && c.s[c.pos] == ']' {
				c.pos++
				return n, nil
			}
		case ']':
			c.pos++
			return n, nil
		default:
			return nil, c.errf("expected ',' or ']'")
		}
	}
}

func (c *perlCursor) parseHash() (*irNode, error) {
	if err := c.expect("{"); err != nil {
		return nil, err
	}
	n := &irNode{kind: dumpMap}
	c.skipWS()
	if c.pos < len(c.s) && c.s[c.pos] == '}' {
		c.pos++
		return n, nil
	}
	for {
		c.skipWS()
		key, err := c.parseHashKey()
		if err != nil {
			return nil, err
		}
		c.skipWS()
		if err := c.expect("=>"); err != nil {
			return nil, err
		}
		c.skipWS()
		v, err := c.parseValue()
		if err != nil {
			return nil, err
		}
		n.pairs = append(n.pairs, irPair{key: key, val: v})
		c.skipWS()
		if c.pos >= len(c.s) {
			return nil, c.errf("unterminated hash")
		}
		switch c.s[c.pos] {
		case ',':
			c.pos++
			c.skipWS()
			if c.pos < len(c.s) && c.s[c.pos] == '}' {
				c.pos++
				return n, nil
			}
		case '}':
			c.pos++
			return n, nil
		default:
			return nil, c.errf("expected ',' or '}'")
		}
	}
}

// parseHashKey reads a hash key: a single-quoted string or a bareword
// (Data::Dumper may emit unquoted simple-identifier keys).
func (c *perlCursor) parseHashKey() (string, error) {
	if c.pos >= len(c.s) {
		return "", c.errf("expected hash key")
	}
	if c.s[c.pos] == '\'' {
		return c.parseStringRaw()
	}
	start := c.pos
	for c.pos < len(c.s) {
		ch := c.s[c.pos]
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '_' {
			c.pos++
		} else {
			break
		}
	}
	if c.pos == start {
		return "", c.errf("expected hash key")
	}
	return c.s[start:c.pos], nil
}

// parseBless handles `bless( <inner>, '<Class>' )`. The inner is one of:
//   - the bool sentinel `do{\(my $o = 0|1)}` → dumpBool if Class ∈ bool family
//   - a `{ … }` hash ref → dumpClass
//   - a `[ … ]` array ref → error (not round-trippable; see below)
//   - a blessed scalar ref (`\$x`) that is not a recognized bool → error
func (c *perlCursor) parseBless() (*irNode, error) {
	if err := c.expect("bless"); err != nil {
		return nil, err
	}
	c.skipWS()
	if err := c.expect("("); err != nil {
		return nil, err
	}
	c.skipWS()
	if c.pos >= len(c.s) {
		return nil, c.errf("unterminated bless")
	}

	var inner *irNode
	boolVal := -1 // -1 = not a bool sentinel

	switch {
	case strings.HasPrefix(c.s[c.pos:], "do{"):
		v, err := c.parseBoolSentinel()
		if err != nil {
			return nil, err
		}
		boolVal = v
	case c.s[c.pos] == '\\':
		// A blessed scalar ref such as \$x that is not the bool sentinel form.
		return nil, c.errf("unsupported: blessed scalar ref")
	case c.s[c.pos] == '{':
		v, err := c.parseHash()
		if err != nil {
			return nil, err
		}
		inner = v
	case c.s[c.pos] == '[':
		v, err := c.parseArray()
		if err != nil {
			return nil, err
		}
		inner = v
	default:
		return nil, c.errf("unsupported bless inner")
	}

	c.skipWS()
	if err := c.expect(","); err != nil {
		return nil, err
	}
	c.skipWS()
	class, err := c.parseStringRaw()
	if err != nil {
		return nil, err
	}
	c.skipWS()
	if err := c.expect(")"); err != nil {
		return nil, err
	}

	if boolVal >= 0 {
		if !perlBoolClasses[class] {
			return nil, c.errf("unsupported: blessed scalar ref")
		}
		return nodeBool(boolVal == 1), nil
	}
	// inner is a hash (dumpMap) or array (dumpArray). Only the hash case is
	// round-trippable: a JS object can carry the __class sentinel. A JS array
	// cannot (jsToIR always treats arrays as dumpArray, dropping the class),
	// and irToJS / the encoder only read dumpClass pairs — so a blessed array
	// ref would silently lose its elements. Reject it, like blessed scalars.
	switch inner.kind {
	case dumpMap:
		return &irNode{kind: dumpClass, class: class, pairs: inner.pairs}, nil
	case dumpArray:
		return nil, c.errf("unsupported: blessed array ref")
	default:
		return nil, c.errf("unsupported bless inner kind")
	}
}

// parseBoolSentinel reads the `do{\(my $o = 0|1)}` form and returns 0 or 1.
func (c *perlCursor) parseBoolSentinel() (int, error) {
	if err := c.expect("do{"); err != nil {
		return -1, err
	}
	c.skipWS()
	if err := c.expect(`\(my`); err != nil {
		return -1, err
	}
	c.skipWS()
	if err := c.expect("$o"); err != nil {
		return -1, err
	}
	c.skipWS()
	if err := c.expect("="); err != nil {
		return -1, err
	}
	c.skipWS()
	if c.pos >= len(c.s) {
		return -1, c.errf("unterminated bool sentinel")
	}
	var v int
	switch c.s[c.pos] {
	case '0':
		v = 0
	case '1':
		v = 1
	default:
		return -1, c.errf("invalid bool sentinel value")
	}
	c.pos++
	c.skipWS()
	if err := c.expect(")"); err != nil {
		return -1, err
	}
	c.skipWS()
	if err := c.expect("}"); err != nil {
		return -1, err
	}
	return v, nil
}
