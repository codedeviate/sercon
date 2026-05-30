package main

import "testing"

func TestPerlDumper_BoolGolden(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	tr, _ := perlDumperEncode(nodeBool(true), opts)
	if want := `$VAR1 = bless( do{\(my $o = 1)}, 'JSON::XS::Boolean' );`; tr != want {
		t.Fatalf("true:\n got: %s\nwant: %s", tr, want)
	}
	fa, _ := perlDumperEncode(nodeBool(false), opts)
	if want := `$VAR1 = bless( do{\(my $o = 0)}, 'JSON::XS::Boolean' );`; fa != want {
		t.Fatalf("false:\n got: %s\nwant: %s", fa, want)
	}
}

func TestPerlDumper_BoolClassOverride(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{perlBoolClass: "JSON::PP::Boolean"})
	tr, _ := perlDumperEncode(nodeBool(true), opts)
	if want := `$VAR1 = bless( do{\(my $o = 1)}, 'JSON::PP::Boolean' );`; tr != want {
		t.Fatalf("override:\n got: %s\nwant: %s", tr, want)
	}
}

func TestPerlDumper_ParseBoolFamily(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	for _, cls := range []string{"JSON::XS::Boolean", "JSON::PP::Boolean", "Types::Serialiser::Boolean"} {
		src := `$VAR1 = bless( do{\(my $o = 1)}, '` + cls + `' );`
		n, err := perlDumperDecode(src, opts)
		if err != nil || n.kind != dumpBool || !n.b {
			t.Errorf("%s → bool true; got kind=%d b=%v err=%v", cls, n.kind, n.b, err)
		}
	}
	n, err := perlDumperDecode(`$VAR1 = 1;`, opts)
	if err != nil || n.kind != dumpInt {
		t.Errorf("bare 1 should stay int, got kind=%d err=%v", n.kind, err)
	}
}

func TestPerlDumper_BlessedScalarThrows(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	if _, err := perlDumperDecode(`$VAR1 = bless( \$x, 'My::Class' );`, opts); err == nil {
		t.Fatal("expected unsupported blessed scalar ref error")
	}
}

func TestPerlDumper_SelfRefCycleThrows(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	if _, err := perlDumperDecode("$VAR1 = {};\n$VAR1->{self} = $VAR1;", opts); err == nil {
		t.Fatal("expected circular reference error")
	}
}

func TestPerlDumper_RoundTrip(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	cases := []*irNode{
		nodeNull(), nodeBool(true), nodeBool(false), nodeInt(-7), nodeFloat(3.5), nodeString("hi"),
		nodeString(`a'b\c`),
		{kind: dumpArray, items: []*irNode{nodeInt(1), nodeString("x"), nodeNull()}},
		{kind: dumpMap, pairs: []irPair{{"name", nodeString("Al")}, {"age", nodeInt(30)}}},
		{kind: dumpClass, class: "Point", pairs: []irPair{{"x", nodeInt(1)}, {"y", nodeInt(2)}}},
		{kind: dumpMap, pairs: []irPair{{"flag", nodeBool(true)}, {"list", &irNode{kind: dumpArray, items: []*irNode{nodeInt(1)}}}}},
	}
	for _, in := range cases {
		s, err := perlDumperEncode(in, opts)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		out, err := perlDumperDecode(s, opts)
		if err != nil {
			t.Fatalf("decode %q: %v", s, err)
		}
		re, err := perlDumperEncode(out, opts)
		if err != nil {
			t.Fatalf("re-encode: %v", err)
		}
		if re != s {
			t.Fatalf("round-trip:\n in:  %q\n out: %q", s, re)
		}
	}
}

func TestPerlDumper_DecodeErrors(t *testing.T) {
	opts := withDumpDefaults(dumpOpts{})
	for _, bad := range []string{``, `'abc`, `$VAR1 = [`, `$VAR1 = {`, `$VAR1 = bless(`, `$VAR1 =`, `nonsense`, `$VAR1 = 1; $VAR2 = 2;`} {
		if _, err := perlDumperDecode(bad, opts); err == nil {
			t.Errorf("%q: expected error", bad)
		}
	}
}
