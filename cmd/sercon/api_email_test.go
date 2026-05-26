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
// host so the lookup paths surface DNS errors / not-found without touching
// the network for a long time. Both bindings should reach the JS side and
// return something — the exact shape is "present" boolean / error.
func TestEmailNamespace_HandlesMissing(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), DisableConsole: true})
	if err := eng.RegisterNamespaceFactory("email", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return emailNamespace(vm, loop)
	}); err != nil {
		t.Fatal(err)
	}
	script := `
// A clearly non-resolvable domain: DNS should NXDOMAIN, and both bindings
// must turn that into present:false rather than throwing.
const target = "this-domain-most-certainly-does-not-exist.invalid";

const spf = await email.spf(target);
if (typeof spf !== "object" || typeof spf.present !== "boolean") {
  throw new Error("spf shape: " + JSON.stringify(spf));
}
if (spf.present !== false) throw new Error("spf.present: " + spf.present);

const dmarc = await email.dmarc(target);
if (typeof dmarc !== "object" || typeof dmarc.present !== "boolean") {
  throw new Error("dmarc shape: " + JSON.stringify(dmarc));
}
if (dmarc.present !== false) throw new Error("dmarc.present: " + dmarc.present);
`
	if _, err := eng.Run(context.Background(), "email_smoke.ts", script); err != nil {
		t.Fatalf("email namespace smoke: %v", err)
	}
}
