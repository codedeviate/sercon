package main

import (
	"strings"
	"testing"
)

// normalizeXML strips inter-element whitespace so compact vs pretty output can
// be compared structurally in round-trip tests.
func normalizeXML(s string) string {
	s = strings.TrimSpace(s)
	for strings.Contains(s, ">\n") || strings.Contains(s, "> ") || strings.Contains(s, "\n<") {
		s = strings.ReplaceAll(s, ">\n", "><")
		s = strings.ReplaceAll(s, "> ", ">")
		s = strings.ReplaceAll(s, "\n<", "<")
		s = strings.ReplaceAll(s, "\t", "")
	}
	return s
}

func TestXML_DecodeEncodeRoundTrip(t *testing.T) {
	cases := []string{
		`<a>x</a>`,
		`<note id="5">hi</note>`,
		`<root><to>alice</to><from>bob</from></root>`,
		`<root><item>1</item><item>2</item></root>`,
		`<note id="5">hi<to>alice</to></note>`,
		`<empty/>`,
		`<ns:tag ns:id="1">x</ns:tag>`,
	}
	for _, in := range cases {
		n, err := xmlToIR(in)
		if err != nil {
			t.Errorf("xmlToIR(%q): %v", in, err)
			continue
		}
		out, err := irToXMLDoc(n, xmlOpts{})
		if err != nil {
			t.Errorf("irToXMLDoc for %q: %v", in, err)
			continue
		}
		if normalizeXML(out) != normalizeXML(in) {
			t.Errorf("round-trip:\n in:  %s\n out: %s", in, out)
		}
	}
}

func TestXML_ScalarToXMLString(t *testing.T) {
	chk := func(n *irNode, want string) {
		got, err := scalarToXMLString(n)
		if err != nil {
			t.Fatalf("scalarToXMLString: %v", err)
		}
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
	chk(&irNode{kind: dumpBool, b: true}, "true")
	chk(&irNode{kind: dumpBool, b: false}, "false")
	chk(&irNode{kind: dumpInt, i: 42}, "42")
	chk(&irNode{kind: dumpFloat, f: 1.5}, "1.5")
	chk(&irNode{kind: dumpNull}, "")
	chk(nodeString("hi"), "hi")
	if _, err := scalarToXMLString(&irNode{kind: dumpMap}); err == nil {
		t.Error("expected error for non-scalar")
	}
}

func TestXML_EncodeRootErrors(t *testing.T) {
	multi := &irNode{kind: dumpMap, pairs: []irPair{{key: "a", val: nodeString("1")}, {key: "b", val: nodeString("2")}}}
	if _, err := irToXMLDoc(multi, xmlOpts{}); err == nil {
		t.Error("expected error for multi-key root without rootName")
	}
	if _, err := irToXMLDoc(multi, xmlOpts{rootName: "doc"}); err != nil {
		t.Errorf("rootName wrap should succeed: %v", err)
	}
	arr := &irNode{kind: dumpArray, items: []*irNode{nodeString("1")}}
	if _, err := irToXMLDoc(arr, xmlOpts{rootName: "doc"}); err == nil {
		t.Error("expected error for array root content")
	}
}

func TestXML_Escaping(t *testing.T) {
	in := `<a x="&lt;&amp;&quot;">a &lt; b &amp; c</a>`
	n, err := xmlToIR(in)
	if err != nil {
		t.Fatal(err)
	}
	out, err := irToXMLDoc(n, xmlOpts{})
	if err != nil {
		t.Fatal(err)
	}
	n2, err := xmlToIR(out)
	if err != nil {
		t.Fatalf("re-decode: %v (out=%s)", err, out)
	}
	m := n2.pairs[0].val
	var gotAttr, gotText string
	for _, p := range m.pairs {
		if p.key == "@x" {
			gotAttr = p.val.s
		}
		if p.key == "#text" {
			gotText = p.val.s
		}
	}
	if gotAttr != `<&"` {
		t.Errorf("attr: got %q want %q", gotAttr, `<&"`)
	}
	if gotText != "a < b & c" {
		t.Errorf("text: got %q want %q", gotText, "a < b & c")
	}
}

func TestXML_ScriptRoundTrip(t *testing.T) {
	got := runSocketScript(t, `
		const x = { note: { "@id": "5", "#text": "hi", to: "alice" } };
		const s = codec.xml.encode(x);
		const back = codec.xml.decode(s);
		__capture(JSON.stringify(back) === JSON.stringify(x) ? "ok" : ("MISMATCH " + s + " -> " + JSON.stringify(back)));
	`)
	if got != "ok" {
		t.Errorf("round-trip: %v", got)
	}
}

func TestXML_ScriptArrays(t *testing.T) {
	got := runSocketScript(t, `
		const x = { root: { item: ["a", "b", "c"] } };
		const s = codec.xml.encode(x);
		const back = codec.xml.decode(s);
		__capture(JSON.stringify(back) === JSON.stringify(x) ? "ok" : (s + " -> " + JSON.stringify(back)));
	`)
	if got != "ok" {
		t.Errorf("arrays: %v", got)
	}
}

func TestXML_ScriptIndentDeclaration(t *testing.T) {
	got := runSocketScript(t, `
		const x = { root: { a: "1", b: { c: "2" } } };
		const s = codec.xml.encode(x, { indent: "  ", declaration: true });
		const back = codec.xml.decode(s);
		const ok = JSON.stringify(back) === JSON.stringify(x) && s.indexOf("<?xml") === 0 && s.indexOf("\n") > 0;
		__capture(ok ? "ok" : (s + " -> " + JSON.stringify(back)));
	`)
	if got != "ok" {
		t.Errorf("indent/declaration: %v", got)
	}
}

func TestXML_ScriptEncodeError(t *testing.T) {
	got := runSocketScript(t, `
		let outcome;
		try {
			codec.xml.encode({ a: 1, b: 2 });
			outcome = "no-throw";
		} catch (e) {
			outcome = "threw: " + (e && e.message ? e.message : String(e));
		}
		__capture(outcome);
	`)
	s, _ := got.(string)
	if !strings.Contains(s, "threw:") || !strings.Contains(s, "root") {
		t.Errorf("expected a root-related throw, got %q", s)
	}
}

// TestXML_MalformedRejected: RawToken doesn't enforce tag matching, so the
// decoder must reject mismatched/mis-nested tags and multiple roots itself.
func TestXML_MalformedRejected(t *testing.T) {
	bad := []string{
		`<a></b>`,          // mismatched
		`<a><b></a></b>`,   // mis-nested
		`<a/><b/>`,         // two roots
		`<a>1</a><b>2</b>`, // two roots with content
	}
	for _, in := range bad {
		if _, err := xmlToIR(in); err == nil {
			t.Errorf("expected error for %q, got nil", in)
		}
	}
}

// TestXML_AttrWhitespaceRoundTrip: tab/newline/CR in an attribute value must be
// escaped as numeric entities so they survive a round-trip (a raw newline in an
// attribute would be normalized to a space by a conformant parser).
func TestXML_AttrWhitespaceRoundTrip(t *testing.T) {
	in := &irNode{kind: dumpMap, pairs: []irPair{
		{key: "a", val: &irNode{kind: dumpMap, pairs: []irPair{
			{key: "@x", val: nodeString("a\nb\tc")},
		}}},
	}}
	out, err := irToXMLDoc(in, xmlOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "&#xA;") || !strings.Contains(out, "&#x9;") {
		t.Errorf("expected escaped whitespace entities in %q", out)
	}
	n2, err := xmlToIR(out)
	if err != nil {
		t.Fatal(err)
	}
	got := n2.pairs[0].val.pairs[0].val.s
	if got != "a\nb\tc" {
		t.Errorf("attr round-trip: got %q want %q", got, "a\nb\tc")
	}
}
