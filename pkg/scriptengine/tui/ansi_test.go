package tui_test

import (
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine/tui"
)

func translate(s string) string {
	tr := tui.NewANSITranslator()
	return tr.Translate(s)
}

func TestANSI_PlainTextPassthrough(t *testing.T) {
	if got := translate("hello world"); got != "hello world" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_EscapeLiteralBracket(t *testing.T) {
	// tview parses [...] as a tag; literal "[" in script text must be
	// doubled so it renders as a single "[".
	if got := translate("foo [bar] baz"); got != "foo [[bar] baz" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_Reset(t *testing.T) {
	if got := translate("\x1b[0mtext"); got != "[-:-:-]text" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_BasicForeground(t *testing.T) {
	cases := map[int]string{
		30: "black", 31: "darkred", 32: "darkgreen", 33: "olive",
		34: "darkblue", 35: "darkmagenta", 36: "darkcyan", 37: "silver",
	}
	for code, name := range cases {
		in := "\x1b[" + itoa(code) + "mX"
		want := "[" + name + ":-:-]X"
		if got := translate(in); got != want {
			t.Errorf("SGR %d: got %q, want %q", code, got, want)
		}
	}
}

func TestANSI_BrightForeground(t *testing.T) {
	if got := translate("\x1b[91mERR"); got != "[red:-:-]ERR" {
		// Bright red maps to plain "red" (the default tview "red" is
		// the bright one); the dim red is "darkred".
		t.Errorf("got %q", got)
	}
}

func TestANSI_BasicBackground(t *testing.T) {
	if got := translate("\x1b[41mX"); got != "[-:darkred:-]X" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_BoldAndUnderline(t *testing.T) {
	if got := translate("\x1b[1;4mX"); got != "[-:-:bu]X" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_FgAndBoldCombined(t *testing.T) {
	if got := translate("\x1b[1;31mERR\x1b[0m done"); got != "[darkred:-:b]ERR[-:-:-] done" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_256ColorFg(t *testing.T) {
	// SGR 38;5;N — picks color from 256-color palette. We emit a
	// recognizable hex form so tview renders it.
	got := translate("\x1b[38;5;202mO")
	if !strings.HasPrefix(got, "[#") || !strings.Contains(got, ":-:-]O") {
		t.Errorf("expected truecolor fg tag, got %q", got)
	}
}

func TestANSI_TruecolorFg(t *testing.T) {
	// SGR 38;2;R;G;B — direct truecolor.
	if got := translate("\x1b[38;2;255;128;0mX"); got != "[#FF8000:-:-]X" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_StripCursorMotion(t *testing.T) {
	// CSI letters other than 'm' are not SGR; we strip them.
	if got := translate("\x1b[2Aabc"); got != "abc" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_StripOSC(t *testing.T) {
	// OSC: ESC ] ... BEL or ESC ] ... ESC \. Used by terminals for
	// window titles etc; meaningless in a scrollback pane.
	if got := translate("\x1b]0;title\x07text"); got != "text" {
		t.Errorf("got %q", got)
	}
}

func TestANSI_MalformedEscape(t *testing.T) {
	// Lone ESC at end of input: drop it (no buffered carry-over inside
	// a single Translate call — the caller already line-buffers).
	if got := translate("abc\x1b"); got != "abc" {
		t.Errorf("got %q", got)
	}
}

// itoa is a tiny helper to avoid pulling in strconv in test code; the
// values are all 2-3 digit ANSI codes.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [4]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
