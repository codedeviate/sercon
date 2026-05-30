package main

import (
	"fmt"
	"strconv"
	"strings"
)

// phpVarDumpEncode renders an IR node as PHP var_dump() output: the canonical
// debug layout with two-space indentation per nesting level. String lengths are
// byte counts, and the object id is always emitted as #1 (we do not track real
// PHP object ids).
func phpVarDumpEncode(n *irNode, opts dumpOpts) (string, error) {
	var b strings.Builder
	if err := phpDumpWrite(&b, n, 0); err != nil {
		return "", err
	}
	return b.String(), nil
}

// phpDumpWrite writes n with its opening token at the given depth. Composite
// members sit at depth+1; the closing brace returns to depth.
func phpDumpWrite(b *strings.Builder, n *irNode, depth int) error {
	switch n.kind {
	case dumpNull:
		b.WriteString("NULL")
	case dumpBool:
		if n.b {
			b.WriteString("bool(true)")
		} else {
			b.WriteString("bool(false)")
		}
	case dumpInt:
		fmt.Fprintf(b, "int(%d)", n.i)
	case dumpFloat:
		fmt.Fprintf(b, "float(%s)", strconv.FormatFloat(n.f, 'G', -1, 64))
	case dumpString:
		fmt.Fprintf(b, "string(%d) %q", len(n.s), n.s)
	case dumpArray:
		fmt.Fprintf(b, "array(%d) {\n", len(n.items))
		inner := strings.Repeat("  ", depth+1)
		for i, it := range n.items {
			fmt.Fprintf(b, "%s[%d]=>\n%s", inner, i, inner)
			if err := phpDumpWrite(b, it, depth+1); err != nil {
				return err
			}
			b.WriteByte('\n')
		}
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteByte('}')
	case dumpMap:
		fmt.Fprintf(b, "array(%d) {\n", len(n.pairs))
		if err := phpDumpPairs(b, n.pairs, depth); err != nil {
			return err
		}
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteByte('}')
	case dumpClass:
		fmt.Fprintf(b, "object(%s)#1 (%d) {\n", n.class, len(n.pairs))
		if err := phpDumpPairs(b, n.pairs, depth); err != nil {
			return err
		}
		b.WriteString(strings.Repeat("  ", depth))
		b.WriteByte('}')
	default:
		return fmt.Errorf("php.varDump: unsupported node kind %d", n.kind)
	}
	return nil
}

// phpDumpPairs writes a sequence of `["key"]=>` entries (map/object members) at
// depth+1.
func phpDumpPairs(b *strings.Builder, pairs []irPair, depth int) error {
	inner := strings.Repeat("  ", depth+1)
	for _, p := range pairs {
		fmt.Fprintf(b, "%s[%q]=>\n%s", inner, p.key, inner)
		if err := phpDumpWrite(b, p.val, depth+1); err != nil {
			return err
		}
		b.WriteByte('\n')
	}
	return nil
}

// phpVarDumpDecode parses canonical var_dump output back into an IR node on a
// best-effort basis. It throws "not losslessly parseable" on markers that
// cannot be faithfully reconstructed (*RECURSION*, *uninitialized*, truncated
// strings, visibility-annotated properties) rather than guessing. It is
// panic-free on malformed/truncated input and rejects trailing garbage.
func phpVarDumpDecode(text string, opts dumpOpts) (*irNode, error) {
	// Split into lines; a trailing newline produces an empty final element we
	// tolerate as long as nothing non-blank follows the top-level value.
	c := &phpDumpCursor{lines: strings.Split(text, "\n")}
	n, err := c.parseValue()
	if err != nil {
		return nil, err
	}
	// Reject any trailing non-blank content.
	for c.line < len(c.lines) {
		if strings.TrimSpace(c.lines[c.line]) != "" {
			return nil, c.errf("trailing data %q", c.lines[c.line])
		}
		c.line++
	}
	return n, nil
}

type phpDumpCursor struct {
	lines []string
	line  int
}

func (c *phpDumpCursor) errf(format string, args ...any) error {
	args = append(args, c.line+1)
	return fmt.Errorf("php.parseVarDump: "+format+" at line %d", args...)
}

func (c *phpDumpCursor) lossy(reason string) error {
	return fmt.Errorf("php.parseVarDump: var_dump output is not losslessly parseable: %s", reason)
}

// parseValue consumes the line(s) for one value starting at the cursor.
func (c *phpDumpCursor) parseValue() (*irNode, error) {
	if c.line >= len(c.lines) {
		return nil, c.errf("unexpected end of input")
	}
	raw := c.lines[c.line]
	body := strings.TrimSpace(raw)

	// Lossy markers: throw rather than guess.
	switch body {
	case "*RECURSION*":
		return nil, c.lossy("*RECURSION* marker")
	case "*uninitialized*":
		return nil, c.lossy("*uninitialized* marker")
	}

	switch {
	case body == "NULL":
		c.line++
		return nodeNull(), nil
	case body == "bool(true)":
		c.line++
		return nodeBool(true), nil
	case body == "bool(false)":
		c.line++
		return nodeBool(false), nil
	case strings.HasPrefix(body, "int("):
		return c.parseInt(body)
	case strings.HasPrefix(body, "float("):
		return c.parseFloat(body)
	case strings.HasPrefix(body, "string("):
		return c.parseString(body)
	case strings.HasPrefix(body, "array("):
		return c.parseArray(body)
	case strings.HasPrefix(body, "object("):
		return c.parseObject(body)
	default:
		return nil, c.errf("unexpected token %q", body)
	}
}

// scalarInner extracts the text between the first '(' and the matching trailing
// ')' for forms like int(42) / float(3.14). It requires the line to end with
// ')'.
func scalarInner(body, prefix string) (string, bool) {
	if !strings.HasPrefix(body, prefix) || !strings.HasSuffix(body, ")") {
		return "", false
	}
	return body[len(prefix) : len(body)-1], true
}

func (c *phpDumpCursor) parseInt(body string) (*irNode, error) {
	inner, ok := scalarInner(body, "int(")
	if !ok {
		return nil, c.errf("malformed int %q", body)
	}
	i, err := strconv.ParseInt(inner, 10, 64)
	if err != nil {
		return nil, c.errf("invalid int %q", inner)
	}
	c.line++
	return nodeInt(i), nil
}

func (c *phpDumpCursor) parseFloat(body string) (*irNode, error) {
	inner, ok := scalarInner(body, "float(")
	if !ok {
		return nil, c.errf("malformed float %q", body)
	}
	f, err := strconv.ParseFloat(inner, 64)
	if err != nil {
		return nil, c.errf("invalid float %q", inner)
	}
	c.line++
	return nodeFloat(f), nil
}

// parseString reads string(N) "..." and verifies the declared byte length N
// matches the bytes present before the closing quote. A mismatch (PHP truncates
// long strings in some contexts) is treated as a lossy parse and thrown.
func (c *phpDumpCursor) parseString(body string) (*irNode, error) {
	s, err := c.decodeStringLine(body)
	if err != nil {
		return nil, err
	}
	c.line++
	return nodeString(s), nil
}

// decodeStringLine parses a `string(N) "<bytes>"` line and returns the bytes,
// verifying the declared length. Shared by value and the visibility check is
// done by the caller (keys are not strings here).
func (c *phpDumpCursor) decodeStringLine(body string) (string, error) {
	rest := strings.TrimPrefix(body, "string(")
	idx := strings.IndexByte(rest, ')')
	if idx < 0 {
		return "", c.errf("malformed string header %q", body)
	}
	nTok := rest[:idx]
	declared, err := strconv.Atoi(nTok)
	if err != nil || declared < 0 {
		return "", c.errf("invalid string length %q", nTok)
	}
	after := rest[idx+1:]
	// Expect ` "<bytes>"`.
	if !strings.HasPrefix(after, " \"") || !strings.HasSuffix(after, "\"") || len(after) < 3 {
		return "", c.errf("malformed string body %q", body)
	}
	val := after[2 : len(after)-1]
	if len(val) != declared {
		return "", c.lossy(fmt.Sprintf("string(%d) declares %d bytes but %d present (truncated)", declared, declared, len(val)))
	}
	return val, nil
}

// parseCount extracts the count from a composite header line, bounding it
// against the remaining number of lines before any allocation. Each member
// occupies at least two lines (a key/index line and a value line), but we use
// the weaker >=1 line bound to stay safe and panic-free regardless of layout.
func (c *phpDumpCursor) parseCount(tok string) (int, error) {
	count, err := strconv.Atoi(tok)
	if err != nil || count < 0 {
		return 0, c.errf("invalid count %q", tok)
	}
	// A legitimate member needs at least one further line, so a count larger
	// than the remaining lines can never be satisfied — reject before any
	// allocation to avoid an OOM from an attacker-controlled count.
	if count > len(c.lines)-c.line {
		return 0, c.errf("count %d exceeds remaining input", count)
	}
	return count, nil
}

// parseArray reads `array(N) {` ... `}`. Indices [0],[1],... in order 0..N-1
// yield a dumpArray; any other keys yield a dumpMap.
func (c *phpDumpCursor) parseArray(body string) (*irNode, error) {
	inner, ok := scalarInner(strings.TrimSuffix(body, " {"), "array(")
	if !ok || !strings.HasSuffix(body, " {") {
		return nil, c.errf("malformed array header %q", body)
	}
	count, err := c.parseCount(inner)
	if err != nil {
		return nil, err
	}
	c.line++ // consume header

	var keys []string
	var vals []*irNode
	for i := 0; i < count; i++ {
		key, err := c.parseKeyLine()
		if err != nil {
			return nil, err
		}
		val, err := c.parseValue()
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
		vals = append(vals, val)
	}
	if err := c.expectClose(); err != nil {
		return nil, err
	}

	isList := true
	for i, k := range keys {
		if k != strconv.Itoa(i) {
			isList = false
			break
		}
	}
	if isList {
		return &irNode{kind: dumpArray, items: vals}, nil
	}
	n := &irNode{kind: dumpMap, pairs: make([]irPair, len(keys))}
	for i := range keys {
		n.pairs[i] = irPair{key: keys[i], val: vals[i]}
	}
	return n, nil
}

// parseObject reads `object(Cls)#id (N) {` ... `}` into a dumpClass node,
// ignoring the object id.
func (c *phpDumpCursor) parseObject(body string) (*irNode, error) {
	if !strings.HasSuffix(body, " {") {
		return nil, c.errf("malformed object header %q", body)
	}
	head := strings.TrimSuffix(body, " {")
	// head = object(Cls)#id (N)
	rest := strings.TrimPrefix(head, "object(")
	closeParen := strings.IndexByte(rest, ')')
	if closeParen < 0 {
		return nil, c.errf("malformed object header %q", body)
	}
	class := rest[:closeParen]
	after := rest[closeParen+1:] // "#id (N)"
	if !strings.HasPrefix(after, "#") {
		return nil, c.errf("malformed object id %q", body)
	}
	sp := strings.IndexByte(after, ' ')
	if sp < 0 {
		return nil, c.errf("malformed object header %q", body)
	}
	cntPart := after[sp+1:] // "(N)"
	inner, ok := scalarInner(cntPart, "(")
	if !ok {
		return nil, c.errf("malformed object count %q", body)
	}
	count, err := c.parseCount(inner)
	if err != nil {
		return nil, err
	}
	c.line++ // consume header

	n := &irNode{kind: dumpClass, class: class}
	for i := 0; i < count; i++ {
		key, err := c.parsePropKeyLine()
		if err != nil {
			return nil, err
		}
		val, err := c.parseValue()
		if err != nil {
			return nil, err
		}
		n.pairs = append(n.pairs, irPair{key: key, val: val})
	}
	if err := c.expectClose(); err != nil {
		return nil, err
	}
	return n, nil
}

// parseKeyLine reads an array key line: `[<idx>]=>` for list indices or
// `["<key>"]=>` for string keys. Returns the key text.
func (c *phpDumpCursor) parseKeyLine() (string, error) {
	if c.line >= len(c.lines) {
		return "", c.errf("unexpected end of input (expected key)")
	}
	body := strings.TrimSpace(c.lines[c.line])
	if !strings.HasSuffix(body, "]=>") {
		return "", c.errf("malformed key line %q", body)
	}
	inside := body[:len(body)-len("=>")] // "[...]"
	if !strings.HasPrefix(inside, "[") || !strings.HasSuffix(inside, "]") {
		return "", c.errf("malformed key line %q", body)
	}
	tok := inside[1 : len(inside)-1] // either <idx> or "<key>" (possibly :vis)
	c.line++
	if strings.HasPrefix(tok, "\"") {
		return c.decodeKeyString(tok, body)
	}
	// Integer index.
	if _, err := strconv.Atoi(tok); err != nil {
		return "", c.errf("invalid array index %q", tok)
	}
	return tok, nil
}

// parsePropKeyLine reads an object property key line, which is always quoted:
// `["<prop>"]=>`. Visibility-annotated forms (`["x":"Cls":private]` /
// `["y":protected]`) are lossy and thrown.
func (c *phpDumpCursor) parsePropKeyLine() (string, error) {
	if c.line >= len(c.lines) {
		return "", c.errf("unexpected end of input (expected property)")
	}
	body := strings.TrimSpace(c.lines[c.line])
	if !strings.HasSuffix(body, "]=>") {
		return "", c.errf("malformed property line %q", body)
	}
	inside := body[:len(body)-len("=>")]
	if !strings.HasPrefix(inside, "[") || !strings.HasSuffix(inside, "]") {
		return "", c.errf("malformed property line %q", body)
	}
	tok := inside[1 : len(inside)-1]
	c.line++
	if !strings.HasPrefix(tok, "\"") {
		return "", c.errf("malformed property key %q", body)
	}
	return c.decodeKeyString(tok, body)
}

// decodeKeyString parses a quoted key token `"<key>"`. A visibility annotation
// after the closing quote (`"x":"Cls":private` / `"y":protected`) makes the
// dump non-reconstructible, so it is reported as a lossy parse. tok is the text
// between the surrounding [ ].
func (c *phpDumpCursor) decodeKeyString(tok, body string) (string, error) {
	// Find the closing quote of the key. var_dump does not escape quotes inside
	// keys in its canonical form, so the key ends at the next '"'.
	endQuote := strings.IndexByte(tok[1:], '"')
	if endQuote < 0 {
		return "", c.errf("unterminated key string %q", body)
	}
	key := tok[1 : 1+endQuote]
	trailer := tok[1+endQuote+1:] // text after the closing quote
	if trailer != "" {
		// e.g. `:"Point":private` or `:protected` — visibility annotation.
		return "", c.lossy(fmt.Sprintf("visibility-annotated property %q", body))
	}
	return key, nil
}

// expectClose consumes a `}` line at the cursor.
func (c *phpDumpCursor) expectClose() error {
	if c.line >= len(c.lines) {
		return c.errf("unexpected end of input (expected '}')")
	}
	if strings.TrimSpace(c.lines[c.line]) != "}" {
		return c.errf("expected '}' but got %q", strings.TrimSpace(c.lines[c.line]))
	}
	c.line++
	return nil
}
