package main

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

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

// Asymmetric algorithm names (RS256 / ES256 / EdDSA) must throw with
// a clear message rather than silently fall through — those land in
// a later cut.
func TestJwt_AsymmetricAlgoRejected(t *testing.T) {
	eng := scriptengine.New(scriptengine.Options{ScriptRoot: t.TempDir()})
	if err := eng.RegisterNamespaceFactory("jwt", func(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
		return jwtNamespace(vm)
	}); err != nil {
		t.Fatal(err)
	}
	for _, alg := range []string{"RS256", "ES256", "EdDSA", "none"} {
		t.Run(alg, func(t *testing.T) {
			_, err := eng.Run(context.Background(), "x.ts",
				fmt.Sprintf(`jwt.sign({sub: "x"}, "k", { algorithm: %q });`, alg))
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
