package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// TestEmitReference_CryptoFullyDocumented drives the real CLI surface
// (registerSurface) through Engine.WriteReference and asserts the crypto
// namespace renders a complete reference — real signatures, parameters,
// returns, and throws — proving the docs.go MemberDoc → reference pipeline
// end to end for the phase-1 proof namespace.
func TestEmitReference_CryptoFullyDocumented(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{DisableConsole: true})
	if err := registerSurface(eng); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := eng.WriteReference(&buf); err != nil {
		t.Fatal(err)
	}
	out := buf.String()

	for _, want := range []string{
		"#### crypto.hash.sha256",
		"sha256(input: string): string",
		"- `input` *(string)*",
		"**Returns:** string",
		"**Throws:**",
		"#### crypto.jwt.sign",
		"sign(claims: Record<string, unknown>, secret: string, opts?: { algorithm?: string }): string",
		"#### crypto.encrypt.keygen",
		"keygen(): { publicKey: string; privateKey: string }",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("crypto reference missing %q", want)
		}
	}
}
