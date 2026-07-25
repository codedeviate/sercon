// cmd/sercon/text_case.go
package main

import (
	"fmt"
	"strings"
	"unicode"
)

// caseSplit tokenizes s into lowercased words. Boundaries: separator runs
// (_ - . / and Unicode whitespace, discarded), lower/digit→upper transitions,
// and acronym→word boundaries (HTTPServer → [http, server]). Digits attach to
// their adjacent run (utf8, v2 stay whole).
func caseSplit(s string) []string {
	runes := []rune(s)
	var words []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			words = append(words, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	isSep := func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/' || unicode.IsSpace(r)
	}
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if isSep(r) {
			flush()
			continue
		}
		if len(cur) > 0 {
			prev := cur[len(cur)-1]
			switch {
			case unicode.IsUpper(r) && (unicode.IsLower(prev) || unicode.IsDigit(prev)):
				// camel boundary: a capital after lower/digit starts a new word.
				flush()
			case unicode.IsUpper(prev) && unicode.IsUpper(r) &&
				i+1 < len(runes) && unicode.IsLower(runes[i+1]):
				// acronym→word: ...HTTP|Server... — split before the last capital.
				flush()
			}
		}
		cur = append(cur, r)
	}
	flush()
	return words
}

// per-word casers. Words from caseSplit are already lowercase.
func caseWordLower(w string) string { return w }
func caseWordUpper(w string) string { return strings.ToUpper(w) }
func caseWordTitle(w string) string {
	r := []rune(w)
	if len(r) == 0 {
		return w
	}
	r[0] = unicode.ToUpper(r[0])
	return string(r) // remaining runes already lowercase
}

// caseEmit splits s, applies perWord(index, word), and joins with sep.
func caseEmit(s string, perWord func(i int, w string) string, sep string) string {
	words := caseSplit(s)
	for i, w := range words {
		words[i] = perWord(i, w)
	}
	return strings.Join(words, sep)
}

// caseNamesOrder is the canonical case list, in priority order (drives
// names(), convert() validation, and detect() precedence). Aliases excluded.
var caseNamesOrder = []string{
	"camel", "pascal", "snake", "screamingSnake", "ada", "camelSnake",
	"kebab", "train", "screamingKebab", "flat", "upperFlat", "dot", "path",
	"title", "sentence", "capital",
}

// caseConverters maps each canonical name to its converter.
var caseConverters = map[string]func(string) string{
	"camel": func(s string) string {
		return caseEmit(s, func(i int, w string) string {
			if i == 0 {
				return caseWordLower(w)
			}
			return caseWordTitle(w)
		}, "")
	},
	"pascal": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordTitle(w) }, "")
	},
	"snake": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordLower(w) }, "_")
	},
	"screamingSnake": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordUpper(w) }, "_")
	},
	"ada": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordTitle(w) }, "_")
	},
	"camelSnake": func(s string) string {
		return caseEmit(s, func(i int, w string) string {
			if i == 0 {
				return caseWordLower(w)
			}
			return caseWordTitle(w)
		}, "_")
	},
	"kebab": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordLower(w) }, "-")
	},
	"train": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordTitle(w) }, "-")
	},
	"screamingKebab": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordUpper(w) }, "-")
	},
	"flat": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordLower(w) }, "")
	},
	"upperFlat": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordUpper(w) }, "")
	},
	"dot": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordLower(w) }, ".")
	},
	"path": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordLower(w) }, "/")
	},
	"title": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordTitle(w) }, " ")
	},
	"sentence": func(s string) string {
		return caseEmit(s, func(i int, w string) string {
			if i == 0 {
				return caseWordTitle(w)
			}
			return caseWordLower(w)
		}, " ")
	},
	"capital": func(s string) string {
		return caseEmit(s, func(i int, w string) string { return caseWordTitle(w) }, " ")
	},
}

// caseAliases maps convenience names to a canonical converter name.
var caseAliases = map[string]string{
	"header": "train",          // Header-Case (e.g. Content-Type)
	"cobol":  "screamingKebab", // COBOL-CASE
	"slug":   "kebab",          // NOTE: kebab only — no transliteration/diacritic stripping
}

// caseConvert dispatches s to the named converter (canonical or alias).
func caseConvert(s, name string) (string, error) {
	if canon, ok := caseAliases[name]; ok {
		name = canon
	}
	fn, ok := caseConverters[name]
	if !ok {
		return "", fmt.Errorf("text.case.convert: unknown case %q (valid: %s)", name, strings.Join(caseNamesOrder, ", "))
	}
	return fn(s), nil
}

// caseDetect returns the first canonical name whose converter reproduces s
// exactly, or "unknown" (empty input is always "unknown").
func caseDetect(s string) string {
	if s == "" {
		return "unknown"
	}
	for _, name := range caseNamesOrder {
		if caseConverters[name](s) == s {
			return name
		}
	}
	return "unknown"
}
