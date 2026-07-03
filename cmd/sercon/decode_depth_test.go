package main

import (
	"strings"
	"testing"
)

// TestDecodeDepth_NoStackOverflow feeds each of the four dump-format decoders
// a pathologically deep (but otherwise well-formed) nested structure. Before
// the depth guard, this crashed the whole test binary with a Go runtime
// `fatal error: stack overflow` — a crash a caller's (value, error) API can
// never signal cleanly. After the guard, each decoder must return a normal
// Go error mentioning depth/nesting once MaxDecodeDepth is exceeded, and must
// not panic or crash.
func TestDecodeDepth_NoStackOverflow(t *testing.T) {
	const n = 200_000

	cases := []struct {
		name string
		fn   func() (*irNode, error)
	}{
		{
			name: "perl",
			fn: func() (*irNode, error) {
				deep := "$VAR1 = " + strings.Repeat("[", n) + strings.Repeat("]", n) + ";"
				return perlDumperDecode(deep, dumpOpts{})
			},
		},
		{
			name: "php_unserialize",
			fn: func() (*irNode, error) {
				deep := strings.Repeat("a:1:{i:0;", n) + "N;" + strings.Repeat("}", n)
				return phpSerializeDecode(deep, dumpOpts{})
			},
		},
		{
			name: "php_var_dump",
			fn: func() (*irNode, error) {
				lines := make([]string, 0, 2*n+1)
				for i := 0; i < n; i++ {
					lines = append(lines, "array(1) {", "[0]=>")
				}
				lines = append(lines, "NULL")
				for i := 0; i < n; i++ {
					lines = append(lines, "}")
				}
				return phpVarDumpDecode(strings.Join(lines, "\n"), dumpOpts{})
			},
		},
		{
			name: "php_var_export",
			fn: func() (*irNode, error) {
				deep := strings.Repeat("array(0=>", n) + "NULL" + strings.Repeat(",)", n)
				return phpVarExportDecode(deep, dumpOpts{})
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.fn()
			if err == nil {
				t.Fatalf("%s: expected a depth-limit error, got nil (deep nesting must not be accepted)", c.name)
			}
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "depth") && !strings.Contains(msg, "nest") {
				t.Fatalf("%s: expected a depth/nesting error, got %v", c.name, err)
			}
		})
	}
}
