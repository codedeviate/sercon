package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// checkdigitAlgos enumerates the algorithm names accepted across the
// namespace. `isbn13` is a documented alias for `ean13` (same check-digit
// math; the 978/979 prefix is the only thing that distinguishes them at
// the syntactic level).
var checkdigitAlgos = []string{"luhn", "isbn10", "isbn13", "ean13", "ean8", "upca"}

// mod10Config describes the per-algorithm weighted-digit setup used by the
// EAN/UPC family. weightCycle cycles from position 0; length is the full
// number length including the check digit.
type mod10Config struct {
	length      int
	weightCycle []int
}

var mod10Configs = map[string]mod10Config{
	"ean13": {length: 13, weightCycle: []int{1, 3}},
	"isbn13": {length: 13, weightCycle: []int{1, 3}},
	"ean8":  {length: 8, weightCycle: []int{3, 1}},
	"upca":  {length: 12, weightCycle: []int{3, 1}},
}

// checkdigitNamespace wires `api.checkdigit.*`. All members are synchronous —
// the algorithms are pure local math, no I/O — so we return plain Go types
// directly and let goja's reflection convert them. `compute` returns
// `(string, error)`; goja turns a non-nil error into a JS exception.
func checkdigitNamespace(_ *goja.Runtime) map[string]any {
	algos := make([]string, len(checkdigitAlgos))
	copy(algos, checkdigitAlgos)
	return map[string]any{
		"algos":    func() []string { return algos },
		"validate": checkdigitValidate,
		"compute":  checkdigitCompute,
		"inspect":  checkdigitInspect,
	}
}

// Each binding takes positional args instead of `goja.FunctionCall` because
// goja's host-callback special case only kicks in for funcs returning
// `goja.Value`. With per-positional args goja's reflection unmarshals each
// JS arg into the named Go param.

func checkdigitValidate(algo, input string) bool {
	ok, _ := validateCheck(strings.ToLower(strings.TrimSpace(algo)), strings.TrimSpace(input))
	return ok
}

func checkdigitCompute(algo, partial string) (string, error) {
	return computeCheck(strings.ToLower(strings.TrimSpace(algo)), strings.TrimSpace(partial))
}

func checkdigitInspect(algo, input string) map[string]any {
	algo = strings.ToLower(strings.TrimSpace(algo))
	input = strings.TrimSpace(input)

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
			out["valid"] = strings.EqualFold(given, computed)
		}
	}
	return out
}

// splitCheckDigit returns the input minus its last char, the last char, and
// whether the lengths are sensible for the algorithm. Returns false for
// inputs that are too short to possibly hold a check digit.
func splitCheckDigit(algo, input string) (partial, given string, ok bool) {
	if input == "" {
		return "", "", false
	}
	if algo == "luhn" && len(input) < 2 {
		return "", "", false
	}
	if cfg, hasCfg := mod10Configs[algo]; hasCfg && len(input) != cfg.length {
		return "", "", false
	}
	if algo == "isbn10" && len(input) != 10 {
		return "", "", false
	}
	return input[:len(input)-1], string(input[len(input)-1]), true
}

func validateCheck(algo, input string) (bool, error) {
	switch algo {
	case "luhn":
		return luhnValidate(input), nil
	case "isbn10":
		return isbn10Validate(input), nil
	case "isbn13", "ean13", "ean8", "upca":
		return mod10Validate(input, mod10Configs[algo]), nil
	default:
		return false, fmt.Errorf("unknown algorithm %q (supported: %s)",
			algo, strings.Join(checkdigitAlgos, ", "))
	}
}

func computeCheck(algo, partial string) (string, error) {
	switch algo {
	case "luhn":
		return luhnCompute(partial)
	case "isbn10":
		return isbn10Compute(partial)
	case "isbn13", "ean13", "ean8", "upca":
		return mod10Compute(partial, mod10Configs[algo])
	default:
		return "", fmt.Errorf("unknown algorithm %q (supported: %s)",
			algo, strings.Join(checkdigitAlgos, ", "))
	}
}

// --- Luhn -------------------------------------------------------------

// luhnValidate reads the digits right-to-left, doubling every second digit
// (positions 1, 3, 5, …; the check digit sits at position 0). Sum mod 10
// must be zero for the input to be valid.
func luhnValidate(input string) bool {
	if len(input) < 2 {
		return false
	}
	sum := 0
	for i := 0; i < len(input); i++ {
		c := input[len(input)-1-i]
		if c < '0' || c > '9' {
			return false
		}
		d := int(c - '0')
		if i%2 == 1 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return sum%10 == 0
}

// luhnCompute walks the partial number right-to-left applying the doubling
// rule, then picks the check digit that brings the running total to a
// multiple of 10. Doubling starts on the *rightmost* digit of `partial`
// because that digit sits at position 1 in the full number (position 0 is
// the check digit we're computing).
func luhnCompute(partial string) (string, error) {
	if partial == "" {
		return "", errors.New("luhn: empty input")
	}
	sum := 0
	for i := 0; i < len(partial); i++ {
		c := partial[len(partial)-1-i]
		if c < '0' || c > '9' {
			return "", errors.New("luhn: non-digit")
		}
		d := int(c - '0')
		if i%2 == 0 {
			d *= 2
			if d > 9 {
				d -= 9
			}
		}
		sum += d
	}
	return strconv.Itoa((10 - sum%10) % 10), nil
}

// --- ISBN-10 ---------------------------------------------------------

// isbn10Validate sums digit[i] * (10-i) for i in 0..9. The check digit
// (position 9) may be "X" representing 10. Valid if the sum is divisible
// by 11.
func isbn10Validate(input string) bool {
	if len(input) != 10 {
		return false
	}
	sum := 0
	for i := 0; i < 10; i++ {
		c := input[i]
		var d int
		switch {
		case i == 9 && (c == 'X' || c == 'x'):
			d = 10
		case c >= '0' && c <= '9':
			d = int(c - '0')
		default:
			return false
		}
		sum += d * (10 - i)
	}
	return sum%11 == 0
}

// isbn10Compute picks the digit at position 9 such that the weighted sum
// stays a multiple of 11. The check value 10 is encoded as "X" per the
// standard.
func isbn10Compute(partial string) (string, error) {
	if len(partial) != 9 {
		return "", errors.New("isbn10: expected 9 digits")
	}
	sum := 0
	for i := 0; i < 9; i++ {
		c := partial[i]
		if c < '0' || c > '9' {
			return "", errors.New("isbn10: non-digit")
		}
		sum += int(c-'0') * (10 - i)
	}
	cd := (11 - sum%11) % 11
	if cd == 10 {
		return "X", nil
	}
	return strconv.Itoa(cd), nil
}

// --- EAN / UPC family ------------------------------------------------

func mod10Validate(input string, cfg mod10Config) bool {
	if len(input) != cfg.length {
		return false
	}
	sum := 0
	for i := 0; i < cfg.length; i++ {
		c := input[i]
		if c < '0' || c > '9' {
			return false
		}
		sum += int(c-'0') * cfg.weightCycle[i%len(cfg.weightCycle)]
	}
	return sum%10 == 0
}

func mod10Compute(partial string, cfg mod10Config) (string, error) {
	if len(partial) != cfg.length-1 {
		return "", fmt.Errorf("expected %d digits", cfg.length-1)
	}
	sum := 0
	for i := 0; i < len(partial); i++ {
		c := partial[i]
		if c < '0' || c > '9' {
			return "", errors.New("non-digit input")
		}
		sum += int(c-'0') * cfg.weightCycle[i%len(cfg.weightCycle)]
	}
	w := cfg.weightCycle[(cfg.length-1)%len(cfg.weightCycle)]
	for cd := 0; cd < 10; cd++ {
		if (sum+cd*w)%10 == 0 {
			return strconv.Itoa(cd), nil
		}
	}
	return "", errors.New("no valid check digit (impossible)")
}
