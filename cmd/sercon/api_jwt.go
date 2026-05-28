package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/golang-jwt/jwt/v5"
	"github.com/lestrrat-go/jwx/v2/jwk"
)

// jwtNamespace wires `api.jwt.*`. Supports the full RFC 7518
// algorithm matrix: HMAC (HS256 / HS384 / HS512), RSA-PKCS1 (RS256
// / RS384 / RS512), RSA-PSS (PS256 / PS384 / PS512), ECDSA (ES256
// / ES384 / ES512), and EdDSA (Ed25519). All three members are
// synchronous — JWT work is pure CPU and doesn't need the event
// loop.
//
// Key shape: the `secret` parameter accepts either a raw HMAC byte
// string OR a PEM-encoded asymmetric key (private for sign, public
// for validate). PEM detection looks for the literal `-----BEGIN`
// header — every standard PEM block starts with it. Cross-checks
// (PEM with HMAC algo, HMAC bytes with asymmetric algo) throw with
// a named-algorithm error so silent fallback to a weaker algo is
// impossible.
func jwtNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"sign": func(claims map[string]any, secret string, opts goja.Value) goja.Value {
			return jwtSign(vm, claims, secret, opts)
		},
		"view":     func(token string) goja.Value { return jwtView(vm, token) },
		"validate": func(token, secret string, opts goja.Value) goja.Value { return jwtValidate(vm, token, secret, opts) },
	}
}

// jwtSigningMethod resolves a name like "HS256" / "RS256" / "ES384"
// / "EdDSA" to a jwt-go SigningMethod. Returns nil for unknown
// names so the caller can produce a uniform "unsupported algorithm"
// error rather than this helper deciding the wording.
func jwtSigningMethod(name string) jwt.SigningMethod {
	switch name {
	case "HS256":
		return jwt.SigningMethodHS256
	case "HS384":
		return jwt.SigningMethodHS384
	case "HS512":
		return jwt.SigningMethodHS512
	case "RS256":
		return jwt.SigningMethodRS256
	case "RS384":
		return jwt.SigningMethodRS384
	case "RS512":
		return jwt.SigningMethodRS512
	case "PS256":
		return jwt.SigningMethodPS256
	case "PS384":
		return jwt.SigningMethodPS384
	case "PS512":
		return jwt.SigningMethodPS512
	case "ES256":
		return jwt.SigningMethodES256
	case "ES384":
		return jwt.SigningMethodES384
	case "ES512":
		return jwt.SigningMethodES512
	case "EdDSA":
		return jwt.SigningMethodEdDSA
	default:
		return nil
	}
}

// jwtSupportedAlgoList is the order-stable list of algorithm names
// reported in error messages and passed to jwt.WithValidMethods. The
// HMAC family comes first because that's the most common case for
// `sercon` scripts that don't already have key material to hand.
var jwtSupportedAlgoList = []string{
	"HS256", "HS384", "HS512",
	"RS256", "RS384", "RS512",
	"PS256", "PS384", "PS512",
	"ES256", "ES384", "ES512",
	"EdDSA",
}

// looksLikePEM is the cheap test we use to decide "treat secret as
// PEM key" vs "treat secret as HMAC bytes". `-----BEGIN` is the
// universal PEM block prefix; nothing legitimate that's meant as
// HMAC bytes would start with it.
func looksLikePEM(s string) bool {
	return strings.HasPrefix(strings.TrimSpace(s), "-----BEGIN")
}

// looksLikeJWK reports whether `secret` is a JSON Web Key — a JSON
// object carrying a `"kty"` member. The cheap structural test is
// "starts with `{` and contains `kty`"; jwk.ParseKey does the real
// validation. HMAC secrets and PEM blocks never start with `{`, so
// this disambiguates cleanly from the other two input forms.
func looksLikeJWK(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "{") && strings.Contains(t, "\"kty\"")
}

// resolveJWKKey parses `secret` as a JWK when it's JWK-shaped and
// extracts the underlying crypto key (the *rsa.PrivateKey /
// *ecdsa.PrivateKey / ed25519 key / []byte that jwt-go's signing
// methods accept directly). The three returns are:
//
//	(key, true,  nil) — was a valid JWK; `key` is the raw crypto key
//	(nil, true,  err) — looked like JWK but failed to parse
//	(nil, false, nil) — not JWK-shaped; caller falls back to PEM / bytes
//
// The `kty` inside the JWK determines the key type, so the
// algorithm-based dispatch in the caller is bypassed for JWK input —
// a mismatch between the JWK's key type and opts.algorithm surfaces
// later as jwt-go's own "key is of invalid type" at sign/verify
// time, which is the right place for it.
func resolveJWKKey(secret string) (any, bool, error) {
	if !looksLikeJWK(secret) {
		return nil, false, nil
	}
	key, err := jwk.ParseKey([]byte(secret))
	if err != nil {
		return nil, true, fmt.Errorf("parse JWK: %w", err)
	}
	var raw any
	if err := key.Raw(&raw); err != nil {
		return nil, true, fmt.Errorf("extract key from JWK: %w", err)
	}
	return raw, true, nil
}

// isHMACAlgorithm reports whether the named algorithm is HMAC-based.
// Used to gate the PEM/bytes cross-checks: HMAC + PEM = mistake,
// asymmetric + plain bytes = mistake.
func isHMACAlgorithm(name string) bool {
	return strings.HasPrefix(name, "HS")
}

// parsePrivateKeyForAlg parses `secret` as the key shape the named
// algorithm needs. The two cross-check error paths catch the most
// common JS-side mistakes — a script that supplies a PEM private
// key but forgets to set `opts.algorithm` would otherwise sign an
// HS256 token over the PEM bytes; conversely, a script that asks
// for RS256 but supplies a plain string secret would get a cryptic
// "x509: malformed certificate" deep inside jwt-go.
func parsePrivateKeyForAlg(secret, algName string) (any, error) {
	if key, ok, err := resolveJWKKey(secret); ok {
		return key, err
	}
	switch {
	case isHMACAlgorithm(algName):
		if looksLikePEM(secret) {
			return nil, fmt.Errorf("algorithm %s is HMAC but secret looks like a PEM-encoded key — set opts.algorithm to RS256 / ES256 / EdDSA / etc., or pass raw bytes for HMAC", algName)
		}
		return []byte(secret), nil
	case !looksLikePEM(secret):
		return nil, fmt.Errorf("algorithm %s needs a PEM-encoded private key but secret is plain bytes (no -----BEGIN header)", algName)
	case strings.HasPrefix(algName, "RS"), strings.HasPrefix(algName, "PS"):
		k, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("parse RSA private key: %w", err)
		}
		return k, nil
	case strings.HasPrefix(algName, "ES"):
		k, err := jwt.ParseECPrivateKeyFromPEM([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("parse EC private key: %w", err)
		}
		return k, nil
	case algName == "EdDSA":
		k, err := jwt.ParseEdPrivateKeyFromPEM([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("parse Ed25519 private key: %w", err)
		}
		return k, nil
	default:
		return nil, fmt.Errorf("unsupported algorithm %q", algName)
	}
}

// parsePublicKeyForAlg is the validate-side counterpart. Accepts
// `-----BEGIN PUBLIC KEY-----` and `-----BEGIN CERTIFICATE-----`
// blocks (the latter via jwt-go's helpers, which pull the public
// key out of the cert). Same cross-check policy as
// parsePrivateKeyForAlg.
func parsePublicKeyForAlg(secret, algName string) (any, error) {
	if key, ok, err := resolveJWKKey(secret); ok {
		return key, err
	}
	switch {
	case isHMACAlgorithm(algName):
		if looksLikePEM(secret) {
			return nil, fmt.Errorf("algorithm %s is HMAC but secret looks like a PEM-encoded key — set opts.algorithm to the asymmetric algo used to sign, or pass raw bytes for HMAC", algName)
		}
		return []byte(secret), nil
	case !looksLikePEM(secret):
		return nil, fmt.Errorf("algorithm %s needs a PEM-encoded public key but secret is plain bytes (no -----BEGIN header)", algName)
	case strings.HasPrefix(algName, "RS"), strings.HasPrefix(algName, "PS"):
		k, err := jwt.ParseRSAPublicKeyFromPEM([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("parse RSA public key: %w", err)
		}
		return k, nil
	case strings.HasPrefix(algName, "ES"):
		k, err := jwt.ParseECPublicKeyFromPEM([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("parse EC public key: %w", err)
		}
		return k, nil
	case algName == "EdDSA":
		k, err := jwt.ParseEdPublicKeyFromPEM([]byte(secret))
		if err != nil {
			return nil, fmt.Errorf("parse Ed25519 public key: %w", err)
		}
		return k, nil
	default:
		return nil, fmt.Errorf("unsupported algorithm %q", algName)
	}
}

// jwtSign produces a signed compact-serialisation JWT. `claims` is
// passed straight through to jwt-go's MapClaims, which recognises
// the RFC 7519 reserved claims and handles arbitrary user-defined
// claims alongside them. Missing claims aren't synthesised — scripts
// that want `iat` set should compute it explicitly.
func jwtSign(vm *goja.Runtime, claims map[string]any, secret string, opts goja.Value) goja.Value {
	if claims == nil {
		panic(vm.NewGoError(errors.New("jwt.sign: claims object required")))
	}
	if secret == "" {
		panic(vm.NewGoError(errors.New("jwt.sign: secret is empty")))
	}
	algName := optAlgorithm(opts, "HS256")
	method := jwtSigningMethod(algName)
	if method == nil {
		panic(vm.NewGoError(fmt.Errorf("jwt.sign: unsupported algorithm %q (supported: %s)", algName, strings.Join(jwtSupportedAlgoList, ", "))))
	}
	key, err := parsePrivateKeyForAlg(secret, algName)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("jwt.sign: %w", err)))
	}
	tok := jwt.NewWithClaims(method, jwt.MapClaims(claims))
	signed, err := tok.SignedString(key)
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("jwt.sign: %w", err)))
	}
	return vm.ToValue(signed)
}

// jwtView decodes a token's header + payload + signature WITHOUT
// verifying the signature. Malformed input throws.
func jwtView(vm *goja.Runtime, token string) goja.Value {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		panic(vm.NewGoError(fmt.Errorf("jwt.view: malformed token (expected 3 dot-separated segments, got %d)", len(parts))))
	}
	header, err := decodeJWTSegment(parts[0])
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("jwt.view: header %w", err)))
	}
	payload, err := decodeJWTSegment(parts[1])
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("jwt.view: payload %w", err)))
	}
	return vm.ToValue(map[string]any{
		"header":    header,
		"payload":   payload,
		"signature": parts[2],
	})
}

// decodeJWTSegment base64url-decodes a JWT segment and JSON-parses
// the result. Accepts both `RawURLEncoding` (no padding, per spec)
// and the rarer padded form.
func decodeJWTSegment(seg string) (map[string]any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		var err2 error
		raw, err2 = base64.URLEncoding.DecodeString(seg)
		if err2 != nil {
			return nil, fmt.Errorf("base64url decode: %w", err)
		}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("json parse: %w", err)
	}
	return out, nil
}

// jwtValidate verifies a token using `secret` and the algorithm
// declared in the token's header. When `opts.algorithm` is set, the
// header's alg must match it exactly — this prevents algorithm
// confusion attacks (a server expecting RS256 won't accept an HS256
// token signed with what was supposed to be a public verification
// key). When unset, any algorithm in jwtSupportedAlgoList is
// accepted.
//
// Resolves (doesn't throw) on every validation failure with
// `{ valid: false, reason }`. Only structural input errors (wrong
// segment count, empty secret) throw.
func jwtValidate(vm *goja.Runtime, token, secret string, opts goja.Value) goja.Value {
	if token == "" {
		panic(vm.NewGoError(errors.New("jwt.validate: token is empty")))
	}
	if secret == "" {
		panic(vm.NewGoError(errors.New("jwt.validate: secret is empty")))
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		panic(vm.NewGoError(fmt.Errorf("jwt.validate: malformed token (expected 3 dot-separated segments, got %d)", len(parts))))
	}

	optsMap := asMap(opts)
	expectedAlg := strings.ToUpper(optString(optsMap, "algorithm", ""))
	// EdDSA is the one algorithm name where ToUpper isn't right —
	// jwt-go's identifier is `EdDSA`, not `EDDSA`. Normalise back.
	if expectedAlg == "EDDSA" {
		expectedAlg = "EdDSA"
	}

	validMethods := jwtSupportedAlgoList
	if expectedAlg != "" {
		validMethods = []string{expectedAlg}
	}

	// Pre-validate the secret-vs-algorithm cross-check. When opts.algorithm
	// is set we know the algo upfront; otherwise we cheaply decode the
	// token's header (we already split parts) so we can throw at this
	// boundary rather than letting the keyfunc surface it as a soft
	// `valid: false` validation failure. Cross-check errors are
	// structural ("you wired this wrong"); they shouldn't be confused
	// with cryptographic verification failures.
	preCheckAlg := expectedAlg
	if preCheckAlg == "" {
		if hdr, err := decodeJWTSegment(parts[0]); err == nil {
			if a, ok := hdr["alg"].(string); ok {
				preCheckAlg = a
			}
		}
	}
	if preCheckAlg != "" && jwtSigningMethod(preCheckAlg) != nil {
		if _, err := parsePublicKeyForAlg(secret, preCheckAlg); err != nil {
			panic(vm.NewGoError(fmt.Errorf("jwt.validate: %w", err)))
		}
	}

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods(validMethods),
	}
	if aud := optString(optsMap, "audience", ""); aud != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(aud))
	}
	if iss := optString(optsMap, "issuer", ""); iss != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(iss))
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		// The keyfunc is called after the header is parsed but
		// before the signature is checked. t.Method.Alg() is the
		// canonical algorithm string from the token's header; use
		// it to pick the right key shape.
		actualAlg := t.Method.Alg()
		return parsePublicKeyForAlg(secret, actualAlg)
	}, parserOpts...)
	if err != nil {
		return vm.ToValue(map[string]any{
			"valid":  false,
			"reason": err.Error(),
		})
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return vm.ToValue(map[string]any{
			"valid":  false,
			"reason": "claims have unexpected shape",
		})
	}
	return vm.ToValue(map[string]any{
		"valid":  true,
		"claims": map[string]any(claims),
	})
}

// optAlgorithm reads an optional `algorithm` field from opts,
// falling back to `fallback` when missing. The algorithm name is
// upper-cased so callers can write either `"hs256"` or `"HS256"` —
// with one exception: EdDSA's canonical jwt-go identifier is mixed
// case, so we restore that after upper-casing.
func optAlgorithm(opts goja.Value, fallback string) string {
	m := asMap(opts)
	if m == nil {
		return fallback
	}
	v, ok := m["algorithm"].(string)
	if !ok || v == "" {
		return fallback
	}
	upper := strings.ToUpper(v)
	if upper == "EDDSA" {
		return "EdDSA"
	}
	return upper
}

// asMap unwraps a goja.Value into a Go map. nil / undefined / null
// / non-object all collapse to nil.
func asMap(v goja.Value) map[string]any {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	m, _ := v.Export().(map[string]any)
	return m
}
