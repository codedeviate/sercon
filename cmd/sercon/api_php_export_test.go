package main

import "testing"

func TestPHPVarExport_Golden(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	got, _ := phpVarExportEncode(&irNode{kind: dumpMap, pairs: []irPair{
		{"name", nodeString("Al")}, {"age", nodeInt(30)},
	}}, opts)
	want := "array (\n  'name' => 'Al',\n  'age' => 30,\n)"
	if got != want {
		t.Fatalf("var_export:\n got: %q\nwant: %q", got, want)
	}
}

func TestPHPVarExport_StringEscaping(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	got, _ := phpVarExportEncode(nodeString(`a'b\c`), opts)
	if want := `'a\'b\\c'`; got != want {
		t.Fatalf("escape:\n got: %s\nwant: %s", got, want)
	}
}

func TestPHPVarExport_RoundTrip(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	cases := []*irNode{
		nodeNull(), nodeBool(true), nodeBool(false), nodeInt(-7), nodeFloat(3.14), nodeString("hi"),
		{kind: dumpArray, items: []*irNode{nodeInt(1), nodeString("x"), nodeBool(true), nodeNull()}},
		{kind: dumpMap, pairs: []irPair{{"name", nodeString("Al")}, {"age", nodeInt(30)}}},
		{kind: dumpClass, class: "Point", pairs: []irPair{{"x", nodeInt(1)}, {"y", nodeInt(2)}}},
		// nested
		{kind: dumpMap, pairs: []irPair{{"list", &irNode{kind: dumpArray, items: []*irNode{nodeInt(1), nodeInt(2)}}}}},
	}
	for _, in := range cases {
		s, err := phpVarExportEncode(in, opts)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		out, err := phpVarExportDecode(s, opts)
		if err != nil {
			t.Fatalf("decode %q: %v", s, err)
		}
		re, err := phpVarExportEncode(out, opts)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		if re != s {
			t.Fatalf("round-trip:\n in:  %s\n out: %s", s, re)
		}
	}
}

func TestPHPVarExport_DecodeHeuristic(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	n, err := phpVarExportDecode("array (\n  0 => 1,\n  1 => 2,\n)", opts)
	if err != nil || n.kind != dumpArray || len(n.items) != 2 {
		t.Fatalf("list → dumpArray; kind=%d err=%v", n.kind, err)
	}
	m, err := phpVarExportDecode("array (\n  'k' => 1,\n)", opts)
	if err != nil || m.kind != dumpMap || m.pairs[0].key != "k" {
		t.Fatalf("assoc → dumpMap; kind=%d err=%v", m.kind, err)
	}
	c, err := phpVarExportDecode("\\Point::__set_state(array(\n   'x' => 1,\n))", opts)
	if err != nil || c.kind != dumpClass || c.class != "Point" {
		t.Fatalf("__set_state → dumpClass; kind=%d class=%q err=%v", c.kind, c.class, err)
	}
}

func TestPHPVarExport_DecodeErrors(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	for _, bad := range []string{``, `'abc`, `array (`, `array`, `array (0 => )`, `nonsense`, `42 trailing`} {
		if _, err := phpVarExportDecode(bad, opts); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
