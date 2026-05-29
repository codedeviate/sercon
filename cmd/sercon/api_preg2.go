package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dlclark/regexp2"
	"github.com/dop251/goja"
)

// preg2Namespace wires `text.preg2.*` — the PCRE-flavoured sibling of
// `text.preg`. Where `text.preg` runs on Go's RE2 (linear-time, no
// lookaround or backreferences), `text.preg2` runs on
// [`github.com/dlclark/regexp2`](https://github.com/dlclark/regexp2),
// a port of .NET's regex engine. That buys lookahead, lookbehind,
// backreferences, and possessive quantifiers — at the cost of RE2's
// linear-time guarantee (a pathological pattern can backtrack
// exponentially, so don't run untrusted patterns without a timeout).
//
// The API mirrors `text.preg` exactly (match / matchAll / replace,
// `/pattern/flags` syntax, the same `{ match, groups, index }`
// result shape) so scripts can switch engines by changing the
// namespace name. The flag set adds `x` (ignore-pattern-whitespace),
// which RE2 couldn't support.
func preg2Namespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value { panic(vm.NewGoError(err)) }

	matchOne := func(pattern, subject string) goja.Value {
		re, err := compilePattern2(pattern)
		if err != nil {
			return throw(err)
		}
		m, err := re.FindStringMatch(subject)
		if err != nil {
			return throw(fmt.Errorf("preg2: match: %w", err))
		}
		if m == nil {
			return goja.Null()
		}
		return buildMatch2(vm, m)
	}

	matchAll := func(pattern, subject string) goja.Value {
		re, err := compilePattern2(pattern)
		if err != nil {
			return throw(err)
		}
		out := []any{}
		m, err := re.FindStringMatch(subject)
		if err != nil {
			return throw(fmt.Errorf("preg2: matchAll: %w", err))
		}
		for m != nil {
			out = append(out, buildMatch2(vm, m))
			m, err = re.FindNextMatch(m)
			if err != nil {
				return throw(fmt.Errorf("preg2: matchAll: %w", err))
			}
		}
		return vm.ToValue(out)
	}

	replace := func(pattern, replacement, subject string) goja.Value {
		re, err := compilePattern2(pattern)
		if err != nil {
			return throw(err)
		}
		// regexp2's Replace uses .NET substitution syntax: `$1` / `${1}`
		// for groups, `$$` for a literal dollar. startAt 0, count -1
		// (replace all).
		result, err := re.Replace(subject, replacement, 0, -1)
		if err != nil {
			return throw(fmt.Errorf("preg2: replace: %w", err))
		}
		return vm.ToValue(result)
	}

	return map[string]any{
		"match":    matchOne,
		"matchAll": matchAll,
		"replace":  replace,
	}
}

// compilePattern2 parses a `/regex/flags` delimited pattern into a
// regexp2.Regexp. Mirrors compilePattern (the RE2 one) but maps
// flags onto regexp2.RegexOptions and supports `x`. Only the
// forward-slash delimiter is recognised.
func compilePattern2(pattern string) (*regexp2.Regexp, error) {
	if pattern == "" {
		return nil, errors.New("preg2: empty pattern")
	}
	if pattern[0] != '/' {
		return nil, fmt.Errorf("preg2: pattern must start with `/`, got %q", string(pattern[0]))
	}
	end := strings.LastIndexByte(pattern, '/')
	if end == 0 {
		return nil, errors.New("preg2: missing closing `/` delimiter")
	}
	body := pattern[1:end]
	flags := pattern[end+1:]

	opts, err := translateFlags2(flags)
	if err != nil {
		return nil, err
	}
	re, err := regexp2.Compile(body, opts)
	if err != nil {
		return nil, fmt.Errorf("preg2: %w", err)
	}
	return re, nil
}

// translateFlags2 maps PCRE-style flag letters onto regexp2 options.
// Supports i / m / s / x. Unlike the RE2 binding, `x` is available
// here (regexp2 honours ignore-pattern-whitespace). `u` and `U`
// still error — regexp2 is Unicode-aware by default so `u` is
// redundant, and there's no global-ungreedy switch.
func translateFlags2(flags string) (regexp2.RegexOptions, error) {
	opts := regexp2.None
	seen := map[byte]bool{}
	for i := 0; i < len(flags); i++ {
		c := flags[i]
		if seen[c] {
			continue
		}
		seen[c] = true
		switch c {
		case 'i':
			opts |= regexp2.IgnoreCase
		case 'm':
			opts |= regexp2.Multiline
		case 's':
			opts |= regexp2.Singleline
		case 'x':
			opts |= regexp2.IgnorePatternWhitespace
		case 'u':
			return 0, errors.New("preg2: flag `u` (Unicode) not supported; regexp2 is Unicode-aware by default — drop the flag")
		case 'U':
			return 0, errors.New("preg2: flag `U` (ungreedy by default) not supported; use `?` per-quantifier instead")
		default:
			return 0, fmt.Errorf("preg2: unknown flag %q", string(c))
		}
	}
	return opts, nil
}

// buildMatch2 converts a regexp2 *Match into the JS-side shape used
// by both regex bindings: `{ match, groups, index }`. `groups` is
// every numbered submatch after group 0; an optional group that
// didn't participate surfaces as an empty string (regexp2's
// Group.String() already returns "" for those), matching the RE2
// binding's stable-shape policy.
func buildMatch2(vm *goja.Runtime, m *regexp2.Match) goja.Value {
	groups := m.Groups()
	out := make([]string, 0, len(groups)-1)
	for i := 1; i < len(groups); i++ {
		out = append(out, groups[i].String())
	}
	// Shared with text.preg via newMatchObject so the key order is stable
	// (see that helper for why a Go map would shuffle JSON.stringify keys).
	return newMatchObject(vm, m.String(), out, m.Index)
}
