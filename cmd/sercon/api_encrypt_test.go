package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runEncryptScript wires the encrypt namespace + a __capture side
// channel and returns whatever the script writes via __capture.
// Mirrors the harness from api_preg_test.go.
func runEncryptScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot: t.TempDir(),
		Timeout:    5 * time.Second,
	})
	if err := eng.RegisterNamespaceFactory("encrypt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return encryptNamespace(vm)
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatalf("register __capture: %v", err)
	}
	if _, err := eng.Run(context.Background(), "enc.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// Keygen output: publicKey starts with "age1", privateKey starts
// with "AGE-SECRET-KEY-". Both are non-trivial length.
func TestEncryptKeygen_ShapeSane(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygen();
		const __result = [
			k.publicKey.startsWith("age1"),
			k.privateKey.startsWith("AGE-SECRET-KEY-"),
			k.publicKey.length >= 30,
			k.privateKey.length >= 30,
		].join(",");
	`)
	if got != "true,true,true,true" {
		t.Errorf("keygen shape: %v", got)
	}
}

// Two consecutive keygen calls produce different keys — entropy is
// flowing. (Not a cryptographic test, just a sanity check that
// nobody accidentally wired a fixed seed.)
func TestEncryptKeygen_TwoCallsDiffer(t *testing.T) {
	got := runEncryptScript(t, `
		const a = encrypt.keygen();
		const b = encrypt.keygen();
		const __result = (a.publicKey !== b.publicKey) && (a.privateKey !== b.privateKey);
	`)
	if got != true {
		t.Errorf("expected different keys; got %#v", got)
	}
}

// Single-recipient round-trip: encrypt with the public key, decrypt
// with the matching private key, recover the plaintext byte-for-byte.
func TestEncrypt_SingleRecipientRoundTrip(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygen();
		const ct = encrypt.encrypt("the eagle has landed", k.publicKey);
		const pt = encrypt.decrypt(ct, k.privateKey);
		// Compare bytes by joining decimal codes — easier than wiring
		// TextDecoder which goja doesn't ship.
		const __result = Array.from(pt).join(",");
	`)
	want := "116,104,101,32,101,97,103,108,101,32,104,97,115,32,108,97,110,100,101,100"
	if got != want {
		t.Errorf("round-trip bytes: %v", got)
	}
}

// Multi-recipient: encrypt for two recipients, EACH identity must
// be able to decrypt independently. age's documented multi-recipient
// behaviour.
func TestEncrypt_MultiRecipient(t *testing.T) {
	got := runEncryptScript(t, `
		const a = encrypt.keygen();
		const b = encrypt.keygen();
		const ct = encrypt.encrypt("shared", [a.publicKey, b.publicKey]);
		const pa = encrypt.decrypt(ct, a.privateKey);
		const pb = encrypt.decrypt(ct, b.privateKey);
		const __result = [
			Array.from(pa).join(",") === "115,104,97,114,101,100",
			Array.from(pb).join(",") === "115,104,97,114,101,100",
		].join(",");
	`)
	if got != "true,true" {
		t.Errorf("multi-recipient: %v", got)
	}
}

// A recipient who isn't on the message's stanza list can't decrypt.
// age's parser returns "no identity matched any of the recipients";
// we forward that through.
func TestEncrypt_WrongIdentityFails(t *testing.T) {
	got := runEncryptScript(t, `
		const owner = encrypt.keygen();
		const eve = encrypt.keygen();
		const ct = encrypt.encrypt("private", owner.publicKey);
		let caught = "";
		try { encrypt.decrypt(ct, eve.privateKey); }
		catch (e) { caught = String(e); }
		// age 1.3 phrases it "identity did not match any of the
		// recipients"; older versions said "no identity matched". Test
		// for the discriminating substring instead of the full message.
		const __result = caught.includes("did not match") || caught.includes("no identity matched");
	`)
	if got != true {
		t.Errorf("wrong identity should throw 'no identity matched'; got %#v", got)
	}
}

// Cross-check: passing a private key where a recipient is expected
// throws with a named-key hint. Without this guard a script that
// mixes up the two halves would either (a) encrypt to nothing useful
// or (b) get a cryptic bech32 parse error.
func TestEncrypt_PrivateKeyAsRecipientThrows(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygen();
		let msg = "";
		try { encrypt.encrypt("x", k.privateKey); }
		catch (e) { msg = String(e); }
		const __result = msg.includes("private key") && msg.includes("public");
	`)
	if got != true {
		t.Errorf("private-as-recipient hint: %v", got)
	}
}

// Cross-check the other direction: a public key where an identity
// is expected throws with the inverse hint.
func TestEncrypt_PublicKeyAsIdentityThrows(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygen();
		const ct = encrypt.encrypt("x", k.publicKey);
		let msg = "";
		try { encrypt.decrypt(ct, k.publicKey); }
		catch (e) { msg = String(e); }
		const __result = msg.includes("public key") && msg.includes("private");
	`)
	if got != true {
		t.Errorf("public-as-identity hint: %v", got)
	}
}

// Empty / missing args throw early with binding-named errors.
func TestEncrypt_InputValidation(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("encrypt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return encryptNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, src, want string
	}{
		{"no recipients", `encrypt.encrypt("x", []);`, "recipient"},
		{"empty cipher", `encrypt.decrypt("", "AGE-SECRET-KEY-1XYZ");`, "empty"},
		{"no identities", `const k = encrypt.keygen(); encrypt.decrypt(encrypt.encrypt("x", k.publicKey), []);`, "identit"},
		{"bad recipient string", `encrypt.encrypt("x", "not-a-recipient");`, "age1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := eng.Run(context.Background(), "v.ts", tc.src)
			if err == nil {
				t.Fatalf("expected throw")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error wording: %v (want substring %q)", err, tc.want)
			}
		})
	}
}

// Non-age ciphertext bytes throw with a clear "encrypt.decrypt"
// prefix rather than letting age's own error bubble unfiltered.
func TestEncrypt_NonAgeCiphertextThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("encrypt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return encryptNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "v.ts", `
		const k = encrypt.keygen();
		encrypt.decrypt("this is not age ciphertext at all", k.privateKey);
	`)
	if err == nil {
		t.Fatal("expected throw for non-age ciphertext")
	}
	if !strings.Contains(err.Error(), "encrypt.decrypt") {
		t.Errorf("expected sercon-prefixed error; got %v", err)
	}
}

// opts.armored true produces age's ASCII-armoured format — payload
// starts with the literal `-----BEGIN AGE ENCRYPTED FILE-----`
// banner. Round-trips through the same decrypt call.
func TestEncrypt_ArmoredOutputAndRoundTrip(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygen();
		const ct = encrypt.encrypt("armoured hi", k.publicKey, { armored: true });
		// Read first chars of the banner without TextDecoder.
		const head = Array.from(ct).slice(0, 34)
			.map(b => String.fromCharCode(b)).join("");
		const pt = encrypt.decrypt(ct, k.privateKey);
		const ptStr = Array.from(pt).map(b => String.fromCharCode(b)).join("");
		const __result = [head, ptStr].join(" | ");
	`)
	want := "-----BEGIN AGE ENCRYPTED FILE----- | armoured hi"
	if got != want {
		t.Errorf("got %v\nwant %s", got, want)
	}
}

// Armored ciphertext as a JS string (the natural shape after pasting
// into a script or reading from JSON) also round-trips. The string
// passes through jsArgToBytes → []byte(s) → armor banner detection.
func TestEncrypt_ArmoredStringRoundTrip(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygen();
		const ctBytes = encrypt.encrypt("through-string", k.publicKey, { armored: true });
		// Convert to a JS string the cheap way — works for ASCII armor.
		const ctStr = Array.from(ctBytes)
			.map(b => String.fromCharCode(b)).join("");
		const pt = encrypt.decrypt(ctStr, k.privateKey);
		const __result = Array.from(pt).map(b => String.fromCharCode(b)).join("");
	`)
	if got != "through-string" {
		t.Errorf("string-form round-trip: %v", got)
	}
}

// opts.armored false / unset / missing all produce binary output —
// the existing v0.5.5 contract is unchanged.
func TestEncrypt_DefaultStaysBinary(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygen();
		const a = encrypt.encrypt("x", k.publicKey);                        // unset
		const b = encrypt.encrypt("x", k.publicKey, {});                    // missing
		const c = encrypt.encrypt("x", k.publicKey, { armored: false });    // explicit false
		const head = (ct) => Array.from(ct).slice(0, 21)
			.map(b => String.fromCharCode(b)).join("");
		const __result = [head(a), head(b), head(c)].every(
			h => h === "age-encryption.org/v1") + "";
	`)
	if got != "true" {
		t.Errorf("expected all three to start with 'age-encryption.org/v1' (binary header); got %v", got)
	}
}

// looksArmored unit-test. The detector ignores leading whitespace
// (a common artefact of pasting from JSON/email containers) and
// matches the full banner — not just the `-----BEGIN` prefix that
// PEM keys share.
func TestLooksArmored(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want bool
	}{
		{"exact banner", []byte("-----BEGIN AGE ENCRYPTED FILE-----\n..."), true},
		{"leading whitespace", []byte("\n  -----BEGIN AGE ENCRYPTED FILE-----\n..."), true},
		{"BOM prefix", []byte{0xff, 0xfe, '-', '-', '-', '-', '-', 'B', 'E', 'G', 'I', 'N', ' ', 'A', 'G', 'E', ' ', 'E', 'N', 'C', 'R', 'Y', 'P', 'T', 'E', 'D', ' ', 'F', 'I', 'L', 'E', '-', '-', '-', '-', '-'}, true},
		{"PEM private key", []byte("-----BEGIN PRIVATE KEY-----\n..."), false},
		{"binary age header", []byte("age-encryption.org/v1\n..."), false},
		{"empty", []byte{}, false},
		{"plain text", []byte("hello world"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksArmored(tc.data); got != tc.want {
				t.Errorf("looksArmored(%q): %v (want %v)", tc.name, got, tc.want)
			}
		})
	}
}

// Armored ciphertext signed with a private key not in the recipient
// list still surfaces the "no identity matched" error wording — the
// armor layer is transparent to identity checking.
func TestEncrypt_ArmoredWrongIdentityErrors(t *testing.T) {
	got := runEncryptScript(t, `
		const owner = encrypt.keygen();
		const eve = encrypt.keygen();
		const ct = encrypt.encrypt("private", owner.publicKey, { armored: true });
		let caught = "";
		try { encrypt.decrypt(ct, eve.privateKey); }
		catch (e) { caught = String(e); }
		const __result = caught.includes("did not match") || caught.includes("no identity matched");
	`)
	if got != true {
		t.Errorf("armored wrong-identity error wording: %v", got)
	}
}

// Rekey round-trip: encrypt for alice, rekey to bob, bob decrypts.
// Plaintext byte-for-byte recovered.
func TestEncryptRekey_BinaryRoundTrip(t *testing.T) {
	got := runEncryptScript(t, `
		const alice = encrypt.keygen();
		const bob = encrypt.keygen();
		const orig = encrypt.encrypt("rotate me please", alice.publicKey);
		const rk = encrypt.rekey(orig, alice.privateKey, bob.publicKey);
		const pt = encrypt.decrypt(rk, bob.privateKey);
		const __result = Array.from(pt).map(b => String.fromCharCode(b)).join("");
	`)
	if got != "rotate me please" {
		t.Errorf("rekey round-trip: %v", got)
	}
}

// After rekey, the original identity can no longer decrypt — the
// recipient set has actually changed, not just been padded.
func TestEncryptRekey_OldIdentityLockedOut(t *testing.T) {
	got := runEncryptScript(t, `
		const alice = encrypt.keygen();
		const bob = encrypt.keygen();
		const orig = encrypt.encrypt("secret", alice.publicKey);
		const rk = encrypt.rekey(orig, alice.privateKey, bob.publicKey);
		let caught = "";
		try { encrypt.decrypt(rk, alice.privateKey); }
		catch (e) { caught = String(e); }
		const __result = caught.includes("did not match") || caught.includes("no identity matched");
	`)
	if got != true {
		t.Errorf("expected alice to be locked out after rekey; got %#v", got)
	}
}

// Format preservation: armored in → armored out (default behaviour).
func TestEncryptRekey_ArmoredPreserved(t *testing.T) {
	got := runEncryptScript(t, `
		const alice = encrypt.keygen();
		const bob = encrypt.keygen();
		const orig = encrypt.encrypt("a", alice.publicKey, { armored: true });
		const rk = encrypt.rekey(orig, alice.privateKey, bob.publicKey);
		const head = Array.from(rk).slice(0, 34).map(b => String.fromCharCode(b)).join("");
		const __result = head;
	`)
	if got != "-----BEGIN AGE ENCRYPTED FILE-----" {
		t.Errorf("armored preservation: %v", got)
	}
}

// Format preservation: binary in → binary out (default behaviour).
// We check by sniffing the first bytes — age binary starts with
// "age-encryption.org/v1".
func TestEncryptRekey_BinaryPreserved(t *testing.T) {
	got := runEncryptScript(t, `
		const alice = encrypt.keygen();
		const bob = encrypt.keygen();
		const orig = encrypt.encrypt("a", alice.publicKey);            // binary
		const rk = encrypt.rekey(orig, alice.privateKey, bob.publicKey);
		const head = Array.from(rk).slice(0, 21).map(b => String.fromCharCode(b)).join("");
		const __result = head;
	`)
	if got != "age-encryption.org/v1" {
		t.Errorf("binary preservation: %v", got)
	}
}

// Format override: binary in + opts.armored=true → armored out.
func TestEncryptRekey_OverrideToArmored(t *testing.T) {
	got := runEncryptScript(t, `
		const alice = encrypt.keygen();
		const bob = encrypt.keygen();
		const orig = encrypt.encrypt("a", alice.publicKey);              // binary
		const rk = encrypt.rekey(orig, alice.privateKey, bob.publicKey, { armored: true });
		const head = Array.from(rk).slice(0, 34).map(b => String.fromCharCode(b)).join("");
		const __result = head;
	`)
	if got != "-----BEGIN AGE ENCRYPTED FILE-----" {
		t.Errorf("override to armored: %v", got)
	}
}

// Format override: armored in + opts.armored=false → binary out.
func TestEncryptRekey_OverrideToBinary(t *testing.T) {
	got := runEncryptScript(t, `
		const alice = encrypt.keygen();
		const bob = encrypt.keygen();
		const orig = encrypt.encrypt("a", alice.publicKey, { armored: true });
		const rk = encrypt.rekey(orig, alice.privateKey, bob.publicKey, { armored: false });
		const head = Array.from(rk).slice(0, 21).map(b => String.fromCharCode(b)).join("");
		const __result = head;
	`)
	if got != "age-encryption.org/v1" {
		t.Errorf("override to binary: %v", got)
	}
}

// Wrong oldIdentity (not one of the original recipients) throws
// with sercon's `encrypt.rekey:` prefix, NOT the plain
// `encrypt.decrypt:` from the inner call.
func TestEncryptRekey_WrongOldIdentityThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 3 * time.Second})
	if err := eng.RegisterNamespaceFactory("encrypt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return encryptNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "v.ts", `
		const owner = encrypt.keygen();
		const eve = encrypt.keygen();
		const new_ = encrypt.keygen();
		const ct = encrypt.encrypt("x", owner.publicKey);
		encrypt.rekey(ct, eve.privateKey, new_.publicKey);   // eve can't decrypt
	`)
	if err == nil {
		t.Fatal("expected throw for wrong oldIdentity")
	}
	if !strings.Contains(err.Error(), "encrypt.rekey") {
		t.Errorf("expected encrypt.rekey: prefix; got %v", err)
	}
}

// Multi-recipient on the new side: both new readers can decrypt.
// Demonstrates that rekey accepts the same string-or-array shape
// as encrypt.
func TestEncryptRekey_MultiNewRecipient(t *testing.T) {
	got := runEncryptScript(t, `
		const orig_owner = encrypt.keygen();
		const r1 = encrypt.keygen();
		const r2 = encrypt.keygen();
		const ct = encrypt.encrypt("shared rekey", orig_owner.publicKey);
		const rk = encrypt.rekey(ct, orig_owner.privateKey, [r1.publicKey, r2.publicKey]);
		const p1 = Array.from(encrypt.decrypt(rk, r1.privateKey)).map(b => String.fromCharCode(b)).join("");
		const p2 = Array.from(encrypt.decrypt(rk, r2.privateKey)).map(b => String.fromCharCode(b)).join("");
		const __result = [p1 === "shared rekey", p2 === "shared rekey"].join(",");
	`)
	if got != "true,true" {
		t.Errorf("multi-new-recipient: %v", got)
	}
}

// Cross-check: passing a public key as an oldIdentity throws with
// the inverse hint (an oldIdentity is a private key).
func TestEncryptRekey_PublicKeyAsOldIdentityThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("encrypt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return encryptNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "v.ts", `
		const owner = encrypt.keygen();
		const new_ = encrypt.keygen();
		const ct = encrypt.encrypt("x", owner.publicKey);
		encrypt.rekey(ct, owner.publicKey, new_.publicKey);   // public used as identity
	`)
	if err == nil {
		t.Fatal("expected throw for public-as-old-identity")
	}
	if !strings.Contains(err.Error(), "public key") {
		t.Errorf("expected named-key cross-check; got %v", err)
	}
}

// Input validation: empty ciphertext, empty old/new lists.
func TestEncryptRekey_InputValidation(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 2 * time.Second})
	if err := eng.RegisterNamespaceFactory("encrypt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return encryptNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, src, want string }{
		{"empty ciphertext", `encrypt.rekey("", "AGE-SECRET-KEY-1X", "age1x");`, "empty"},
		{"empty oldIdentities", `
			const k = encrypt.keygen();
			const ct = encrypt.encrypt("x", k.publicKey);
			encrypt.rekey(ct, [], k.publicKey);
		`, "oldIdentity"},
		{"empty newRecipients", `
			const k = encrypt.keygen();
			const ct = encrypt.encrypt("x", k.publicKey);
			encrypt.rekey(ct, k.privateKey, []);
		`, "newRecipient"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := eng.Run(context.Background(), "v.ts", tc.src)
			if err == nil {
				t.Fatalf("expected throw for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error wording for %s: %v", tc.name, err)
			}
		})
	}
}

// detectBackend classifies recipient / identity strings into the
// backend they belong to plus public/private kind. Pure prefix
// matching; no parsing or I/O.
func TestEncryptDetectBackend(t *testing.T) {
	cases := []struct {
		name, input, wantBackend, wantKind string
	}{
		// age — bech32 forms
		{"age1 recipient", "age1abcdef1234567890", "age", "public"},
		{"AGE-SECRET-KEY identity", "AGE-SECRET-KEY-1XYZ123", "age", "private"},
		{"age-secret-key lowercase", "age-secret-key-1xyz123", "age", "private"},
		// age — SSH recipient forms (age accepts these natively)
		{"ssh-rsa recipient", "ssh-rsa AAAAB3NzaC1yc2E...", "age", "public"},
		{"ssh-ed25519 recipient", "ssh-ed25519 AAAAC3NzaC1lZDI1...", "age", "public"},
		// PGP — armored block markers
		{"PGP public key block", "-----BEGIN PGP PUBLIC KEY BLOCK-----\nVersion...", "pgp", "public"},
		{"PGP private key block", "-----BEGIN PGP PRIVATE KEY BLOCK-----\nVersion...", "pgp", "private"},
		// Whitespace tolerance
		{"leading whitespace", "  \nage1xyz", "age", "public"},
		{"trailing whitespace", "age1xyz   ", "age", "public"},
		// Unknown cases — anything that doesn't match should NOT be
		// guessed at, even if it looks key-shaped. False positives
		// here would route plaintext to the wrong backend.
		{"empty string", "", "unknown", ""},
		{"plain text", "hello world", "unknown", ""},
		{"PEM private key", "-----BEGIN PRIVATE KEY-----\n...", "unknown", ""},
		{"RSA PRIVATE KEY", "-----BEGIN RSA PRIVATE KEY-----", "unknown", ""},
		{"PGP message block", "-----BEGIN PGP MESSAGE-----", "unknown", ""},
		{"ssh-dss not supported", "ssh-dss AAAA...", "unknown", ""},
		{"agent-1 (not age)", "agent-1xyz", "unknown", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runEncryptScript(t, `
				const r = encrypt.detectBackend(`+toJSStringLit(tc.input)+`);
				const __result = JSON.stringify(r);
			`)
			gotStr, _ := got.(string)
			wantBackend := `"backend":"` + tc.wantBackend + `"`
			if !strings.Contains(gotStr, wantBackend) {
				t.Errorf("backend: got %v, want substring %s", gotStr, wantBackend)
			}
			if tc.wantKind != "" {
				wantKind := `"kind":"` + tc.wantKind + `"`
				if !strings.Contains(gotStr, wantKind) {
					t.Errorf("kind: got %v, want substring %s", gotStr, wantKind)
				}
			} else {
				if strings.Contains(gotStr, `"kind"`) {
					t.Errorf("expected no kind field for unknown backend; got %v", gotStr)
				}
			}
		})
	}
}

// toJSStringLit returns a JS source-code string literal for s — backtick
// would be cleaner but s can contain `${`, so go with double quotes plus
// careful escaping. Newlines, double quotes, and backslashes get escaped;
// other control chars get \xHH form.
func toJSStringLit(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, c := range s {
		switch c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if c < 32 {
				b.WriteString("\\x")
				const hex = "0123456789abcdef"
				b.WriteByte(hex[(c>>4)&0xF])
				b.WriteByte(hex[c&0xF])
			} else {
				b.WriteRune(c)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Round-trip integration: detectBackend agrees with what encrypt and
// rekey accept. An "age public" classification means encrypt-ing TO
// that input works; an "age private" classification means
// decrypt-ing WITH it works.
func TestEncryptDetectBackend_AgreesWithRoundTrip(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygen();
		const pubClass = encrypt.detectBackend(k.publicKey);
		const privClass = encrypt.detectBackend(k.privateKey);

		// Confirm the classification corresponds to what the binding accepts.
		const ct = encrypt.encrypt("x", k.publicKey);   // uses age public
		const pt = encrypt.decrypt(ct, k.privateKey);   // uses age private
		const ptStr = Array.from(pt).map(b => String.fromCharCode(b)).join("");

		const __result = [
			pubClass.backend === "age" && pubClass.kind === "public",
			privClass.backend === "age" && privClass.kind === "private",
			ptStr === "x",
		].join(",");
	`)
	if got != "true,true,true" {
		t.Errorf("classification / round-trip integration: %v", got)
	}
}

// PGP keygen + encrypt + decrypt round-trip. keygenPgp returns
// armored public + private blocks; encrypt produces an armored PGP
// MESSAGE; decrypt with the private block recovers the plaintext.
func TestEncrypt_PGPRoundTrip(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygenPgp({ name: "Alice", email: "alice@example.com" });
		const ct = encrypt.encrypt("pgp secret", k.publicKey);
		const pt = encrypt.decrypt(ct, k.privateKey);
		const ptStr = Array.from(pt).map(b => String.fromCharCode(b)).join("");
		const __result = [
			k.publicKey.startsWith("-----BEGIN PGP PUBLIC KEY BLOCK-----"),
			k.privateKey.startsWith("-----BEGIN PGP PRIVATE KEY BLOCK-----"),
			ptStr,
		].join("|");
	`)
	if got != "true|true|pgp secret" {
		t.Errorf("PGP round-trip: %v", got)
	}
}

// detectBackend already classifies the PGP keys keygenPgp produces.
func TestEncrypt_PGPDetectBackend(t *testing.T) {
	got := runEncryptScript(t, `
		const k = encrypt.keygenPgp({});
		const pub = encrypt.detectBackend(k.publicKey);
		const priv = encrypt.detectBackend(k.privateKey);
		const __result = [pub.backend, pub.kind, priv.backend, priv.kind].join(",");
	`)
	if got != "pgp,public,pgp,private" {
		t.Errorf("detectBackend on PGP keys: %v", got)
	}
}

// A PGP message decrypted with the wrong private key throws (no
// matching key in the keyring).
func TestEncrypt_PGPWrongKeyThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir(), Timeout: 10 * time.Second})
	if err := eng.RegisterNamespaceFactory("encrypt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return encryptNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	_, err := eng.Run(context.Background(), "x.ts", `
		const owner = encrypt.keygenPgp({});
		const eve = encrypt.keygenPgp({});
		const ct = encrypt.encrypt("secret", owner.publicKey);
		encrypt.decrypt(ct, eve.privateKey);
	`)
	if err == nil {
		t.Fatal("expected throw for wrong PGP key")
	}
	if !strings.Contains(err.Error(), "encrypt.decrypt") {
		t.Errorf("expected encrypt.decrypt prefix; got %v", err)
	}
}

// age and PGP stay independent — an age payload doesn't accidentally
// route through the PGP path and vice versa. (Regression guard for
// the dispatch.)
func TestEncrypt_AgeAndPGPDontCross(t *testing.T) {
	got := runEncryptScript(t, `
		const age = encrypt.keygen();
		const pgp = encrypt.keygenPgp({});
		const ageCt = encrypt.encrypt("age msg", age.publicKey);
		const pgpCt = encrypt.encrypt("pgp msg", pgp.publicKey);
		const a = Array.from(encrypt.decrypt(ageCt, age.privateKey)).map(b=>String.fromCharCode(b)).join("");
		const p = Array.from(encrypt.decrypt(pgpCt, pgp.privateKey)).map(b=>String.fromCharCode(b)).join("");
		const __result = [a, p].join("|");
	`)
	if got != "age msg|pgp msg" {
		t.Errorf("age/PGP independence: %v", got)
	}
}
