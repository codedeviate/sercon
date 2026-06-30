// cmd/sercon/codec_dotenv_test.go
package main

import "testing"

func TestDotenvParse(t *testing.T) {
	m, err := dotenvParse("# comment\nexport A=1\nB=\"two words\"\nC='x'\n\nA=2\n")
	if err != nil {
		t.Fatal(err)
	}
	if m["A"] != "2" { // later duplicate wins
		t.Fatalf("A = %q, want 2", m["A"])
	}
	if m["B"] != "two words" || m["C"] != "x" {
		t.Fatalf("B=%q C=%q", m["B"], m["C"])
	}
}

func TestDotenvParse_Malformed(t *testing.T) {
	if _, err := dotenvParse("nokey\n"); err == nil {
		t.Fatal("expected error for a line without '='")
	}
}

func TestDotenvStringify_RoundTrip(t *testing.T) {
	cases := []map[string]any{
		{"A": "1"},
		{"PLAIN": "value", "SPACED": "two words", "HASH": "a#b", "EQ": "a=b"},
		{"EMPTY": "", "LEAD": " x", "TRAIL": "x ", "BOTH": " x "},
		{"QUOTED": `"already"`, "SQ": "'q'", "INNER": `a"b`},
		{"NUM": int64(42), "FLT": 3.5, "BOOL": true},
	}
	for _, in := range cases {
		text, err := dotenvStringify(in)
		if err != nil {
			t.Fatalf("stringify(%v): %v", in, err)
		}
		got, err := dotenvParse(text)
		if err != nil {
			t.Fatalf("parse(%q): %v", text, err)
		}
		// Expected: every value as its string form.
		for k, v := range in {
			want := ""
			switch t2 := v.(type) {
			case string:
				want = t2
			case bool:
				if t2 {
					want = "true"
				} else {
					want = "false"
				}
			case int64:
				want = "42"
			case float64:
				want = "3.5"
			}
			if got[k] != want {
				t.Fatalf("round-trip key %q: got %q want %q (text=%q)", k, got[k], want, text)
			}
		}
	}
}

func TestDotenvStringify_Errors(t *testing.T) {
	if _, err := dotenvStringify(map[string]any{"K": "a\nb"}); err == nil {
		t.Fatal("expected error for newline value")
	}
	if _, err := dotenvStringify(map[string]any{"bad key": "v"}); err == nil {
		t.Fatal("expected error for invalid key")
	}
	if _, err := dotenvStringify(map[string]any{"#A": "v"}); err == nil {
		t.Fatal("expected error for key starting with '#' (parseEnvFile drops comment lines)")
	}
	if _, err := dotenvStringify(map[string]any{"K": []any{1}}); err == nil {
		t.Fatal("expected error for non-coercible value")
	}
}
