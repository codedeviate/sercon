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
	m := &grepMatcher{ignoreCase: ignoreCase, word: word}
	if fixed && !word && !multiline {
		m.literalStr = pattern
		if ignoreCase {
			m.literal = bytes.ToLower([]byte(pattern))
		} else {
			m.literal = []byte(pattern)
		}
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
		hay := line
		if m.ignoreCase {
			hay = bytes.ToLower(line)
		}
		i := bytes.Index(hay, m.literal)
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
func grepFile(path string, data []byte, m *grepMatcher, before, after, maxMatches int) ([]grepMatch, int) {
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
