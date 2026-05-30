package main

import "testing"

func TestPHPSerialize_Golden(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	cases := []struct {
		node *irNode
		want string
	}{
		{nodeNull(), "N;"},
		{nodeBool(true), "b:1;"},
		{nodeBool(false), "b:0;"},
		{nodeInt(42), "i:42;"},
		{nodeString("café"), `s:5:"café";`}, // 'é' is 2 bytes → 5
		{&irNode{kind: dumpArray, items: []*irNode{nodeInt(1), nodeInt(2)}},
			`a:2:{i:0;i:1;i:1;i:2;}`},
		{&irNode{kind: dumpMap, pairs: []irPair{{"name", nodeString("Al")}, {"age", nodeInt(30)}}},
			`a:2:{s:4:"name";s:2:"Al";s:3:"age";i:30;}`},
		{&irNode{kind: dumpClass, class: "Point", pairs: []irPair{{"x", nodeInt(1)}, {"y", nodeInt(2)}}},
			`O:5:"Point":2:{s:1:"x";i:1;s:1:"y";i:2;}`},
	}
	for _, c := range cases {
		got, err := phpSerializeEncode(c.node, opts)
		if err != nil {
			t.Fatalf("encode %q: %v", c.want, err)
		}
		if got != c.want {
			t.Errorf("encode:\n got: %s\nwant: %s", got, c.want)
		}
	}
}

func TestPHPSerialize_DecodeArrayHeuristic(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	n, err := phpSerializeDecode(`a:2:{i:0;i:10;i:1;i:20;}`, opts)
	if err != nil || n.kind != dumpArray || len(n.items) != 2 {
		t.Fatalf("list array → dumpArray; got kind=%d err=%v", n.kind, err)
	}
	m, err := phpSerializeDecode(`a:1:{s:3:"key";i:7;}`, opts)
	if err != nil || m.kind != dumpMap || m.pairs[0].key != "key" {
		t.Fatalf("assoc array → dumpMap; got kind=%d err=%v", m.kind, err)
	}
	o, err := phpSerializeDecode(`O:5:"Point":1:{s:1:"x";i:1;}`, opts)
	if err != nil || o.kind != dumpClass || o.class != "Point" {
		t.Fatalf("object → dumpClass; got kind=%d class=%q err=%v", o.kind, o.class, err)
	}
}

func TestPHPSerialize_SharedRefAndCycle(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	// a:2:{i:0; <obj>; i:1; r:2;} — element 1 references value #2 (the inner array, which is value #2: value #1 is the outer array).
	shared := `a:2:{i:0;a:1:{s:1:"k";i:1;}i:1;r:2;}`
	n, err := phpSerializeDecode(shared, opts)
	if err != nil {
		t.Fatalf("shared ref decode: %v", err)
	}
	if n.items[0] != n.items[1] {
		t.Fatal("r: should resolve to the same *irNode")
	}
	// a cyclic reference (value points at an ancestor under construction) throws
	if _, err := phpSerializeDecode(`a:1:{i:0;r:1;}`, opts); err == nil {
		t.Fatal("expected circular reference error")
	}
}

func TestPHPSerialize_DecodeErrors(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	for _, bad := range []string{``, `x`, `i:abc;`, `s:5:"hi";`, `a:1:{i:0;}`} {
		if _, err := phpSerializeDecode(bad, opts); err == nil {
			t.Errorf("%q: expected decode error", bad)
		}
	}
}

func TestPHPSerialize_RoundTrip(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	for _, s := range []string{
		`N;`, `b:1;`, `i:-7;`, `s:3:"abc";`,
		`a:2:{i:0;i:1;i:1;i:2;}`,
		`a:2:{s:4:"name";s:2:"Al";s:3:"age";i:30;}`,
		`O:5:"Point":2:{s:1:"x";i:1;s:1:"y";i:2;}`,
	} {
		n, err := phpSerializeDecode(s, opts)
		if err != nil {
			t.Fatalf("decode %q: %v", s, err)
		}
		got, err := phpSerializeEncode(n, opts)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		if got != s {
			t.Errorf("round-trip:\n got: %s\nwant: %s", got, s)
		}
	}
}

func TestPHPSerialize_RejectsHugeCount(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	for _, bad := range []string{
		`a:9999299999999:{`,
		`O:1:"x":9999299999999:{`,
	} {
		if _, err := phpSerializeDecode(bad, opts); err == nil {
			t.Errorf("%q: expected error, not a crash/accept", bad)
		}
	}
}
