package main

import (
	"strings"
	"testing"
)

func TestCDPExec_FirefoxRejected(t *testing.T) {
	s := &wdSession{browser: "firefox"}
	_, err := s.cdpExec("Browser.getVersion", nil)
	if err == nil {
		t.Fatal("expected firefox to be rejected for CDP, got nil error")
	}
	if !strings.Contains(err.Error(), "Chrome-only") {
		t.Fatalf("error should mention Chrome-only, got: %v", err)
	}
}

func TestCDPQuery(t *testing.T) {
	cases := []struct {
		by, value   string
		wantQuery   string
		wantXPath   bool
		wantErr     bool
	}{
		{"css", "button.pay", "button.pay", false, false},
		{"id", "pay", `[id="pay"]`, false, false},
		{"name", "q", `[name="q"]`, false, false},
		{"tag", "button", "button", false, false},
		{"className", "pay", ".pay", false, false},
		{"xpath", `//button[.="Pay"]`, `//button[.="Pay"]`, true, false},
		{"linkText", "Next", `//a[normalize-space(.)='Next']`, true, false},
		{"partialLinkText", "Nex", `//a[contains(normalize-space(.), 'Nex')]`, true, false},
		// XPath 1.0 has no escape mechanism: a value with a double-quote uses
		// single quotes; a value with a single-quote uses double quotes.
		{"linkText", `Say "hi"`, `//a[normalize-space(.)='Say "hi"']`, true, false},
		{"linkText", "O'Brien", `//a[normalize-space(.)="O'Brien"]`, true, false},
		{"bogus", "x", "", false, true},
	}
	for _, c := range cases {
		q, xp, err := cdpQuery(c.by, c.value)
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected error", c.by)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: unexpected error %v", c.by, err)
		}
		if q != c.wantQuery || xp != c.wantXPath {
			t.Errorf("%s: got (%q,%v) want (%q,%v)", c.by, q, xp, c.wantQuery, c.wantXPath)
		}
	}
}

func TestXPathLiteral(t *testing.T) {
	cases := map[string]string{
		"plain":     "'plain'",
		`has "dq"`:  `'has "dq"'`,
		"has 'sq'":  `"has 'sq'"`,
		`both '" x`: `concat('both ', "'", '" x')`,
	}
	for in, want := range cases {
		if got := xpathLiteral(in); got != want {
			t.Errorf("xpathLiteral(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQuadCenter(t *testing.T) {
	// a 10x20 box at origin (0,0)-(10,20)
	quad := []float64{0, 0, 10, 0, 10, 20, 0, 20}
	x, y := quadCenter(quad, 0, 0)
	if x != 5 || y != 10 {
		t.Fatalf("center got (%v,%v) want (5,10)", x, y)
	}
	x, y = quadCenter(quad, 3, -4)
	if x != 8 || y != 6 {
		t.Fatalf("offset center got (%v,%v) want (8,6)", x, y)
	}
}

func TestMouseButtonsMask(t *testing.T) {
	for in, want := range map[string]int{"left": 1, "right": 2, "middle": 4, "": 1} {
		if got := mouseButtonsMask(in); got != want {
			t.Errorf("mask(%q)=%d want %d", in, got, want)
		}
	}
}

func TestCollectDocumentNodeIDs(t *testing.T) {
	// root #document(1) -> body -> iframe -> contentDocument #document(2)
	tree := map[string]any{
		"nodeName": "#document", "nodeId": float64(1),
		"children": []any{
			map[string]any{
				"nodeName": "BODY",
				"children": []any{
					map[string]any{
						"nodeName": "IFRAME",
						"contentDocument": map[string]any{
							"nodeName": "#document", "nodeId": float64(2),
						},
					},
				},
			},
		},
	}
	var ids []float64
	collectDocumentNodeIDs(tree, &ids)
	if len(ids) != 2 || ids[0] != 1 || ids[1] != 2 {
		t.Fatalf("got %v want [1 2]", ids)
	}
}
