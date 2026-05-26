package main

import (
	"context"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// parseDMARCTags is the parser the binding leans on; testing it directly is
// the cheapest way to pin DMARC tag-format expectations without touching
// the network.
func TestParseDMARCTags(t *testing.T) {
	cases := []struct {
		name, input string
		want        map[string]string
	}{
		{
			name:  "basic policy + percent + rua",
			input: "v=DMARC1; p=reject; pct=100; rua=mailto:dmarc@example.com",
			want: map[string]string{
				"v": "DMARC1", "p": "reject", "pct": "100",
				"rua": "mailto:dmarc@example.com",
			},
		},
		{
			name:  "mixed case and odd whitespace",
			input: "v=DMARC1 ; P=quarantine ;SP=none;pct=50",
			want: map[string]string{
				"v": "DMARC1", "p": "quarantine", "sp": "none", "pct": "50",
			},
		},
		{
			name:  "rua list with comma — keep internal whitespace",
			input: "v=DMARC1; p=reject; rua=mailto:a@x, mailto:b@x",
			want: map[string]string{
				"v": "DMARC1", "p": "reject",
				"rua": "mailto:a@x, mailto:b@x",
			},
		},
		{
			name:  "empty parts and trailing semicolon are ignored",
			input: "v=DMARC1; ; p=none;",
			want:  map[string]string{"v": "DMARC1", "p": "none"},
		},
		{
			name:  "malformed bare token without = is dropped",
			input: "v=DMARC1; junk; p=none",
			want:  map[string]string{"v": "DMARC1", "p": "none"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseDMARCTags(c.input)
			if len(got) != len(c.want) {
				t.Errorf("size: got %d, want %d (%v vs %v)", len(got), len(c.want), got, c.want)
			}
			for k, v := range c.want {
				if got[k] != v {
					t.Errorf("%q: got %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// Smoke-test the namespace registration end-to-end. We hit a non-resolvable
// host so the lookup paths surface DNS NXDOMAIN without touching the network
// for long. Every individual probe + the aggregate `email.all` must reach
// the JS side and return either { present: false } or, for `all`, an object
// keyed by probe with the same.
func TestEmailNamespace_HandlesMissing(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterNamespaceFactory("email", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return emailNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	script := `
const target = "this-domain-most-certainly-does-not-exist.invalid";

async function checkAbsent(name, fn) {
  const r = await fn(target);
  if (typeof r !== "object") throw new Error(name + ": not object: " + JSON.stringify(r));
  if (r.present !== false) throw new Error(name + ": present should be false, got: " + JSON.stringify(r));
}
await checkAbsent("spf",    email.spf);
await checkAbsent("dmarc",  email.dmarc);
await checkAbsent("mtaSts", email.mtaSts);
await checkAbsent("tlsRpt", email.tlsRpt);
await checkAbsent("bimi",   email.bimi);

const all = await email.all(target);
if (all.domain !== target) throw new Error("all.domain: " + all.domain);
for (const k of ["spf", "dmarc", "mtaSts", "tlsRpt", "bimi"]) {
  const probe = all[k];
  if (typeof probe !== "object") throw new Error(k + ": missing from all: " + JSON.stringify(all));
  if (probe.present !== false) throw new Error(k + ": all.<probe>.present should be false: " + JSON.stringify(probe));
}
`
	if _, err := eng.Run(context.Background(), "email_smoke.ts", script); err != nil {
		t.Fatalf("email namespace smoke: %v", err)
	}
}

// parseMTASTSPolicy table-tests the RFC 8461 policy-file parser. Keys
// case-fold; mx: lines aggregate into a slice; max_age coerces to int
// when numeric; comments and blank lines are dropped.
func TestParseMTASTSPolicy(t *testing.T) {
	cases := []struct {
		name, input string
		check       func(t *testing.T, out map[string]any)
	}{
		{
			name: "canonical",
			input: `version: STSv1
mode: enforce
mx: mail.example.com
mx: *.mail.example.com
max_age: 604800`,
			check: func(t *testing.T, out map[string]any) {
				if out["version"] != "STSv1" {
					t.Errorf("version: %v", out["version"])
				}
				if out["mode"] != "enforce" {
					t.Errorf("mode: %v", out["mode"])
				}
				if out["maxAge"] != 604800 {
					t.Errorf("maxAge: %v (%T)", out["maxAge"], out["maxAge"])
				}
				mx, ok := out["mx"].([]string)
				if !ok || len(mx) != 2 ||
					mx[0] != "mail.example.com" || mx[1] != "*.mail.example.com" {
					t.Errorf("mx: %v", out["mx"])
				}
			},
		},
		{
			name: "crlf, comments, leading whitespace",
			input: "# comment line\r\n\r\n  version : STSv1 \r\nmode:testing\r\n   mx :  alt.example.com\r\n",
			check: func(t *testing.T, out map[string]any) {
				if out["version"] != "STSv1" {
					t.Errorf("version: %v", out["version"])
				}
				if out["mode"] != "testing" {
					t.Errorf("mode: %v", out["mode"])
				}
				mx, _ := out["mx"].([]string)
				if len(mx) != 1 || mx[0] != "alt.example.com" {
					t.Errorf("mx: %v", out["mx"])
				}
			},
		},
		{
			name:  "non-numeric max_age stays string",
			input: "version: STSv1\nmode: none\nmax_age: never",
			check: func(t *testing.T, out map[string]any) {
				if out["maxAge"] != "never" {
					t.Errorf("maxAge: %v", out["maxAge"])
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.check(t, parseMTASTSPolicy(c.input))
		})
	}
}
