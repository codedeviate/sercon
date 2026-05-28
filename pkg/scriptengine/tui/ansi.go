package tui

import (
	"fmt"
	"strings"
)

// ANSITranslator turns ANSI SGR escape sequences (the color/style subset
// emitted by typical CLI tools) into tview color tags. It is **not** a
// general-purpose ANSI emulator: cursor motion, scroll regions, save /
// restore, and OSC sequences are silently dropped — they have no
// meaningful representation in a scrollback log pane.
//
// A Translator carries the current style across Translate calls so that
// a multi-chunk stream (the common case for piped subprocess output) is
// rendered consistently. Construct one Translator per pane.
type ANSITranslator struct {
	fg, bg                                string
	bold, dim, italic, underline, reverse bool
	lastEmittedTag                        string
}

// NewANSITranslator returns a fresh translator with default style.
func NewANSITranslator() *ANSITranslator { return &ANSITranslator{} }

// Translate runs s through the translator and returns the tview-ready
// text. Literal `[` in s is escaped to `[[` so tview does not interpret
// script text as a color tag.
func (t *ANSITranslator) Translate(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		if c != 0x1b {
			t.emitChar(&out, c)
			i++
			continue
		}
		// ESC <something>
		if i+1 >= len(s) {
			// Lone ESC at end — drop.
			return out.String()
		}
		next := s[i+1]
		switch next {
		case '[':
			// CSI sequence: ESC [ params... final
			end := indexCSIEnd(s, i+2)
			if end < 0 {
				// Unterminated — drop the rest.
				return out.String()
			}
			final := s[end]
			params := s[i+2 : end]
			if final == 'm' {
				t.applySGR(&out, params)
			}
			// Non-SGR CSI: silently dropped.
			i = end + 1
		case ']':
			// OSC: ESC ] ... BEL (0x07) or ESC ] ... ESC \
			end := indexOSCEnd(s, i+2)
			if end < 0 {
				return out.String()
			}
			i = end
		default:
			// Two-byte escape (ESC A, ESC 7, ESC 8, ...). Drop.
			i += 2
		}
	}
	return out.String()
}

// emitChar writes a single byte to out, escaping `[` to `[[`.
func (t *ANSITranslator) emitChar(out *strings.Builder, c byte) {
	if c == '[' {
		out.WriteString("[[")
		return
	}
	out.WriteByte(c)
}

// indexCSIEnd returns the index of the final byte of a CSI sequence
// starting at start (i.e. the first byte after `\e[`). CSI final bytes
// are in 0x40–0x7E. Returns -1 if none is found.
func indexCSIEnd(s string, start int) int {
	for i := start; i < len(s); i++ {
		c := s[i]
		if c >= 0x40 && c <= 0x7E {
			return i
		}
	}
	return -1
}

// indexOSCEnd returns the index AFTER the terminator of an OSC
// sequence (so the caller can set i to it). Terminator is BEL (0x07)
// or ST (ESC \). Returns -1 if unterminated.
func indexOSCEnd(s string, start int) int {
	for i := start; i < len(s); i++ {
		if s[i] == 0x07 {
			return i + 1
		}
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '\\' {
			return i + 2
		}
	}
	return -1
}

// applySGR parses an SGR parameter list ("1;31", "38;5;202", etc.),
// updates style state, and emits a tview tag if the style changed.
func (t *ANSITranslator) applySGR(out *strings.Builder, params string) {
	// Empty params = reset (CSI m == CSI 0 m).
	if params == "" {
		t.reset()
		t.emitTag(out)
		return
	}
	codes, ok := parseSGRCodes(params)
	if !ok {
		return
	}
	i := 0
	for i < len(codes) {
		c := codes[i]
		switch {
		case c == 0:
			t.reset()
		case c == 1:
			t.bold = true
		case c == 2:
			t.dim = true
		case c == 3:
			t.italic = true
		case c == 4:
			t.underline = true
		case c == 7:
			t.reverse = true
		case c == 22:
			t.bold, t.dim = false, false
		case c == 23:
			t.italic = false
		case c == 24:
			t.underline = false
		case c == 27:
			t.reverse = false
		case c == 39:
			t.fg = ""
		case c == 49:
			t.bg = ""
		case c >= 30 && c <= 37:
			t.fg = basicFg[c-30]
		case c >= 90 && c <= 97:
			t.fg = brightFg[c-90]
		case c >= 40 && c <= 47:
			t.bg = basicFg[c-40]
		case c >= 100 && c <= 107:
			t.bg = brightFg[c-100]
		case c == 38, c == 48:
			// Extended color: 38;5;N or 38;2;R;G;B (and likewise 48).
			if i+1 >= len(codes) {
				return
			}
			isFg := c == 38
			mode := codes[i+1]
			switch mode {
			case 5:
				if i+2 >= len(codes) {
					return
				}
				hex := palette256(codes[i+2])
				if isFg {
					t.fg = hex
				} else {
					t.bg = hex
				}
				i += 2
			case 2:
				if i+4 >= len(codes) {
					return
				}
				hex := fmt.Sprintf("#%02X%02X%02X", clamp255(codes[i+2]), clamp255(codes[i+3]), clamp255(codes[i+4]))
				if isFg {
					t.fg = hex
				} else {
					t.bg = hex
				}
				i += 4
			}
		}
		i++
	}
	t.emitTag(out)
}

func (t *ANSITranslator) reset() {
	t.fg, t.bg = "", ""
	t.bold, t.dim, t.italic, t.underline, t.reverse = false, false, false, false, false
}

// emitTag writes the current style as a tview tag if it differs from the
// last one emitted. Reduces redundant tags when consecutive SGR
// sequences leave style unchanged (rare in real input but cheap to dedupe).
func (t *ANSITranslator) emitTag(out *strings.Builder) {
	tag := t.tag()
	if tag == t.lastEmittedTag {
		return
	}
	out.WriteString(tag)
	t.lastEmittedTag = tag
}

func (t *ANSITranslator) tag() string {
	fg := t.fg
	if fg == "" {
		fg = "-"
	}
	bg := t.bg
	if bg == "" {
		bg = "-"
	}
	attrs := ""
	if t.bold {
		attrs += "b"
	}
	if t.dim {
		attrs += "d"
	}
	if t.italic {
		attrs += "i"
	}
	if t.underline {
		attrs += "u"
	}
	if t.reverse {
		attrs += "r"
	}
	if attrs == "" {
		attrs = "-"
	}
	return "[" + fg + ":" + bg + ":" + attrs + "]"
}

// basicFg / brightFg map ANSI 30-37 / 90-97 to tview color names. tview
// names "red", "green", etc. correspond to the *bright* terminal colors
// by tradition; we map the dim form to "dark<color>" where tview has it
// (red, green, yellow, blue, magenta, cyan) and pick reasonable fallbacks
// for black/white where there is no "dark" variant. Background uses the
// same names.
var basicFg = [8]string{
	"black", "darkred", "darkgreen", "olive",
	"darkblue", "darkmagenta", "darkcyan", "silver",
}
var brightFg = [8]string{
	"gray", "red", "green", "yellow",
	"blue", "fuchsia", "aqua", "white",
}

// parseSGRCodes splits "1;31" into []int{1,31}. Empty params between
// semicolons are treated as 0 per the ECMA-48 default. Returns false
// only if a param is non-numeric (malformed sequence; caller drops).
func parseSGRCodes(s string) ([]int, bool) {
	parts := strings.Split(s, ";")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			out = append(out, 0)
			continue
		}
		n, ok := atoi(p)
		if !ok {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}

// atoi parses a non-negative decimal integer. Lightweight; avoids the
// strconv error wrap on the hot path.
func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func clamp255(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// palette256 maps an xterm 256-color index to a hex string. The xterm
// palette is: 0–15 = the basic + bright 16 colors; 16–231 = 6×6×6 color
// cube; 232–255 = 24 grayscale shades.
func palette256(n int) string {
	switch {
	case n < 0 || n > 255:
		return "#FFFFFF"
	case n < 16:
		// Basic + bright 16: hardcoded standard xterm values.
		base := [16]string{
			"#000000", "#800000", "#008000", "#808000",
			"#000080", "#800080", "#008080", "#C0C0C0",
			"#808080", "#FF0000", "#00FF00", "#FFFF00",
			"#0000FF", "#FF00FF", "#00FFFF", "#FFFFFF",
		}
		return base[n]
	case n < 232:
		// 6x6x6 cube; each axis steps 0,95,135,175,215,255.
		idx := n - 16
		r := idx / 36
		g := (idx / 6) % 6
		b := idx % 6
		levels := [6]int{0, 95, 135, 175, 215, 255}
		return fmt.Sprintf("#%02X%02X%02X", levels[r], levels[g], levels[b])
	default:
		// 24-shade grayscale.
		v := 8 + (n-232)*10
		return fmt.Sprintf("#%02X%02X%02X", v, v, v)
	}
}
