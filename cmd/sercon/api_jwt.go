package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/dop251/goja"
	"github.com/golang-jwt/jwt/v5"
)

// jwtNamespace wires `api.jwt.*`. v0.5.2 ships HMAC-only support
// (HS256 / HS384 / HS512); asymmetric algorithms (RSA / ECDSA / EdDSA)
// land in a follow-up cut once the key-shape mapping from JS is
// designed (PEM string? base64 raw bytes? jwk object?). All three
// members are synchronous — JWT work is pure CPU and doesn't need
// the event loop.
func jwtNamespace(vm *goja.Runtime) map[string]any {
	return map[string]any{
		"sign":     func(claims map[string]any, secret string, opts goja.Value) goja.Value { return jwtSign(vm, claims, secret, opts) },
		"view":     func(token string) goja.Value { return jwtView(vm, token) },
		"validate": func(token, secret string, opts goja.Value) goja.Value { return jwtValidate(vm, token, secret, opts) },
	}
}

// jwtSupportedAlgos lists the HMAC algorithms scripts can ask for via
// `opts.algorithm`. Anything else is rejected at sign / validate time
// with a named-algorithm error so scripts notice early — silently
// falling back would let production tokens drift onto a weaker algo
// than the caller asked for.
var jwtSupportedAlgos = map[string]*jwt.SigningMethodHMAC{
	"HS256": jwt.SigningMethodHS256,
	"HS384": jwt.SigningMethodHS384,
	"HS512": jwt.SigningMethodHS512,
}

// jwtSign produces a signed compact-serialisation JWT. `claims` is
// passed straight through to jwt-go's MapClaims, which knows how to
// emit RFC 7519 reserved claims (`exp`, `nbf`, `iat`, `aud`, `iss`,
// `sub`, `jti`) when present and handles arbitrary user-defined
// claims alongside them. Missing claims aren't synthesised — scripts
// that want `iat` set should add it explicitly via
// `api.time.nowMs() / 1000`.
func jwtSign(vm *goja.Runtime, claims map[string]any, secret string, opts goja.Value) goja.Value {
	if claims == nil {
		panic(vm.NewGoError(errors.New("jwt.sign: claims object required")))
	}
	if secret == "" {
		panic(vm.NewGoError(errors.New("jwt.sign: secret is empty")))
	}
	algName := optAlgorithm(opts, "HS256")
	method, ok := jwtSupportedAlgos[algName]
	if !ok {
		panic(vm.NewGoError(fmt.Errorf("jwt.sign: unsupported algorithm %q (HS256 / HS384 / HS512 are HMAC only; asymmetric algos land in a later cut)", algName)))
	}
	tok := jwt.NewWithClaims(method, jwt.MapClaims(claims))
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		panic(vm.NewGoError(fmt.Errorf("jwt.sign: %w", err)))
	}
	return vm.ToValue(signed)
}

// jwtView decodes a token's header + payload + signature WITHOUT
// verifying the signature. Useful for inspecting tokens — debugging
// auth flows, surfacing the `aud` / `iss` claim to the user — without
// trusting the contents. Malformed input throws.
//
// `header` and `payload` round-trip back as JS objects; `signature`
// is the raw base64url string (no padding), matching how jwt-go and
// most other inspection tools surface it. Scripts that want the
// signature as bytes should base64url-decode it themselves.
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
// the result into a generic map. Both `RawURLEncoding` (no padding)
// and the rarer padded form are accepted — strictly speaking the
// spec forbids padding but real-world tokens sometimes carry it.
func decodeJWTSegment(seg string) (map[string]any, error) {
	raw, err := base64.RawURLEncoding.DecodeString(seg)
	if err != nil {
		// Fall back to URLEncoding (with `=` padding) before giving up.
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

// jwtValidate verifies a token's signature using the supplied secret
// and the algorithm declared in the token's header. Standard-claim
// validation (`exp`, `nbf`, `iat`) is delegated to jwt-go. If
// `opts.audience` or `opts.issuer` is set, those are checked too;
// otherwise jwt-go skips them.
//
// **Resolves**, doesn't throw, on every validation failure:
//
//	{ valid: false, reason: "signature is invalid" }
//	{ valid: false, reason: "token is expired" }
//
// Scripts branch on `valid`. Malformed input (not three segments,
// invalid base64, invalid JSON) still throws — those aren't validation
// failures, they're structural input errors and a script that
// pattern-matches on `valid: false` shouldn't accidentally accept a
// garbage string.
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

	parserOpts := []jwt.ParserOption{
		jwt.WithValidMethods([]string{"HS256", "HS384", "HS512"}),
	}
	if aud := optString(asMap(opts), "audience", ""); aud != "" {
		parserOpts = append(parserOpts, jwt.WithAudience(aud))
	}
	if iss := optString(asMap(opts), "issuer", ""); iss != "" {
		parserOpts = append(parserOpts, jwt.WithIssuer(iss))
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		return []byte(secret), nil
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

// optAlgorithm reads an optional `algorithm` field out of the opts
// arg, falling back to `fallback` when missing. Lives here (not in
// api_net.go's optString) because the algorithm string is normalised
// to upper-case so callers can write either `"hs256"` or `"HS256"`.
func optAlgorithm(opts goja.Value, fallback string) string {
	m := asMap(opts)
	if m == nil {
		return fallback
	}
	v, ok := m["algorithm"].(string)
	if !ok || v == "" {
		return fallback
	}
	return strings.ToUpper(v)
}

// asMap unwraps a goja.Value into a Go map. Used by jwt.sign /
// jwt.validate to read their opts arg uniformly; nil / undefined /
// null / non-object all collapse to nil.
func asMap(v goja.Value) map[string]any {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return nil
	}
	m, _ := v.Export().(map[string]any)
	return m
}
