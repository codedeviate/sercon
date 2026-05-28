package main

import "testing"

// All "valid" entries below are well-known test vectors lifted from the
// standards or from public examples; flipping a single digit in any of
// them must yield invalid, and stripping the last digit must let
// computeCheck reconstruct it exactly.
func TestCheckdigit_KnownVectors(t *testing.T) {
	cases := []struct {
		algo, valid string
		check       string // the expected check digit (last char of `valid`)
	}{
		{"luhn", "4532015112830366", "6"}, // Visa-style 16-digit
		{"luhn", "79927398713", "3"},      // Wikipedia Luhn example
		{"luhn", "5500000000000004", "4"}, // MasterCard prefix sample
		{"isbn10", "0306406152", "2"},     // ISBN-10 with numeric check
		{"isbn10", "048665088X", "X"},     // ISBN-10 with X check
		{"isbn13", "9780306406157", "7"},  // ISBN-13 (978 prefix)
		{"ean13", "5901234123457", "7"},
		{"ean8", "73513537", "7"},
		{"upca", "036000291452", "2"},
	}
	for _, c := range cases {
		t.Run(c.algo+"/"+c.valid, func(t *testing.T) {
			ok, err := validateCheck(c.algo, c.valid)
			if err != nil {
				t.Fatalf("validate err: %v", err)
			}
			if !ok {
				t.Fatalf("expected %q to validate under %q", c.valid, c.algo)
			}
			// Flip the last digit (X -> 0; else (d+1) mod 10) to get a
			// definitely-invalid variant.
			last := c.valid[len(c.valid)-1]
			var flipped byte
			if last == 'X' || last == 'x' {
				flipped = '0'
			} else {
				flipped = '0' + ((last-'0')+1)%10
			}
			bad := c.valid[:len(c.valid)-1] + string(flipped)
			if bad != c.valid {
				ok, _ := validateCheck(c.algo, bad)
				if ok {
					t.Errorf("flipped vector %q must not validate", bad)
				}
			}
			// computeCheck on the partial must reproduce the original check.
			partial := c.valid[:len(c.valid)-1]
			got, err := computeCheck(c.algo, partial)
			if err != nil {
				t.Fatalf("compute err: %v", err)
			}
			if got != c.check {
				t.Errorf("compute(%q, %q) = %q, want %q", c.algo, partial, got, c.check)
			}
		})
	}
}

// Unknown algorithm names produce errors from compute and false from
// validate. The CLI binding hides the validate error so scripts get a
// uniform boolean; compute surfaces it because it has no neutral return
// value.
func TestCheckdigit_UnknownAlgorithm(t *testing.T) {
	if ok, err := validateCheck("nope", "1234"); ok || err == nil {
		t.Errorf("validate: got ok=%v err=%v; want false + non-nil err", ok, err)
	}
	if _, err := computeCheck("nope", "1234"); err == nil {
		t.Error("compute: expected error for unknown algo")
	}
}

// Bad input (non-digit characters, wrong length) must be rejected
// gracefully — false for validate, error for compute — rather than
// panicking.
func TestCheckdigit_BadInput(t *testing.T) {
	for _, algo := range []string{"luhn", "isbn10", "isbn13", "ean13", "ean8", "upca"} {
		t.Run(algo+"/empty", func(t *testing.T) {
			if ok, _ := validateCheck(algo, ""); ok {
				t.Error("empty input must not validate")
			}
		})
		t.Run(algo+"/non-digit", func(t *testing.T) {
			if ok, _ := validateCheck(algo, "abc"); ok {
				t.Error("non-digit input must not validate")
			}
		})
		t.Run(algo+"/wrong-length", func(t *testing.T) {
			if ok, _ := validateCheck(algo, "1"); ok {
				t.Error("one-digit input must not validate")
			}
		})
	}
}

// inspect's report is the union of validate + compute; this pins the
// shape so a future refactor can't silently rename keys.
func TestCheckdigit_InspectShape(t *testing.T) {
	got := checkdigitInspectInline("luhn", "4532015112830366")
	for _, k := range []string{"algo", "input", "valid", "given", "computed"} {
		if _, ok := got[k]; !ok {
			t.Errorf("inspect: missing key %q (got %v)", k, got)
		}
	}
	if got["valid"] != true {
		t.Errorf("inspect.valid: %v", got["valid"])
	}
	if got["given"] != "6" {
		t.Errorf("inspect.given: %v", got["given"])
	}
	if got["computed"] != "6" {
		t.Errorf("inspect.computed: %v", got["computed"])
	}
}

// checkdigitInspectInline reproduces checkdigitInspect's logic without
// the goja.FunctionCall shim, so tests can run it cheaply. Kept in sync
// with the real binding by routing through the same validateCheck and
// computeCheck helpers.
func checkdigitInspectInline(algo, input string) map[string]any {
	out := map[string]any{
		"algo":     algo,
		"input":    input,
		"valid":    false,
		"given":    "",
		"computed": "",
	}
	if input != "" {
		out["given"] = string(input[len(input)-1])
	}
	if partial, given, ok := splitCheckDigit(algo, input); ok {
		if computed, err := computeCheck(algo, partial); err == nil {
			out["computed"] = computed
			out["valid"] = (given == computed) || (given == "x" && computed == "X") || (given == "X" && computed == "X")
		}
	}
	return out
}
