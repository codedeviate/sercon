package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/dop251/goja"
)

// encryptNamespace wires `api.crypto.encrypt.*`. v0.5.5 ships the core
// round-trip — keygen / encrypt / decrypt — over age's X25519
// identity flavour (`age1...` recipients, `AGE-SECRET-KEY-1...`
// identities). Armoured output, rekeying, and a PGP detection
// helper round out the OUT-OF-SCOPE entry and land in follow-up
// cuts.
//
// All three members are synchronous: age encryption is pure CPU
// work, the API surface is small (no streaming handles), and the
// existing crypto bindings (`api.crypto.jwt`, `api.crypto.hash`) are sync too —
// matching that pattern keeps the call shape uniform.
func encryptNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"keygen": func() goja.Value { return encryptKeygen(vm) },
		"encrypt": func(data, recipients, opts goja.Value) goja.Value {
			return encryptEncrypt(vm, data, recipients, opts)
		},
		"decrypt": func(ciphertext, identities goja.Value) goja.Value {
			return encryptDecrypt(vm, ciphertext, identities)
		},
		"rekey": func(ciphertext, oldIdentities, newRecipients, opts goja.Value) goja.Value {
			return encryptRekey(vm, ciphertext, oldIdentities, newRecipients, opts)
		},
		"detectBackend": func(input string) goja.Value { return encryptDetectBackend(vm, input) },
		"keygenPgp": func(opts goja.Value) goja.Value {
			name := optString(asMap(opts), "name", "")
			email := optString(asMap(opts), "email", "")
			pub, priv, err := pgpKeygen(name, email)
			if err != nil {
				panic(vm.NewGoError(fmt.Errorf("encrypt.keygenPgp: %w", err)))
			}
			return vm.ToValue(map[string]any{"publicKey": pub, "privateKey": priv})
		},
	}
}

// isPGPPublicBlock / isPGPPrivateBlock cheaply classify a key string
// so encrypt / decrypt can route to the PGP path. The exact armor
// banner is the discriminator (age keys are bech32, never start with
// `-----BEGIN PGP`).
func isPGPPublicBlock(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "-----BEGIN PGP PUBLIC KEY BLOCK-----")
}

func isPGPPrivateBlock(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "-----BEGIN PGP PRIVATE KEY BLOCK-----")
}

// encryptDetectBackend classifies a recipient or identity string by
// the encryption backend it belongs to. Pure prefix matching — no
// parsing, no I/O, no new dependencies. Useful for scripts that need
// to dispatch on the format ("is this an age recipient or a PGP
// public key block?") before deciding which encrypt path to take.
//
// Return shape:
//
//	{ backend: "age" | "pgp" | "unknown", kind?: "public" | "private" }
//
// `kind` is only present when the backend was identified. age has
// three input forms — bech32 X25519 (`age1...`) and SSH public-key
// formats (`ssh-rsa ...`, `ssh-ed25519 ...`) are recipients;
// `AGE-SECRET-KEY-1...` is an identity. PGP armored blocks
// (`-----BEGIN PGP PUBLIC KEY BLOCK-----` / `... PRIVATE KEY BLOCK-----`)
// classify cleanly. Anything else returns `{ backend: "unknown" }`
// so callers can branch deterministically without parsing the input.
//
// This v0.5.8 cut is the classifier only — sercon's actual
// `encrypt` / `decrypt` paths still only handle age. A future cut
// would extend the encrypt path to also accept PGP recipients
// detected here. The classifier is useful standalone for routing
// (e.g., a script that reads recipient strings from a config file
// and decides whether to call `api.crypto.encrypt.encrypt` or shell out
// to `gpg --encrypt`).
func encryptDetectBackend(vm *goja.Runtime, input string) goja.Value {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return vm.ToValue(map[string]any{"backend": "unknown"})
	}

	// Age — bech32 X25519 forms come first because they're the
	// canonical case sercon's own encrypt/decrypt already accepts.
	if strings.HasPrefix(trimmed, "age1") {
		return vm.ToValue(map[string]any{"backend": "age", "kind": "public"})
	}
	if strings.HasPrefix(strings.ToUpper(trimmed), "AGE-SECRET-KEY-") {
		return vm.ToValue(map[string]any{"backend": "age", "kind": "private"})
	}
	// SSH public keys age accepts as recipients. We don't try to
	// recognise SSH private keys (PEM-style `-----BEGIN OPENSSH ...`)
	// because age 1.x doesn't accept those as identities through the
	// X25519Identity path our binding uses — adding them would need a
	// new code path and is left for the future PGP cut to design
	// alongside.
	if strings.HasPrefix(trimmed, "ssh-rsa ") || strings.HasPrefix(trimmed, "ssh-ed25519 ") {
		return vm.ToValue(map[string]any{"backend": "age", "kind": "public"})
	}

	// PGP — armored block markers. Trimming TrimSpace at the top
	// already handled leading whitespace; the markers are exact.
	switch {
	case strings.HasPrefix(trimmed, "-----BEGIN PGP PUBLIC KEY BLOCK-----"):
		return vm.ToValue(map[string]any{"backend": "pgp", "kind": "public"})
	case strings.HasPrefix(trimmed, "-----BEGIN PGP PRIVATE KEY BLOCK-----"):
		return vm.ToValue(map[string]any{"backend": "pgp", "kind": "private"})
	}

	return vm.ToValue(map[string]any{"backend": "unknown"})
}

// encryptKeygen creates a fresh X25519 identity and returns both
// halves as the bech32 strings age uses on disk. `publicKey` is the
// `age1...` recipient (safe to share); `privateKey` is the
// `AGE-SECRET-KEY-1...` identity (must be kept secret).
//
// Scripts that want both halves in one go can destructure:
//
//	const { publicKey, privateKey } = api.crypto.encrypt.keygen();
func encryptKeygen(vm *goja.Runtime) goja.Value {
	id, err := age.GenerateX25519Identity()
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.keygen: %w", err)))
	}
	return vm.ToValue(map[string]any{
		"publicKey":  id.Recipient().String(),
		"privateKey": id.String(),
	})
}

// encryptEncrypt seals `data` to one or more recipients. `data`
// accepts string / Uint8Array / ArrayBuffer. `recipients` accepts a
// single string or an array; each string is a bech32-encoded
// `age1...` public key (passing a private key by mistake throws a
// clear error rather than encrypting to an unintended key).
//
// Default output is the binary age format. Passing
// `opts.armored: true` wraps it in age's ASCII armor — the
// `-----BEGIN AGE ENCRYPTED FILE-----` banner + base64 body that's
// safe to embed in JSON / YAML / email. Either form decrypts via
// the same `decrypt` call (auto-detected by the leading bytes).
//
// Multi-recipient encryption uses age's native multi-recipient
// header: any one of the listed identities can decrypt the
// resulting ciphertext.
func encryptEncrypt(vm *goja.Runtime, dataArg, recipientsArg, optsArg goja.Value) goja.Value {
	data, err := jsArgToBytes(dataArg)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: data %w", err)))
	}
	recipientStrs, err := stringOrStringSlice(recipientsArg, "recipients")
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: %w", err)))
	}
	if len(recipientStrs) == 0 {
		panic(vm.NewGoError(errors.New("encrypt.encrypt: at least one recipient required")))
	}

	// PGP dispatch: if the first recipient is a PGP public-key block,
	// route the whole call through openpgp. Mixed age + PGP recipient
	// sets aren't supported — the formats are incompatible, so all
	// recipients must be the same backend. opts.armored is ignored on
	// the PGP path (PGP output is always ASCII-armored).
	if isPGPPublicBlock(recipientStrs[0]) {
		out, err := pgpEncrypt(data, recipientStrs)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: %w", err)))
		}
		return vm.ToValue(out)
	}

	recipients := make([]age.Recipient, 0, len(recipientStrs))
	for _, s := range recipientStrs {
		r, err := parseRecipientStrict(s)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: recipient %q: %w", truncForError(s), err)))
		}
		recipients = append(recipients, r)
	}

	armored := optBool(asMap(optsArg), "armored", false)

	var out bytes.Buffer
	// When armored, we stack: age.Encrypt → armor.NewWriter → out.
	// Both writers need Close() — age finalises its trailer first,
	// then armor flushes its base64-encoded END banner. Closing in
	// the wrong order produces truncated output that decrypts as
	// "unexpected EOF" rather than the "no recipients" misnomer
	// `defer` would create here.
	var dst io.Writer = &out
	var armorW io.WriteCloser
	if armored {
		armorW = armor.NewWriter(&out)
		dst = armorW
	}
	w, err := age.Encrypt(dst, recipients...)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: %w", err)))
	}
	if _, err := w.Write(data); err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: write: %w", err)))
	}
	if err := w.Close(); err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: close: %w", err)))
	}
	if armorW != nil {
		if err := armorW.Close(); err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: armor close: %w", err)))
		}
	}
	return vm.ToValue(out.Bytes())
}

// encryptDecrypt opens an age-encrypted payload using one of the
// supplied identities. `ciphertext` accepts string / Uint8Array /
// ArrayBuffer. `identities` accepts a single string or an array;
// each is a bech32 `AGE-SECRET-KEY-1...` private key. Returns the
// plaintext as Uint8Array — let scripts `new TextDecoder().decode(...)`
// if they want a string, since we don't know the payload's encoding.
//
// `age.Decrypt` walks the supplied identities and uses the first
// one that matches a recipient stanza in the header. None matching
// surfaces as `"no identity matched any of the recipients"` — that
// string comes from age itself; we forward it through so scripts
// can pattern-match on it if needed.
func encryptDecrypt(vm *goja.Runtime, ciphertextArg, identitiesArg goja.Value) goja.Value {
	ciphertext, err := jsArgToBytes(ciphertextArg)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: ciphertext %w", err)))
	}
	if len(ciphertext) == 0 {
		panic(vm.NewGoError(errors.New("encrypt.decrypt: ciphertext is empty")))
	}
	identityStrs, err := stringOrStringSlice(identitiesArg, "identities")
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: %w", err)))
	}
	if len(identityStrs) == 0 {
		panic(vm.NewGoError(errors.New("encrypt.decrypt: at least one identity required")))
	}

	// PGP dispatch: route through openpgp when the identities are PGP
	// private-key blocks, or the ciphertext is an armored PGP message.
	// Either signal is sufficient; in practice they travel together.
	if isPGPPrivateBlock(identityStrs[0]) || looksPGPMessage(ciphertext) {
		out, err := pgpDecrypt(ciphertext, identityStrs)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: %w", err)))
		}
		return vm.ToValue(out)
	}

	identities := make([]age.Identity, 0, len(identityStrs))
	for _, s := range identityStrs {
		id, err := parseIdentityStrict(s)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: identity %q: %w", truncForError(s), err)))
		}
		identities = append(identities, id)
	}

	// Auto-detect armored ciphertext by sniffing the leading bytes
	// for age's literal banner. Stripping leading whitespace first
	// covers ciphertext pasted from email or JSON where a stray
	// newline or BOM might prefix the payload — both cases are
	// safe to discard before banner detection.
	var src io.Reader = bytes.NewReader(ciphertext)
	if looksArmored(ciphertext) {
		src = armor.NewReader(src)
	}

	r, err := age.Decrypt(src, identities...)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: %w", err)))
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: read: %w", err)))
	}
	return vm.ToValue(plain)
}

// encryptRekey re-encrypts a ciphertext for a fresh recipient set
// without exposing the plaintext to the caller. Internally it does
// `decrypt → in-memory plaintext → encrypt`, so the plaintext lives
// only in this function's stack until the encrypt step finishes. The
// payload size determines the buffer size; this isn't suitable for
// multi-GB streams but is fine for the typical "rotate keys on a
// secrets blob" use case.
//
// Output format defaults to **match the input** — armored in / armored
// out, binary in / binary out — which is what you almost always want
// when key-rotating a payload that lives in a fixed location (file,
// vault row, JSON field). Pass `opts.armored: true|false` to force the
// output regardless of input.
//
// Cross-checks: oldIdentities must look like private keys
// (AGE-SECRET-KEY-...); newRecipients must look like public keys
// (age1...). Empty either-side throws. A decrypt failure (no
// identity matched, malformed input) propagates with sercon's
// `encrypt.rekey:` prefix so it's clear which step failed.
func encryptRekey(vm *goja.Runtime, ciphertextArg, oldIdentitiesArg, newRecipientsArg, optsArg goja.Value) goja.Value {
	ciphertext, err := jsArgToBytes(ciphertextArg)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: ciphertext %w", err)))
	}
	if len(ciphertext) == 0 {
		panic(vm.NewGoError(errors.New("encrypt.rekey: ciphertext is empty")))
	}

	oldStrs, err := stringOrStringSlice(oldIdentitiesArg, "oldIdentities")
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: %w", err)))
	}
	if len(oldStrs) == 0 {
		panic(vm.NewGoError(errors.New("encrypt.rekey: at least one oldIdentity required")))
	}
	oldIdentities := make([]age.Identity, 0, len(oldStrs))
	for _, s := range oldStrs {
		id, err := parseIdentityStrict(s)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: oldIdentity %q: %w", truncForError(s), err)))
		}
		oldIdentities = append(oldIdentities, id)
	}

	newStrs, err := stringOrStringSlice(newRecipientsArg, "newRecipients")
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: %w", err)))
	}
	if len(newStrs) == 0 {
		panic(vm.NewGoError(errors.New("encrypt.rekey: at least one newRecipient required")))
	}
	newRecipients := make([]age.Recipient, 0, len(newStrs))
	for _, s := range newStrs {
		r, err := parseRecipientStrict(s)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: newRecipient %q: %w", truncForError(s), err)))
		}
		newRecipients = append(newRecipients, r)
	}

	// Resolve the output format. Default = preserve the input's
	// armor state; opts.armored overrides explicitly.
	inputArmored := looksArmored(ciphertext)
	outputArmored := inputArmored
	if optsMap := asMap(optsArg); optsMap != nil {
		if v, ok := optsMap["armored"].(bool); ok {
			outputArmored = v
		}
	}

	// Stage 1: decrypt with the old identities. Auto-detect armor on
	// the read side, same as encryptDecrypt does.
	var src io.Reader = bytes.NewReader(ciphertext)
	if inputArmored {
		src = armor.NewReader(src)
	}
	dr, err := age.Decrypt(src, oldIdentities...)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: decrypt: %w", err)))
	}
	plain, err := io.ReadAll(dr)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: decrypt read: %w", err)))
	}

	// Stage 2: encrypt for the new recipients. Same writer stacking as
	// encryptEncrypt; close in order (age first, then armor).
	var out bytes.Buffer
	var dst io.Writer = &out
	var armorW io.WriteCloser
	if outputArmored {
		armorW = armor.NewWriter(&out)
		dst = armorW
	}
	ew, err := age.Encrypt(dst, newRecipients...)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: encrypt: %w", err)))
	}
	if _, err := ew.Write(plain); err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: encrypt write: %w", err)))
	}
	if err := ew.Close(); err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: encrypt close: %w", err)))
	}
	if armorW != nil {
		if err := armorW.Close(); err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.rekey: armor close: %w", err)))
		}
	}
	return vm.ToValue(out.Bytes())
}

// looksArmored reports whether ciphertext is in age's ASCII armor.
// The check ignores leading whitespace so payloads pasted from
// email / JSON / YAML containers (where a newline or BOM might
// prefix the body) still detect correctly. The banner itself is
// the literal `-----BEGIN AGE ENCRYPTED FILE-----` constant
// exported by filippo.io/age/armor; matching that exactly avoids
// false positives from PEM keys or other base64-armoured payloads
// that share the `-----BEGIN` prefix.
func looksArmored(b []byte) bool {
	trimmed := bytes.TrimLeft(b, " \t\r\n\xff\xfe")
	return bytes.HasPrefix(trimmed, []byte(armor.Header))
}

// parseRecipientStrict wraps age.ParseX25519Recipient with the
// "are you sure this is a public key?" sanity check — passing an
// AGE-SECRET-KEY-1... string here is a footgun (the script meant to
// keep that secret) and age's own parser would emit a generic
// bech32 error that's hard to action. Catch it ourselves with a
// pointed hint.
func parseRecipientStrict(s string) (age.Recipient, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty string")
	}
	if strings.HasPrefix(strings.ToUpper(s), "AGE-SECRET-KEY-") {
		return nil, errors.New("looks like a private key (AGE-SECRET-KEY-...); recipients are public keys (age1...)")
	}
	if !strings.HasPrefix(s, "age1") {
		return nil, errors.New("not an age1... public key")
	}
	r, err := age.ParseX25519Recipient(s)
	if err != nil {
		return nil, err
	}
	return r, nil
}

// parseIdentityStrict is the inverse cross-check: catches a script
// that handed in a public key string where a private key was
// expected. The hint says "did you mean keygen().privateKey?" so
// new users know what to look for.
func parseIdentityStrict(s string) (age.Identity, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty string")
	}
	if strings.HasPrefix(s, "age1") {
		return nil, errors.New("looks like a public key (age1...); identities are private keys (AGE-SECRET-KEY-1...)")
	}
	if !strings.HasPrefix(strings.ToUpper(s), "AGE-SECRET-KEY-") {
		return nil, errors.New("not an AGE-SECRET-KEY-1... private key")
	}
	id, err := age.ParseX25519Identity(s)
	if err != nil {
		return nil, err
	}
	return id, nil
}

// stringOrStringSlice accepts either a single string or an array of
// strings from JS. Returns []string with everything trimmed; empty
// strings inside an array are dropped (it's almost always a mistake
// — a trailing comma in a literal). Used by encrypt + decrypt for
// their recipients / identities args.
func stringOrStringSlice(v goja.Value, label string) ([]string, error) {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil, fmt.Errorf("%s required", label)
	}
	switch ex := v.Export().(type) {
	case string:
		s := strings.TrimSpace(ex)
		if s == "" {
			return nil, fmt.Errorf("%s string is empty", label)
		}
		return []string{s}, nil
	case []any:
		out := make([]string, 0, len(ex))
		for i, item := range ex {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s[%d] is not a string (got %T)", label, i, item)
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	case []string:
		out := make([]string, 0, len(ex))
		for _, s := range ex {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	default:
		return nil, fmt.Errorf("%s must be a string or string[] (got %T)", label, v.Export())
	}
}

// truncForError snips long key strings to a recognisable prefix so
// error messages stay readable. age keys are 62-ish chars long; the
// first 12 are enough to identify the key without flooding logs
// with secrets.
func truncForError(s string) string {
	if len(s) > 24 {
		return s[:12] + "…" + s[len(s)-4:]
	}
	return s
}
