package main

import (
	"reflect"
	"testing"
)

func TestCaseSplit(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"myVarName", []string{"my", "var", "name"}},
		{"my-var-name", []string{"my", "var", "name"}},
		{"My Var Name", []string{"my", "var", "name"}},
		{"my_var_name", []string{"my", "var", "name"}},
		{"userID", []string{"user", "id"}},
		{"HTTPServer", []string{"http", "server"}},
		{"getHTTPCode", []string{"get", "http", "code"}},
		{"parseHTTP2Response", []string{"parse", "http2", "response"}},
		{"utf8", []string{"utf8"}},
		{"v2", []string{"v2"}},
		{"__foo--bar//", []string{"foo", "bar"}},
		{"foo", []string{"foo"}},
		{"FOO", []string{"foo"}},
		{"", nil},
		{"___", nil},
		// Acronym/digit boundary edges (regression-lock verified behavior).
		{"serverHTTP", []string{"server", "http"}},   // trailing acronym run
		{"IOStream", []string{"io", "stream"}},       // leading acronym run
		{"ABc", []string{"a", "bc"}},                 // 2-cap run → split before last cap
		{"v2A", []string{"v2", "a"}},                 // digit run then a capital word
		{"HTML5Parser", []string{"html5", "parser"}}, // digit attaches to acronym
	}
	for _, c := range cases {
		if got := caseSplit(c.in); !reflect.DeepEqual(got, c.want) {
			t.Errorf("caseSplit(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestCaseConverters(t *testing.T) {
	// One representative input exercised through every converter.
	const in = "HTTPServer"
	want := map[string]string{
		"camel":          "httpServer",
		"pascal":         "HttpServer",
		"snake":          "http_server",
		"screamingSnake": "HTTP_SERVER",
		"ada":            "Http_Server",
		"camelSnake":     "http_Server",
		"kebab":          "http-server",
		"train":          "Http-Server",
		"screamingKebab": "HTTP-SERVER",
		"flat":           "httpserver",
		"upperFlat":      "HTTPSERVER",
		"dot":            "http.server",
		"path":           "http/server",
		"title":          "Http Server",
		"sentence":       "Http server",
		"capital":        "Http Server",
	}
	for name, w := range want {
		got, err := caseConvert(in, name)
		if err != nil {
			t.Fatalf("caseConvert(%q,%q) error: %v", in, name, err)
		}
		if got != w {
			t.Errorf("caseConvert(%q,%q) = %q, want %q", in, name, got, w)
		}
	}
	if len(caseNamesOrder) != len(want) {
		t.Errorf("caseNamesOrder has %d names, want %d", len(caseNamesOrder), len(want))
	}
}

func TestCaseIdempotence(t *testing.T) {
	for _, name := range caseNamesOrder {
		once, _ := caseConvert("myVarName", name)
		twice, _ := caseConvert(once, name)
		if once != twice {
			t.Errorf("%s not idempotent: once=%q twice=%q", name, once, twice)
		}
	}
}

func TestCaseDetect(t *testing.T) {
	cases := map[string]string{
		"my_var":      "snake",
		"MY_VAR":      "screamingSnake",
		"my-var":      "kebab",
		"myVar":       "camel",
		"MyVar":       "pascal",
		"My Var Name": "title", // title precedes capital in table order
		"my.var":      "dot",
		"my/var":      "path",
		"aB_cD-eF":    "unknown", // mixed → no converter reproduces it
		"":            "unknown", // empty → nothing to detect
	}
	for in, w := range cases {
		if got := caseDetect(in); got != w {
			t.Errorf("caseDetect(%q) = %q, want %q", in, got, w)
		}
	}
}

func TestCaseConvertUnknownAndAliases(t *testing.T) {
	if _, err := caseConvert("x", "nope"); err == nil {
		t.Error("caseConvert with unknown name should error")
	}
	// Aliases resolve to their canonical converter.
	for alias, canon := range caseAliases {
		a, err := caseConvert("myVarName", alias)
		if err != nil {
			t.Fatalf("alias %q errored: %v", alias, err)
		}
		c, _ := caseConvert("myVarName", canon)
		if a != c {
			t.Errorf("alias %q → %q, but canonical %q → %q", alias, a, canon, c)
		}
	}
	// Aliases are NOT in the canonical names list.
	for _, n := range caseNamesOrder {
		if _, isAlias := caseAliases[n]; isAlias {
			t.Errorf("alias %q leaked into caseNamesOrder", n)
		}
	}
}
