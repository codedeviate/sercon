package scriptengine

import "testing"

// stripShebang must blank a leading "#!" line (preserving the newline so
// downstream error line numbers stay aligned with the source) and leave
// everything else untouched. A "#!" that isn't at the very start is not a
// shebang and must be preserved.
func TestStripShebang(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"entry blanks line, keeps newline", "#!/usr/bin/env sercon\ncode();\n", "\ncode();\n"},
		{"no shebang unchanged", "const x = 1;\n", "const x = 1;\n"},
		{"shebang-only no newline", "#!/usr/bin/env sercon", ""},
		{"hash-not-at-start preserved", "code();\n#!later\n", "code();\n#!later\n"},
		{"bare hash is not a shebang", "#notshebang\n", "#notshebang\n"},
		// The shebang line's CR is dropped with the rest of its text; keeping the
		// LF preserves the (now-empty) first line, so line numbers still align.
		{"crlf shebang", "#!/usr/bin/env sercon\r\ncode();\n", "\ncode();\n"},
	}
	for _, c := range cases {
		if got := stripShebang(c.in); got != c.want {
			t.Errorf("%s: stripShebang(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
