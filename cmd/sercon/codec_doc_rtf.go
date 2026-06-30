// cmd/sercon/codec_doc_rtf.go
package main

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// cp1252High maps cp1252 bytes 0x80–0x9F to their Unicode runes; bytes
// 0xA0–0xFF equal their Latin-1 code points.
var cp1252High = map[byte]rune{
	0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡',
	0x88: 'ˆ', 0x89: '‰', 0x8A: 'Š', 0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž', 0x91: '‘',
	0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—', 0x98: '˜',
	0x99: '™', 0x9A: 'š', 0x9B: '›', 0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ',
}

func cp1252Rune(b byte) rune {
	if b < 0x80 {
		return rune(b)
	}
	if r, ok := cp1252High[b]; ok {
		return r
	}
	return rune(b)
}

// rtfSkipDest are control words whose group contents are not body text.
var rtfSkipDest = map[string]bool{
	"fonttbl": true, "colortbl": true, "stylesheet": true, "info": true,
	"pict": true, "header": true, "footer": true, "footnote": true,
	"annotation": true, "object": true, "themedata": true,
	"colorschememapping": true, "latentstyles": true, "datastore": true,
}

// readRTF extracts text from an RTF document, skipping non-output destination
// groups and decoding \'hh (cp1252) and \uN escapes.
func readRTF(data []byte) (docModel, error) {
	s := string(data)
	if !strings.HasPrefix(s, `{\rtf`) {
		return docModel{}, fmt.Errorf("codec.doc: not an RTF document")
	}
	var b strings.Builder
	skip := []bool{false} // group stack: is this group a skipped destination?
	top := func() bool { return skip[len(skip)-1] }
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '{':
			if len(skip) > 4096 {
				return docModel{}, fmt.Errorf("codec.doc: RTF nesting too deep")
			}
			skip = append(skip, top())
			i++
		case '}':
			if len(skip) > 1 {
				skip = skip[:len(skip)-1]
			}
			i++
		case '\\':
			if i+1 >= len(s) {
				i++
				continue
			}
			n := s[i+1]
			if n == '\\' || n == '{' || n == '}' {
				if !top() {
					b.WriteByte(n)
				}
				i += 2
				continue
			}
			if n == '\'' { // \'hh hex byte (cp1252)
				if i+3 < len(s) {
					if v, err := strconv.ParseInt(s[i+2:i+4], 16, 32); err == nil && !top() {
						b.WriteRune(cp1252Rune(byte(v)))
					}
					i += 4
					continue
				}
				i += 2
				continue
			}
			if n == '*' { // \* — current group is a skippable destination
				skip[len(skip)-1] = true
				i += 2
				continue
			}
			// control word
			j := i + 1
			for j < len(s) && ((s[j] >= 'a' && s[j] <= 'z') || (s[j] >= 'A' && s[j] <= 'Z')) {
				j++
			}
			word := s[i+1 : j]
			k := j
			neg := false
			if k < len(s) && s[k] == '-' {
				neg = true
				k++
			}
			ds := k
			for k < len(s) && s[k] >= '0' && s[k] <= '9' {
				k++
			}
			arg := s[ds:k]
			if k < len(s) && s[k] == ' ' { // control-word delimiter space
				k++
			}
			if rtfSkipDest[word] {
				skip[len(skip)-1] = true
			}
			switch word {
			case "par", "line", "sect":
				if !top() {
					b.WriteByte('\n')
				}
			case "tab":
				if !top() {
					b.WriteByte('\t')
				}
			case "u":
				if v, err := strconv.Atoi(arg); err == nil {
					if neg {
						v = -v
					}
					if v < 0 {
						v += 65536
					}
					if !top() {
						b.WriteRune(rune(v))
					}
				}
				// Skip the \ucN fallback char (default 1). Never consume an RTF
				// delimiter (\ { }) — only a literal fallback rune; advance a full
				// UTF-8 rune so multibyte fallbacks don't desync the parser.
				if k < len(s) && s[k] != '\\' && s[k] != '{' && s[k] != '}' {
					_, sz := utf8.DecodeRuneInString(s[k:])
					k += sz
				}
			}
			i = k
			continue
		case '\r', '\n':
			i++
		default:
			if !top() {
				b.WriteByte(c)
			}
			i++
		}
	}
	var paras []string
	for _, p := range strings.Split(b.String(), "\n") {
		if t := strings.TrimSpace(p); t != "" {
			paras = append(paras, t)
		}
	}
	return docModel{format: "rtf", text: joinParagraphs(paras), paragraphs: paras}, nil
}

// rtfEscape escapes a paragraph's text for RTF body content.
func rtfEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '{':
			b.WriteString(`\{`)
		case '}':
			b.WriteString(`\}`)
		case '\n':
			b.WriteString(`\par `)
		case '\t':
			b.WriteString(`\tab `)
		default:
			if r < 128 {
				b.WriteRune(r)
			} else {
				fmt.Fprintf(&b, `\u%d?`, int(r))
			}
		}
	}
	return b.String()
}

// writeRTF emits a minimal RTF document with one \par-separated paragraph per
// entry.
func writeRTF(paras []string) ([]byte, error) {
	var b strings.Builder
	b.WriteString(`{\rtf1\ansi\deff0{\fonttbl{\f0 Helvetica;}}\f0 `)
	for idx, p := range paras {
		if idx > 0 {
			b.WriteString(`\par `)
		}
		b.WriteString(rtfEscape(p))
	}
	b.WriteString("}")
	return []byte(b.String()), nil
}
