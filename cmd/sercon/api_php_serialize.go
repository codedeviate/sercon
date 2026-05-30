package main

import (
	"fmt"
	"strconv"
	"strings"
)

// phpSerializeEncode renders an IR node as a PHP serialize() string.
func phpSerializeEncode(n *irNode, opts dumpOpts) (string, error) {
	var b strings.Builder
	if err := phpSerWrite(&b, n); err != nil {
		return "", err
	}
	return b.String(), nil
}

func phpSerWrite(b *strings.Builder, n *irNode) error {
	switch n.kind {
	case dumpNull:
		b.WriteString("N;")
	case dumpBool:
		if n.b {
			b.WriteString("b:1;")
		} else {
			b.WriteString("b:0;")
		}
	case dumpInt:
		fmt.Fprintf(b, "i:%d;", n.i)
	case dumpFloat:
		fmt.Fprintf(b, "d:%s;", strconv.FormatFloat(n.f, 'G', -1, 64))
	case dumpString:
		fmt.Fprintf(b, "s:%d:\"%s\";", len(n.s), n.s) // len = bytes
	case dumpArray:
		fmt.Fprintf(b, "a:%d:{", len(n.items))
		for i, it := range n.items {
			fmt.Fprintf(b, "i:%d;", i)
			if err := phpSerWrite(b, it); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case dumpMap:
		fmt.Fprintf(b, "a:%d:{", len(n.pairs))
		for _, p := range n.pairs {
			phpSerWriteKey(b, p.key)
			if err := phpSerWrite(b, p.val); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	case dumpClass:
		fmt.Fprintf(b, "O:%d:\"%s\":%d:{", len(n.class), n.class, len(n.pairs))
		for _, p := range n.pairs {
			fmt.Fprintf(b, "s:%d:\"%s\";", len(p.key), p.key)
			if err := phpSerWrite(b, p.val); err != nil {
				return err
			}
		}
		b.WriteByte('}')
	}
	return nil
}

// phpSerWriteKey emits a canonical-int key as i:, everything else as s:.
func phpSerWriteKey(b *strings.Builder, k string) {
	if isCanonicalInt(k) {
		fmt.Fprintf(b, "i:%s;", k)
		return
	}
	fmt.Fprintf(b, "s:%d:\"%s\";", len(k), k)
}

// isCanonicalInt reports whether k is the canonical decimal form of an int
// (so it round-trips: "0","42","-7" yes; "01","+1","" no).
func isCanonicalInt(k string) bool {
	if k == "" {
		return false
	}
	i, err := strconv.ParseInt(k, 10, 64)
	return err == nil && strconv.FormatInt(i, 10) == k
}

// phpSerializeDecode parses a PHP serialize() string into an IR node.
func phpSerializeDecode(text string, opts dumpOpts) (*irNode, error) {
	c := &phpCursor{s: text, refs: nil, path: map[*irNode]bool{}}
	n, err := c.parseValue()
	if err != nil {
		return nil, err
	}
	if c.pos != len(c.s) {
		return nil, fmt.Errorf("php.unserialize: trailing data at offset %d", c.pos)
	}
	return n, nil
}

type phpCursor struct {
	s    string
	pos  int
	refs []*irNode
	path map[*irNode]bool
}

func (c *phpCursor) errf(format string, args ...any) error {
	args = append(args, c.pos)
	return fmt.Errorf("php.unserialize: "+format+" at offset %d", args...)
}

func (c *phpCursor) parseValue() (*irNode, error) {
	if c.pos >= len(c.s) {
		return nil, c.errf("unexpected end of input")
	}
	switch c.s[c.pos] {
	case 'N':
		return c.parseNull()
	case 'b':
		return c.parseBool()
	case 'i':
		return c.parseInt()
	case 'd':
		return c.parseFloat()
	case 's':
		return c.parseString()
	case 'a':
		return c.parseArray()
	case 'O':
		return c.parseObject()
	case 'r', 'R':
		return c.parseRef()
	default:
		return nil, c.errf("unexpected token %q", c.s[c.pos])
	}
}

// expect consumes the literal prefix at the cursor or errors.
func (c *phpCursor) expect(lit string) error {
	if !strings.HasPrefix(c.s[c.pos:], lit) {
		return c.errf("expected %q", lit)
	}
	c.pos += len(lit)
	return nil
}

// readUntil returns the substring from the cursor up to (not including) the
// delimiter byte, advancing the cursor past the delimiter.
func (c *phpCursor) readUntil(delim byte) (string, error) {
	idx := strings.IndexByte(c.s[c.pos:], delim)
	if idx < 0 {
		return "", c.errf("expected %q delimiter", delim)
	}
	out := c.s[c.pos : c.pos+idx]
	c.pos += idx + 1
	return out, nil
}

func (c *phpCursor) parseNull() (*irNode, error) {
	if err := c.expect("N;"); err != nil {
		return nil, err
	}
	n := nodeNull()
	c.refs = append(c.refs, n)
	return n, nil
}

func (c *phpCursor) parseBool() (*irNode, error) {
	if err := c.expect("b:"); err != nil {
		return nil, err
	}
	tok, err := c.readUntil(';')
	if err != nil {
		return nil, err
	}
	var n *irNode
	switch tok {
	case "0":
		n = nodeBool(false)
	case "1":
		n = nodeBool(true)
	default:
		return nil, c.errf("invalid bool %q", tok)
	}
	c.refs = append(c.refs, n)
	return n, nil
}

func (c *phpCursor) parseInt() (*irNode, error) {
	if err := c.expect("i:"); err != nil {
		return nil, err
	}
	tok, err := c.readUntil(';')
	if err != nil {
		return nil, err
	}
	i, perr := strconv.ParseInt(tok, 10, 64)
	if perr != nil {
		return nil, c.errf("invalid int %q", tok)
	}
	n := nodeInt(i)
	c.refs = append(c.refs, n)
	return n, nil
}

func (c *phpCursor) parseFloat() (*irNode, error) {
	if err := c.expect("d:"); err != nil {
		return nil, err
	}
	tok, err := c.readUntil(';')
	if err != nil {
		return nil, err
	}
	f, perr := strconv.ParseFloat(tok, 64)
	if perr != nil {
		return nil, c.errf("invalid float %q", tok)
	}
	n := nodeFloat(f)
	c.refs = append(c.refs, n)
	return n, nil
}

// parseStringBody reads s:<bytelen>:"<bytes>"; and returns the raw string.
// It does NOT touch refs — callers that produce a string value node append it.
func (c *phpCursor) parseStringBody() (string, error) {
	if err := c.expect("s:"); err != nil {
		return "", err
	}
	tok, err := c.readUntil(':')
	if err != nil {
		return "", err
	}
	n, perr := strconv.Atoi(tok)
	if perr != nil || n < 0 {
		return "", c.errf("invalid string length %q", tok)
	}
	if err := c.expect("\""); err != nil {
		return "", err
	}
	if c.pos+n > len(c.s) {
		return "", c.errf("string length %d exceeds input", n)
	}
	val := c.s[c.pos : c.pos+n]
	c.pos += n
	if err := c.expect("\";"); err != nil {
		return "", err
	}
	return val, nil
}

func (c *phpCursor) parseString() (*irNode, error) {
	val, err := c.parseStringBody()
	if err != nil {
		return nil, err
	}
	n := nodeString(val)
	c.refs = append(c.refs, n)
	return n, nil
}

// parseKey reads an array key, which is either i:<int>; or s:<n>:"...";.
// Keys are not standalone PHP values, so they are not appended to refs.
func (c *phpCursor) parseKey() (string, error) {
	if c.pos >= len(c.s) {
		return "", c.errf("unexpected end of input (expected key)")
	}
	switch c.s[c.pos] {
	case 'i':
		if err := c.expect("i:"); err != nil {
			return "", err
		}
		tok, err := c.readUntil(';')
		if err != nil {
			return "", err
		}
		if _, perr := strconv.ParseInt(tok, 10, 64); perr != nil {
			return "", c.errf("invalid int key %q", tok)
		}
		return tok, nil
	case 's':
		return c.parseStringBody()
	default:
		return "", c.errf("invalid key token %q", c.s[c.pos])
	}
}

func (c *phpCursor) parseArray() (*irNode, error) {
	if err := c.expect("a:"); err != nil {
		return nil, err
	}
	tok, err := c.readUntil(':')
	if err != nil {
		return nil, err
	}
	count, perr := strconv.Atoi(tok)
	if perr != nil || count < 0 {
		return nil, c.errf("invalid array count %q", tok)
	}
	// Each element (key+value) needs >=1 byte of input, so a legitimate count
	// can never exceed the bytes remaining. Guard before allocating to avoid a
	// fatal OOM from an attacker-controlled count.
	if count > len(c.s)-c.pos {
		return nil, c.errf("array count %d exceeds remaining input", count)
	}
	if err := c.expect("{"); err != nil {
		return nil, err
	}
	// Create and register the node before parsing children so refs and the
	// construction path see it (PHP counts the composite before its members).
	n := &irNode{kind: dumpMap}
	c.refs = append(c.refs, n)
	c.path[n] = true
	defer delete(c.path, n)

	keys := make([]string, 0, count)
	vals := make([]*irNode, 0, count)
	for i := 0; i < count; i++ {
		k, err := c.parseKey()
		if err != nil {
			return nil, err
		}
		v, err := c.parseValue()
		if err != nil {
			return nil, err
		}
		keys = append(keys, k)
		vals = append(vals, v)
	}
	if err := c.expect("}"); err != nil {
		return nil, err
	}

	// Heuristic: keys exactly i:0..count-1 in order → list (dumpArray).
	isList := true
	for i, k := range keys {
		if k != strconv.Itoa(i) {
			isList = false
			break
		}
	}
	if isList {
		n.kind = dumpArray
		n.items = vals
	} else {
		n.kind = dumpMap
		n.pairs = make([]irPair, count)
		for i := range keys {
			n.pairs[i] = irPair{key: keys[i], val: vals[i]}
		}
	}
	return n, nil
}

func (c *phpCursor) parseObject() (*irNode, error) {
	if err := c.expect("O:"); err != nil {
		return nil, err
	}
	clsLenTok, err := c.readUntil(':')
	if err != nil {
		return nil, err
	}
	clsLen, perr := strconv.Atoi(clsLenTok)
	if perr != nil || clsLen < 0 {
		return nil, c.errf("invalid class-name length %q", clsLenTok)
	}
	if err := c.expect("\""); err != nil {
		return nil, err
	}
	if c.pos+clsLen > len(c.s) {
		return nil, c.errf("class-name length %d exceeds input", clsLen)
	}
	class := c.s[c.pos : c.pos+clsLen]
	c.pos += clsLen
	if err := c.expect("\":"); err != nil {
		return nil, err
	}
	cntTok, err := c.readUntil(':')
	if err != nil {
		return nil, err
	}
	count, perr := strconv.Atoi(cntTok)
	if perr != nil || count < 0 {
		return nil, c.errf("invalid property count %q", cntTok)
	}
	// Each property (key+value) needs >=1 byte of input, so a legitimate count
	// can never exceed the bytes remaining. Guard before allocating to avoid a
	// fatal OOM from an attacker-controlled count.
	if count > len(c.s)-c.pos {
		return nil, c.errf("object count %d exceeds remaining input", count)
	}
	if err := c.expect("{"); err != nil {
		return nil, err
	}
	n := &irNode{kind: dumpClass, class: class}
	c.refs = append(c.refs, n)
	c.path[n] = true
	defer delete(c.path, n)

	n.pairs = make([]irPair, 0, count)
	for i := 0; i < count; i++ {
		key, err := c.parseStringBody()
		if err != nil {
			return nil, err
		}
		v, err := c.parseValue()
		if err != nil {
			return nil, err
		}
		n.pairs = append(n.pairs, irPair{key: key, val: v})
	}
	if err := c.expect("}"); err != nil {
		return nil, err
	}
	return n, nil
}

func (c *phpCursor) parseRef() (*irNode, error) {
	// Both r: (reference) and R: (object reference) resolve identically here.
	c.pos++ // consume 'r' or 'R'
	if err := c.expect(":"); err != nil {
		return nil, err
	}
	tok, err := c.readUntil(';')
	if err != nil {
		return nil, err
	}
	idx, perr := strconv.Atoi(tok)
	if perr != nil || idx < 1 || idx > len(c.refs) {
		return nil, c.errf("invalid reference index %q", tok)
	}
	target := c.refs[idx-1]
	if c.path[target] {
		return nil, errCircular("php.unserialize")
	}
	return target, nil
}
