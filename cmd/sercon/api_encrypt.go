package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
	"github.com/dop251/goja"
)

// encryptNamespace wires `api.encrypt.*`. v0.5.5 ships the core
// round-trip — keygen / encrypt / decrypt — over age's X25519
// identity flavour (`age1...` recipients, `AGE-SECRET-KEY-1...`
// identities). Armoured output, rekeying, and a PGP detection
// helper round out the OUT-OF-SCOPE entry and land in follow-up
// cuts.
//
// All three members are synchronous: age encryption is pure CPU
// work, the API surface is small (no streaming handles), and the
// existing crypto bindings (`api.jwt`, `api.hash`) are sync too —
// matching that pattern keeps the call shape uniform.
func encryptNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"keygen":  func() goja.Value { return encryptKeygen(vm) },
		"encrypt": func(data goja.Value, recipients goja.Value) goja.Value { return encryptEncrypt(vm, data, recipients) },
		"decrypt": func(ciphertext goja.Value, identities goja.Value) goja.Value { return encryptDecrypt(vm, ciphertext, identities) },
	}
}

// encryptKeygen creates a fresh X25519 identity and returns both
// halves as the bech32 strings age uses on disk. `publicKey` is the
// `age1...` recipient (safe to share); `privateKey` is the
// `AGE-SECRET-KEY-1...` identity (must be kept secret).
//
// Scripts that want both halves in one go can destructure:
//
//	const { publicKey, privateKey } = api.encrypt.keygen();
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
// clear error rather than encrypting to an unintended key). Output
// is the binary age format as Uint8Array; the armoured ASCII format
// lands in a later cut.
//
// Multi-recipient encryption uses age's native multi-recipient
// header: any one of the listed identities can decrypt the
// resulting ciphertext. This is the documented way to allow several
// people to read the same encrypted message without re-encrypting
// for each.
func encryptEncrypt(vm *goja.Runtime, dataArg, recipientsArg goja.Value) goja.Value {
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

	recipients := make([]age.Recipient, 0, len(recipientStrs))
	for _, s := range recipientStrs {
		r, err := parseRecipientStrict(s)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: recipient %q: %w", truncForError(s), err)))
		}
		recipients = append(recipients, r)
	}

	var out bytes.Buffer
	w, err := age.Encrypt(&out, recipients...)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: %w", err)))
	}
	if _, err := w.Write(data); err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: write: %w", err)))
	}
	if err := w.Close(); err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.encrypt: close: %w", err)))
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

	identities := make([]age.Identity, 0, len(identityStrs))
	for _, s := range identityStrs {
		id, err := parseIdentityStrict(s)
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: identity %q: %w", truncForError(s), err)))
		}
		identities = append(identities, id)
	}

	r, err := age.Decrypt(bytes.NewReader(ciphertext), identities...)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: %w", err)))
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("encrypt.decrypt: read: %w", err)))
	}
	return vm.ToValue(plain)
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
