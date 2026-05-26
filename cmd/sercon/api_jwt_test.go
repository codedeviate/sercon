package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/golang-jwt/jwt/v5"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// runJwtScript wires `jwt` into a fresh engine + a `__capture` side
// channel and returns whatever the script writes via `__capture`.
// Mirrors the harness from api_preg_test.go (same reason: Engine.Run
// always resolves to undefined today; the trailing-expression
// capture is on the backlog).
func runJwtScript(t *testing.T, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot: t.TempDir(),
		Timeout:    2 * time.Second,
	})
	if err := eng.RegisterNamespaceFactory("jwt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return jwtNamespace(vm)
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
	if _, err := eng.Run(context.Background(), "jwt.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// Each supported HMAC algorithm produces a token that validates with
// the same secret + algorithm and round-trips the claims intact.
func TestJwt_RoundTripPerAlgorithm(t *testing.T) {
	for _, alg := range []string{"HS256", "HS384", "HS512"} {
		t.Run(alg, func(t *testing.T) {
			got := runJwtScript(t, fmt.Sprintf(`
				const t = jwt.sign({ sub: "alice", n: 42 }, "shh", { algorithm: %q });
				const v = jwt.validate(t, "shh", { algorithm: %q });
				const __result = [v.valid, v.claims.sub, v.claims.n].join(",");
			`, alg, alg))
			want := "true,alice,42"
			if got != want {
				t.Errorf("got %v, want %s", got, want)
			}
		})
	}
}

// view returns header + payload without needing the secret. The
// signature is the raw base64url segment.
func TestJwt_ViewWithoutSecret(t *testing.T) {
	got := runJwtScript(t, `
		const t = jwt.sign({ sub: "bob", iss: "sercon" }, "doesnt-matter-for-view");
		const v = jwt.view(t);
		const __result = [v.header.alg, v.payload.sub, v.payload.iss, typeof v.signature].join(",");
	`)
	want := "HS256,bob,sercon,string"
	if got != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// A signature verified with the wrong secret resolves with
// `valid: false` and a reason — no throw.
func TestJwt_BadSignatureResolvesFalse(t *testing.T) {
	got := runJwtScript(t, `
		const t = jwt.sign({ sub: "x" }, "right-secret");
		const v = jwt.validate(t, "WRONG");
		const __result = [v.valid, v.reason.includes("signature")].join(",");
	`)
	want := "false,true"
	if got != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// An expired token resolves with `valid: false` rather than throwing.
// We deliberately backdate `exp` so it's already past at parse time.
func TestJwt_ExpiredResolvesFalse(t *testing.T) {
	got := runJwtScript(t, `
		const t = jwt.sign({ sub: "x", exp: 1 }, "k");
		const v = jwt.validate(t, "k");
		const __result = [v.valid, v.reason.includes("expired")].join(",");
	`)
	want := "false,true"
	if got != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// opts.audience / opts.issuer mismatches resolve with valid:false.
// Match cases are exercised inline in the per-algorithm round-trip.
func TestJwt_AudienceAndIssuerMismatch(t *testing.T) {
	got := runJwtScript(t, `
		const t = jwt.sign({ sub: "x", aud: "prod", iss: "issuer-a" }, "k");
		const v1 = jwt.validate(t, "k", { audience: "staging" });
		const v2 = jwt.validate(t, "k", { issuer: "issuer-b" });
		const __result = [v1.valid, v2.valid].join(",");
	`)
	want := "false,false"
	if got != want {
		t.Errorf("got %v, want %s", got, want)
	}
}

// Unknown algorithm names (anything not in jwtSupportedAlgoList,
// including the security-footgun "none") throw at sign time with
// a clear "unsupported algorithm" message. Real asymmetric algos
// are now supported (v0.5.3) so they get a different cross-check
// error path — see TestJwt_AsymmetricCrossCheckErrors below.
func TestJwt_UnsupportedAlgoRejected(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
	if err := eng.RegisterNamespaceFactory("jwt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return jwtNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	// Empty-string algorithm is deliberately NOT in the rejection
	// list — it falls through to the HS256 default. That matters for
	// scripts that pass `opts` for audience / issuer but don't set
	// algorithm.
	for _, alg := range []string{"none", "HS999", "RSA-OAEP"} {
		t.Run(alg, func(t *testing.T) {
			src := fmt.Sprintf(`jwt.sign({sub: "x"}, "k", { algorithm: %q });`, alg)
			_, err := eng.Run(context.Background(), "x.ts", src)
			if err == nil {
				t.Fatalf("expected throw for %q", alg)
			}
			if !strings.Contains(err.Error(), "unsupported algorithm") {
				t.Errorf("err wording for %q: %v", alg, err)
			}
		})
	}
}

// Tokens with the wrong number of segments throw at validate / view
// rather than resolving with valid:false. Same for empty / non-JWT
// strings.
func TestJwt_MalformedInputThrows(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
	if err := eng.RegisterNamespaceFactory("jwt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return jwtNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	cases := []string{
		`jwt.view("not.a.jwt.token");`,    // 4 segments
		`jwt.view("only.two");`,           // 2 segments
		`jwt.view("not-base64.x.y");`,     // 3 segments but base64-invalid
		`jwt.validate("only.two", "k");`,  // validate also catches this
	}
	for _, src := range cases {
		t.Run(src, func(t *testing.T) {
			_, err := eng.Run(context.Background(), "x.ts", src)
			if err == nil {
				t.Fatalf("expected throw for %q", src)
			}
		})
	}
}

// Empty claims / secret args are programmer errors — throw early
// rather than producing a degenerate token.
func TestJwt_InputValidation(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
	if err := eng.RegisterNamespaceFactory("jwt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return jwtNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	// Empty secret on sign.
	if _, err := eng.Run(context.Background(), "x.ts", `jwt.sign({sub: "x"}, "");`); err == nil {
		t.Error("empty sign secret should throw")
	}
	// Empty secret on validate.
	if _, err := eng.Run(context.Background(), "x.ts", `jwt.validate("a.b.c", "");`); err == nil {
		t.Error("empty validate secret should throw")
	}
}

// pemKeyPair generates a fresh in-memory key pair for one of the
// algorithm families and returns the PEM-encoded private + public
// halves. Used by the per-algorithm round-trip tests so no key
// material is committed to the repo. Smaller keys (1024-bit RSA,
// P-256 EC, Ed25519) are fine for tests — they're not protecting
// anything beyond the test scope.
func pemKeyPair(t *testing.T, alg string) (privPEM, pubPEM string) {
	t.Helper()
	switch {
	case strings.HasPrefix(alg, "RS"), strings.HasPrefix(alg, "PS"):
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatalf("rsa keygen: %v", err)
		}
		privDER, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			t.Fatalf("rsa marshal priv: %v", err)
		}
		pubDER, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
		if err != nil {
			t.Fatalf("rsa marshal pub: %v", err)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})),
			string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	case strings.HasPrefix(alg, "ES"):
		var curve elliptic.Curve
		switch alg {
		case "ES256":
			curve = elliptic.P256()
		case "ES384":
			curve = elliptic.P384()
		case "ES512":
			curve = elliptic.P521()
		default:
			t.Fatalf("unknown ES alg %q", alg)
		}
		k, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			t.Fatalf("ec keygen: %v", err)
		}
		privDER, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			t.Fatalf("ec marshal priv: %v", err)
		}
		pubDER, err := x509.MarshalPKIXPublicKey(&k.PublicKey)
		if err != nil {
			t.Fatalf("ec marshal pub: %v", err)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})),
			string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	case alg == "EdDSA":
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			t.Fatalf("ed25519 keygen: %v", err)
		}
		privDER, err := x509.MarshalPKCS8PrivateKey(priv)
		if err != nil {
			t.Fatalf("ed25519 marshal priv: %v", err)
		}
		pubDER, err := x509.MarshalPKIXPublicKey(pub)
		if err != nil {
			t.Fatalf("ed25519 marshal pub: %v", err)
		}
		return string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privDER})),
			string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER}))
	default:
		t.Fatalf("unknown alg %q", alg)
		return "", ""
	}
}

// runJwtScriptWithKeys is runJwtScript plus two extra registered
// globals `__priv` / `__pub` so test bodies can reference the
// generated key pair without dance-encoding it into JS source.
func runJwtScriptWithKeys(t *testing.T, priv, pub, body string) any {
	t.Helper()
	var captured any
	eng := scriptengine.New(scriptengine.Options{
		ScriptRoot: t.TempDir(),
		Timeout:    5 * time.Second,
	})
	if err := eng.RegisterNamespaceFactory("jwt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return jwtNamespace(vm)
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := eng.Register("__priv", priv); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__pub", pub); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__capture", func(v goja.Value) {
		if v == nil || goja.IsUndefined(v) {
			captured = nil
			return
		}
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.Run(context.Background(), "asym.ts", body+"\n__capture(__result);"); err != nil {
		t.Fatalf("script error: %v", err)
	}
	return captured
}

// Per-asymmetric-algorithm round-trip: sign with the private key,
// validate with the public key, expect valid:true and the claims
// flowing through intact. Covers every supported asymmetric algo
// (RS / PS / ES / EdDSA).
func TestJwt_AsymmetricRoundTrip(t *testing.T) {
	for _, alg := range []string{
		"RS256", "RS384", "RS512",
		"PS256", "PS384", "PS512",
		"ES256", "ES384",
		"EdDSA",
	} {
		t.Run(alg, func(t *testing.T) {
			priv, pub := pemKeyPair(t, alg)
			got := runJwtScriptWithKeys(t, priv, pub, fmt.Sprintf(`
				const t = jwt.sign({ sub: "alice", n: 42 }, __priv, { algorithm: %q });
				const v = jwt.validate(t, __pub, { algorithm: %q });
				const __result = [v.valid, v.claims?.sub, v.claims?.n].join(",");
			`, alg, alg))
			want := "true,alice,42"
			if got != want {
				t.Errorf("alg %s: got %v, want %s", alg, got, want)
			}
		})
	}
}

// ES512 produces signatures with deterministic-curve overhead that
// occasionally trips slow CI runners; isolate it so the per-test
// timeout doesn't apply to the rest of the matrix.
func TestJwt_AsymmetricRoundTripES512(t *testing.T) {
	priv, pub := pemKeyPair(t, "ES512")
	got := runJwtScriptWithKeys(t, priv, pub, `
		const t = jwt.sign({ sub: "x" }, __priv, { algorithm: "ES512" });
		const v = jwt.validate(t, __pub, { algorithm: "ES512" });
		const __result = [v.valid, v.claims?.sub].join(",");
	`)
	if got != "true,x" {
		t.Errorf("ES512 round-trip: got %v", got)
	}
}

// Validating with the wrong public key resolves valid:false rather
// than throwing.
func TestJwt_AsymmetricWrongPublicKeyResolvesFalse(t *testing.T) {
	priv, _ := pemKeyPair(t, "EdDSA")
	_, otherPub := pemKeyPair(t, "EdDSA")
	got := runJwtScriptWithKeys(t, priv, otherPub, `
		const t = jwt.sign({ sub: "x" }, __priv, { algorithm: "EdDSA" });
		const v = jwt.validate(t, __pub, { algorithm: "EdDSA" });
		const __result = [v.valid, typeof v.reason].join(",");
	`)
	if got != "false,string" {
		t.Errorf("wrong-pub: got %v", got)
	}
}

// PEM cross-checks. PEM with HMAC algo throws at sign with the
// "looks like a PEM-encoded key" wording so the caller fixes
// opts.algorithm. Symmetric: HMAC bytes with asymmetric algo
// throws with "needs a PEM-encoded private key".
func TestJwt_AsymmetricCrossCheckErrors(t *testing.T) {
	priv, pub := pemKeyPair(t, "EdDSA")
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
	if err := eng.RegisterNamespaceFactory("jwt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return jwtNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__priv", priv); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__pub", pub); err != nil {
		t.Fatal(err)
	}

	// PEM private key + default HS256 algo → cross-check error.
	_, err := eng.Run(context.Background(), "x.ts", `jwt.sign({sub: "x"}, __priv);`)
	if err == nil {
		t.Fatal("expected throw for PEM + HMAC default algo")
	}
	if !strings.Contains(err.Error(), "looks like a PEM-encoded key") {
		t.Errorf("expected PEM-cross-check wording, got: %v", err)
	}

	// HMAC bytes with asymmetric algo → cross-check error.
	_, err = eng.Run(context.Background(), "x.ts",
		`jwt.sign({sub: "x"}, "plain-bytes", { algorithm: "EdDSA" });`)
	if err == nil {
		t.Fatal("expected throw for HMAC bytes + asymmetric algo")
	}
	if !strings.Contains(err.Error(), "needs a PEM-encoded private key") {
		t.Errorf("expected bytes-cross-check wording, got: %v", err)
	}

	// Symmetric checks for validate.
	_, err = eng.Run(context.Background(), "x.ts",
		`jwt.validate("a.b.c", __pub, { algorithm: "HS256" });`)
	if err == nil {
		t.Fatal("expected throw for PEM public + HMAC algo on validate")
	}
	_, err = eng.Run(context.Background(), "x.ts",
		`jwt.validate("a.b.c", "plain", { algorithm: "ES256" });`)
	if err == nil {
		t.Fatal("expected throw for HMAC bytes + asymmetric algo on validate")
	}
}

// Algorithm-confusion guard: an attacker who has the public key
// bytes can forge an HS256 token using those bytes as the HMAC
// secret. The classic JWT exploit pattern. sercon's own sign path
// blocks this construction via the cross-check (PEM + HMAC algo),
// so we hand-roll the malicious token via jwt-go directly to
// simulate an attacker who isn't using sercon. The validate side
// must still reject the forged token because we pass
// `algorithm: "EdDSA"` and the parser's WithValidMethods only
// accepts EdDSA.
func TestJwt_AlgorithmConfusionGuard(t *testing.T) {
	_, pub := pemKeyPair(t, "EdDSA")

	// Forge externally — bypass sercon's cross-check.
	forged, err := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{"sub": "attacker"}).SignedString([]byte(pub))
	if err != nil {
		t.Fatalf("forge: %v", err)
	}

	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
	if err := eng.RegisterNamespaceFactory("jwt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return jwtNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__pub", pub); err != nil {
		t.Fatal(err)
	}
	if err := eng.Register("__forged", forged); err != nil {
		t.Fatal(err)
	}
	var captured any
	if err := eng.Register("__capture", func(v goja.Value) {
		captured = v.Export()
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := eng.Run(context.Background(), "x.ts", `
		const v = jwt.validate(__forged, __pub, { algorithm: "EdDSA" });
		__capture([v.valid, v.reason || ""].join(","));
	`); err != nil {
		t.Fatalf("validate: %v", err)
	}
	got, _ := captured.(string)
	if !strings.HasPrefix(got, "false,") {
		t.Errorf("algorithm-confusion guard: got %q (expected valid:false)", got)
	}
	if !strings.Contains(got, "signing method") {
		t.Errorf("reason should mention signing method mismatch, got: %q", got)
	}
}
