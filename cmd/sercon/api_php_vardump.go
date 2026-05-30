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
		// Emit the RAW bytes between the quotes (PHP var_dump does not escape
		// them) so the declared byte length matches the body. Using Go's %q here
		// would escape embedded " / \ and desync the length.
		fmt.Fprintf(b, "string(%d) \"%s\"", len(n.s), n.s)
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
		// Raw key bytes between the quotes (PHP var_dump does not escape keys).
		fmt.Fprintf(b, "%s[\"%s\"]=>\n%s", inner, p.key, inner)
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
	s, err := c.decodeStringValue(body)
	if err != nil {
		return nil, err
	}
	return nodeString(s), nil
}

// decodeStringValue parses a `string(N) "<bytes>"` value beginning at the
// cursor and returns the raw bytes, advancing the line cursor past every line
// it consumes. Decoding is count-based: it reads exactly the declared N bytes
// after the opening quote, joining subsequent lines with '\n' so that a string
// value containing embedded newlines round-trips. The byte immediately after
// those N bytes must be the closing quote (and nothing but blank trailer may
// follow on its line), otherwise the declared length disagrees with the body
// (PHP truncates long strings in some contexts) and the parse is lossy.
//
// Before growing any buffer, the declared length is bounded against the bytes
// that could possibly belong to this string (this line's tail plus each later
// line's bytes and the rejoined '\n'). An attacker-controlled N larger than
// that is rejected eagerly as truncated, so it can neither trigger a giant
// allocation nor a makeslice panic. The in-loop truncation check remains as a
// backstop.
func (c *phpDumpCursor) decodeStringValue(body string) (string, error) {
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
	// Expect ` "` opening the body.
	if !strings.HasPrefix(after, " \"") {
		return "", c.errf("malformed string body %q", body)
	}
	// firstLineTail is the raw text on this line after the opening quote.
	firstLineTail := after[2:]

	// Bound the declared length against the bytes actually available before
	// growing, so an attacker-controlled N can't trigger a giant allocation or
	// a makeslice panic. Each later line contributes its bytes plus the '\n'
	// that strings.Split removed.
	remaining := len(firstLineTail)
	for i := c.line + 1; i < len(c.lines); i++ {
		remaining += 1 + len(c.lines[i])
	}
	if declared > remaining {
		return "", c.lossy(fmt.Sprintf("string(%d) declares more bytes than present (truncated)", declared))
	}

	// Accumulate raw bytes until we have the declared count, pulling in further
	// lines (rejoined with '\n') when the body spans multiple lines.
	var sb strings.Builder
	sb.Grow(declared) // now bounded by actual input size
	chunk := firstLineTail
	consumedExtraLines := 0
	for {
		need := declared - sb.Len()
		// The current chunk must supply the closing quote at offset `need` once
		// enough bytes are available. Account for the quote when checking length.
		if len(chunk) >= need {
			// We have all remaining declared bytes (plus, ideally, the closing
			// quote) on this chunk.
			val := sb.String() + chunk[:need]
			closing := chunk[need:]
			if !strings.HasPrefix(closing, "\"") {
				return "", c.lossy(fmt.Sprintf("string(%d) declares %d bytes but body does not close after that count", declared, declared))
			}
			// Reject trailing non-blank content after the closing quote on the
			// same line (mirrors the encoder, which emits nothing after it).
			if strings.TrimSpace(closing[1:]) != "" {
				return "", c.errf("trailing data after string %q", strings.TrimSpace(closing[1:]))
			}
			c.line += 1 + consumedExtraLines
			return val, nil
		}
		// Consume the whole chunk and pull the next line, restoring the '\n'
		// that strings.Split removed.
		sb.WriteString(chunk)
		sb.WriteByte('\n')
		nextLine := c.line + 1 + consumedExtraLines
		if nextLine >= len(c.lines) {
			return "", c.lossy(fmt.Sprintf("string(%d) declares more bytes than present (truncated)", declared))
		}
		chunk = c.lines[nextLine]
		consumedExtraLines++
	}
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
	// var_dump emits raw key bytes between the quotes (no escaping), so a key
	// may itself contain '"'. The key therefore runs from the first quote to
	// the LAST quote on the token. A visibility annotation (`:"Cls":private` /
	// `:protected`) leaves trailing non-quote text after that last quote, so
	// such tokens do not end in '"' and are reported as a lossy parse.
	if len(tok) < 2 || !strings.HasSuffix(tok, "\"") {
		// No closing quote at the end means either an unterminated key or a
		// visibility-annotated property. Distinguish for a clearer message.
		if strings.IndexByte(tok[1:], '"') < 0 {
			return "", c.errf("unterminated key string %q", body)
		}
		return "", c.lossy(fmt.Sprintf("visibility-annotated property %q", body))
	}
	return tok[1 : len(tok)-1], nil
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
