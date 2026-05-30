package main

import (
	"strings"
	"testing"
)

func TestPHPVarDump_Golden(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	got, _ := phpVarDumpEncode(&irNode{kind: dumpArray, items: []*irNode{nodeInt(1), nodeInt(2)}}, opts)
	want := "array(2) {\n  [0]=>\n  int(1)\n  [1]=>\n  int(2)\n}"
	if got != want {
		t.Fatalf("var_dump:\n got: %q\nwant: %q", got, want)
	}
}

func TestPHPVarDump_ScalarGoldens(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	cases := []struct {
		node *irNode
		want string
	}{
		{nodeNull(), "NULL"},
		{nodeBool(true), "bool(true)"},
		{nodeInt(42), "int(42)"},
		{nodeString("café"), `string(5) "café"`},
	}
	for _, c := range cases {
		if got, _ := phpVarDumpEncode(c.node, opts); got != c.want {
			t.Errorf("encode:\n got: %s\nwant: %s", got, c.want)
		}
	}
}

func TestPHPVarDump_ObjectGolden(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	got, _ := phpVarDumpEncode(&irNode{kind: dumpClass, class: "Point", pairs: []irPair{{"x", nodeInt(1)}}}, opts)
	want := "object(Point)#1 (1) {\n  [\"x\"]=>\n  int(1)\n}"
	if got != want {
		t.Fatalf("object:\n got: %q\nwant: %q", got, want)
	}
}

func TestPHPVarDump_RoundTrip(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	cases := []*irNode{
		nodeNull(), nodeBool(true), nodeBool(false), nodeInt(-7), nodeString("Al"),
		{kind: dumpMap, pairs: []irPair{{"name", nodeString("Al")}, {"ok", nodeBool(true)}}},
		{kind: dumpArray, items: []*irNode{nodeInt(1), nodeInt(2)}},
		{kind: dumpClass, class: "Point", pairs: []irPair{{"x", nodeInt(1)}, {"y", nodeInt(2)}}},
		{kind: dumpMap, pairs: []irPair{{"nested", &irNode{kind: dumpArray, items: []*irNode{nodeString("a")}}}}},
	}
	for _, in := range cases {
		s, err := phpVarDumpEncode(in, opts)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		out, err := phpVarDumpDecode(s, opts)
		if err != nil {
			t.Fatalf("decode %q: %v", s, err)
		}
		re, err := phpVarDumpEncode(out, opts)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		if re != s {
			t.Fatalf("round-trip:\n in:  %q\n out: %q", s, re)
		}
	}
}

func TestPHPVarDump_LossyThrows(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	bad := []string{
		"array(1) {\n  [0]=>\n  *RECURSION*\n}",
		`string(99) "short"`,
		"object(Point)#1 (1) {\n  [\"x\":\"Point\":private]=>\n  int(1)\n}",
		"object(Point)#1 (1) {\n  [\"y\":protected]=>\n  int(1)\n}",
	}
	for _, in := range bad {
		_, err := phpVarDumpDecode(in, opts)
		if err == nil || !strings.Contains(err.Error(), "not losslessly parseable") {
			t.Errorf("expected lossy-parse error for %q, got %v", in, err)
		}
	}
}

func TestPHPVarDump_RejectsHugeCount(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	for _, in := range []string{"array(9999299999999) {", "object(X)#1 (9999299999999) {"} {
		if _, err := phpVarDumpDecode(in, opts); err == nil {
			t.Errorf("%q: expected error, not a crash/accept", in)
		}
	}
}

func TestPHPVarDump_StringEscapingRoundTrip(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	// Strings/keys with embedded quotes and backslashes must round-trip.
	cases := []*irNode{
		nodeString(`a\b`),
		nodeString(`he said "hi"`),
		nodeString(`mix "q" and \ slash`),
		{kind: dumpMap, pairs: []irPair{{`key"with`, nodeString(`v\al`)}}},
	}
	for _, in := range cases {
		s, err := phpVarDumpEncode(in, opts)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		out, err := phpVarDumpDecode(s, opts)
		if err != nil {
			t.Fatalf("decode %q: %v", s, err)
		}
		re, err := phpVarDumpEncode(out, opts)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		if re != s {
			t.Fatalf("round-trip:\n in:  %q\n out: %q", s, re)
		}
	}
}

func TestPHPVarDump_NewlineInStringSafe(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	s, err := phpVarDumpEncode(nodeString("a\nb"), opts)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	out, derr := phpVarDumpDecode(s, opts)
	if derr != nil {
		return // acceptable best-effort throw
	}
	if out.kind != dumpString || out.s != "a\nb" {
		t.Fatalf("newline string neither round-tripped nor errored cleanly: kind=%d s=%q", out.kind, out.s)
	}
}

func TestPHPVarDump_RejectsHugeStringLength(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	for _, in := range []string{
		`string(9000000000000000000) "x"`, // would panic on makeslice
		`string(2000000000) "x"`,           // would force a ~2GB alloc
	} {
		_, err := phpVarDumpDecode(in, opts)
		if err == nil {
			t.Errorf("%q: expected lossy/truncated error, not a panic/alloc/accept", in)
		}
	}
}
