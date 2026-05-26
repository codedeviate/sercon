package main

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/dop251/goja"
)

// pregNamespace wires `api.preg.*`. The name is an homage to PHP's PCRE
// family (preg_match / preg_replace / preg_match_all). The pattern
// syntax follows PHP's `/regex/flags` delimited form — that's the
// "preg" part — but the matching engine is Go's stdlib `regexp` (RE2).
// RE2 doesn't support backreferences inside patterns, lookahead, or
// lookbehind; scripts that need those should reach for goja's native
// `RegExp` (which is ECMAScript-flavoured) or, eventually, a dedicated
// PCRE-compatible binding we may add later.
//
// Supported flags: `i` (case-insensitive), `m` (multiline — `^` / `$`
// match line boundaries), `s` (dotall — `.` matches newline). Other
// PHP flags (`x`, `u`, `U`, `e`) are not supported; the parser
// returns an error rather than silently dropping them so callers
// notice early.
//
// All three members are synchronous — regex work doesn't benefit from
// the event loop and the wrappers stay simple this way.
func pregNamespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value {
		panic(vm.NewGoError(err))
	}

	matchOne := func(pattern, subject string) goja.Value {
		re, err := compilePattern(pattern)
		if err != nil {
			return throw(err)
		}
		idxs := re.FindStringSubmatchIndex(subject)
		if idxs == nil {
			return goja.Null()
		}
		return vm.ToValue(buildMatchObject(subject, idxs))
	}

	matchAll := func(pattern, subject string) goja.Value {
		re, err := compilePattern(pattern)
		if err != nil {
			return throw(err)
		}
		all := re.FindAllStringSubmatchIndex(subject, -1)
		out := make([]map[string]any, 0, len(all))
		for _, idxs := range all {
			out = append(out, buildMatchObject(subject, idxs))
		}
		return vm.ToValue(out)
	}

	replace := func(pattern, replacement, subject string) goja.Value {
		re, err := compilePattern(pattern)
		if err != nil {
			return throw(err)
		}
		return vm.ToValue(re.ReplaceAllString(subject, replacement))
	}

	return map[string]any{
		"match":    matchOne,
		"matchAll": matchAll,
		"replace":  replace,
	}
}

// compilePattern parses a `/regex/flags` PHP-style delimited pattern
// into a Go *regexp.Regexp. The pattern body and flag set are merged
// into a single RE2-style inline flag prefix (`(?ims)`); empty flag
// strings skip the prefix entirely.
//
// Only the forward-slash delimiter is supported — adding `#`, `~` or
// brace pairs is on the backlog if real scripts demand them. The
// pattern must start AND end with `/`; a missing trailing delimiter
// surfaces as a clear error rather than the cryptic "regexp: parse"
// message Go would emit otherwise.
func compilePattern(pattern string) (*regexp.Regexp, error) {
	if pattern == "" {
		return nil, errors.New("preg: empty pattern")
	}
	if pattern[0] != '/' {
		return nil, fmt.Errorf("preg: pattern must start with `/`, got %q", string(pattern[0]))
	}
	end := strings.LastIndexByte(pattern, '/')
	if end == 0 {
		return nil, errors.New("preg: missing closing `/` delimiter")
	}
	body := pattern[1:end]
	flags := pattern[end+1:]

	prefix, err := translateFlags(flags)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(prefix + body)
	if err != nil {
		return nil, fmt.Errorf("preg: %w", err)
	}
	return re, nil
}

// translateFlags converts PHP-style flag letters into Go's inline
// `(?ims)` prefix. Unknown flags surface as errors with the offending
// character named — `u` (Unicode), `U` (ungreedy default), and `x`
// (extended whitespace-ignoring) are the most likely to trip people
// up, so we name them explicitly in the error rather than just saying
// "unsupported flag".
func translateFlags(flags string) (string, error) {
	if flags == "" {
		return "", nil
	}
	var goFlags strings.Builder
	seen := map[byte]bool{}
	for i := 0; i < len(flags); i++ {
		c := flags[i]
		if seen[c] {
			continue
		}
		seen[c] = true
		switch c {
		case 'i', 'm', 's':
			goFlags.WriteByte(c)
		case 'u':
			return "", errors.New("preg: flag `u` (Unicode) not supported; Go regexp is UTF-8 by default — drop the flag")
		case 'U':
			return "", errors.New("preg: flag `U` (ungreedy by default) not supported; use `?` per-quantifier instead")
		case 'x':
			return "", errors.New("preg: flag `x` (extended whitespace) not supported by RE2; strip the whitespace from the pattern")
		default:
			return "", fmt.Errorf("preg: unknown flag %q", string(c))
		}
	}
	if goFlags.Len() == 0 {
		return "", nil
	}
	return "(?" + goFlags.String() + ")", nil
}

// buildMatchObject converts a single subject + index-pair slice (the
// output of FindStringSubmatchIndex) into the JS-side match shape:
//
//	{ match: string, groups: string[], index: number }
//
// `match` is the full match; `groups` is every numbered submatch
// after group 0, in order. Unmatched optional groups (index pair of
// `(-1, -1)`) surface as empty strings — JS's RegExp returns
// `undefined` for those, but a consistent string type keeps the
// downstream shape easier to reason about.
func buildMatchObject(subject string, idxs []int) map[string]any {
	full := ""
	if idxs[0] >= 0 {
		full = subject[idxs[0]:idxs[1]]
	}
	groupCount := len(idxs)/2 - 1
	groups := make([]string, groupCount)
	for i := 0; i < groupCount; i++ {
		start := idxs[2+2*i]
		end := idxs[3+2*i]
		if start < 0 {
			groups[i] = ""
			continue
		}
		groups[i] = subject[start:end]
	}
	return map[string]any{
		"match":  full,
		"groups": groups,
		"index":  idxs[0],
	}
}
