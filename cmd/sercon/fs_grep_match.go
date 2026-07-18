package main

import (
	"bytes"
	"fmt"
	"regexp"
	"unicode/utf8"
)

type grepMatcher struct {
	re         *regexp.Regexp
	literal    []byte
	literalStr string
	ignoreCase bool
	word       bool
	invert     bool
	multiline  bool
}

type grepMatch struct {
	Path   string   `json:"path"`
	Line   int      `json:"line"`
	Column int      `json:"column"`
	Match  string   `json:"match"`
	Text   string   `json:"text"`
	Before []string `json:"before,omitempty"`
	After  []string `json:"after,omitempty"`
}

// compileGrepMatcher builds a matcher. fixed => literal fast-path (bytes.Index),
// unless ignoreCase/word force a regex. multiline is handled by the caller
// (whole-file vs per-line); here it only sets the (?s) flag so "." spans lines.
func compileGrepMatcher(pattern string, fixed, word, ignoreCase, multiline bool) (*grepMatcher, error) {
	m := &grepMatcher{ignoreCase: ignoreCase, word: word, multiline: multiline}
	// The literal fast-path is only safe when !ignoreCase: a case-insensitive
	// literal would need bytes.ToLower on each line, and ToLower can change the
	// byte length of the haystack for some runes (e.g. U+212A KELVIN SIGN,
	// 3 bytes, folds to "k", 1 byte), which desyncs the returned byte offset
	// from the original line and corrupts Match/column. IgnoreCase literals
	// fall through to the regex path (QuoteMeta + (?i)), where RE2 case-folds
	// with correct offsets.
	if fixed && !word && !multiline && !ignoreCase {
		m.literalStr = pattern
		m.literal = []byte(pattern)
		return m, nil
	}
	expr := pattern
	if fixed {
		expr = regexp.QuoteMeta(pattern)
	}
	if word {
		expr = `\b(?:` + expr + `)\b`
	}
	var flags string
	if ignoreCase {
		flags += "i"
	}
	if multiline {
		flags += "s"
	}
	if flags != "" {
		expr = "(?" + flags + ")" + expr
	}
	re, err := regexp.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid pattern: %w", err)
	}
	m.re = re
	return m, nil
}

// findFirst returns the byte offset+length of the first match on line, or (-1,0).
func (m *grepMatcher) findFirst(line []byte) (int, int) {
	if m.literal != nil {
		// Only reached when !ignoreCase (see compileGrepMatcher), so we can
		// index the original line directly with no lowercasing and thus no
		// per-line allocation.
		i := bytes.Index(line, m.literal)
		if i < 0 {
			return -1, 0
		}
		return i, len(m.literal)
	}
	loc := m.re.FindIndex(line)
	if loc == nil {
		return -1, 0
	}
	return loc[0], loc[1] - loc[0]
}

// grepFile scans data line by line, appending matches. before/after add context
// lines; maxMatches caps the returned slice per file (0 = unlimited). The
// returned count is the total hit count (for grepCount), which may exceed
// len(matches) when maxMatches caps output.
//
// When m.multiline is set, matching runs against the whole file at once (so a
// pattern with (?s) "." can span newlines) rather than line by line; in that
// mode Text is the matched span (which may contain newlines) and m.invert is
// not supported (invert is ignored when multiline).
func grepFile(path string, data []byte, m *grepMatcher, before, after, maxMatches int) ([]grepMatch, int) {
	if m.multiline {
		return grepFileMultiline(path, data, m, before, after, maxMatches)
	}
	lines := splitLines(data)
	var out []grepMatch
	count := 0
	for i, line := range lines {
		off, ln := m.findFirst(line)
		hit := off >= 0
		if m.invert {
			hit = !hit
		}
		if !hit {
			continue
		}
		count++
		if maxMatches > 0 && len(out) >= maxMatches {
			continue // keep counting for grepCount, stop appending
		}
		gm := grepMatch{
			Path:   path,
			Line:   i + 1,
			Column: 1,
			Text:   string(line),
		}
		if off >= 0 {
			gm.Column = utf8.RuneCount(line[:off]) + 1
			gm.Match = string(line[off : off+ln])
		}
		if before > 0 {
			gm.Before = contextLines(lines, i-before, i)
		}
		if after > 0 {
			gm.After = contextLines(lines, i+1, i+1+after)
		}
		out = append(out, gm)
	}
	return out, count
}

// grepFileMultiline matches m.re against the whole file so a match can span
// newlines. Line/Column are computed from byte offsets; context lines are taken
// from splitLines(data) around the match's start/end lines. invert is not
// supported here (see grepFile's doc comment).
func grepFileMultiline(path string, data []byte, m *grepMatcher, before, after, maxMatches int) ([]grepMatch, int) {
	locs := m.re.FindAllIndex(data, -1)
	if len(locs) == 0 {
		return nil, 0
	}
	lines := splitLines(data)
	var out []grepMatch
	count := 0
	for _, loc := range locs {
		count++
		if maxMatches > 0 && len(out) >= maxMatches {
			continue // keep counting for grepCount, stop appending
		}
		startLine := 1 + bytes.Count(data[:loc[0]], []byte{'\n'})
		endLine := 1 + bytes.Count(data[:loc[1]], []byte{'\n'})
		lineStart := lastNewlineBefore(data, loc[0])
		gm := grepMatch{
			Path:   path,
			Line:   startLine,
			Column: utf8.RuneCount(data[lineStart:loc[0]]) + 1,
			Match:  string(data[loc[0]:loc[1]]),
			Text:   string(data[loc[0]:loc[1]]),
		}
		if before > 0 {
			// startLine is 1-based; its index in lines is startLine-1.
			gm.Before = contextLines(lines, (startLine-1)-before, startLine-1)
		}
		if after > 0 {
			gm.After = contextLines(lines, endLine, endLine+after)
		}
		out = append(out, gm)
	}
	return out, count
}

// lastNewlineBefore returns the byte index just after the last '\n' before off
// (i.e. the start of off's line), or 0 if there is no preceding newline.
func lastNewlineBefore(data []byte, off int) int {
	i := bytes.LastIndexByte(data[:off], '\n')
	if i < 0 {
		return 0
	}
	return i + 1
}

// splitLines splits data on '\n', dropping a trailing '\r' and the empty final
// element after a trailing newline.
func splitLines(data []byte) [][]byte {
	if len(data) == 0 {
		return nil
	}
	raw := bytes.Split(data, []byte{'\n'})
	if len(raw) > 0 && len(raw[len(raw)-1]) == 0 {
		raw = raw[:len(raw)-1]
	}
	for i, l := range raw {
		raw[i] = bytes.TrimSuffix(l, []byte{'\r'})
	}
	return raw
}

func contextLines(lines [][]byte, from, to int) []string {
	if from < 0 {
		from = 0
	}
	if to > len(lines) {
		to = len(lines)
	}
	var out []string
	for i := from; i < to; i++ {
		out = append(out, string(lines[i]))
	}
	return out
}

func isBinary(data []byte) bool {
	n := len(data)
	if n > 8192 {
		n = 8192
	}
	return bytes.IndexByte(data[:n], 0) >= 0
}
