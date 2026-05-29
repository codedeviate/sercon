// Demonstrates crypto.jwt.* — HMAC-only sign / view / validate.
// Pure stdlib (golang-jwt/jwt/v5) — no network, runs in CI.

const secret = "topsecret-only-shared-with-the-verifier";

runtime.log("=== sign + view + validate (HS256 default) ===");
const tok = crypto.jwt.sign(
  {
    sub: "alice",
    iat: Math.floor(Date.now() / 1000),
    exp: Math.floor(Date.now() / 1000) + 3600,
    aud: "sercon-demo",
  },
  secret,
);
runtime.log("token (truncated):", tok.slice(0, 40) + "…", "len:", tok.length);

const view = crypto.jwt.view(tok);
runtime.log("header:", JSON.stringify(view.header));
runtime.log("payload sub:", view.payload.sub);
runtime.log("payload aud:", view.payload.aud);

const verdict = crypto.jwt.validate(tok, secret);
runtime.log("valid:", verdict.valid, "claims.sub:", verdict.claims.sub);

runtime.log("");
runtime.log("=== HS384 / HS512 also supported ===");
for (const alg of ["HS384", "HS512"]) {
  const t = crypto.jwt.sign({ sub: "x" }, secret, { algorithm: alg });
  runtime.log(alg, "alg header:", crypto.jwt.view(t).header.alg);
}

runtime.log("");
runtime.log("=== validate fails as data (not as a throw) ===");
const badSig = crypto.jwt.validate(tok, "wrong-secret");
runtime.log("wrong secret:", badSig.valid, "->", badSig.reason);

const expired = crypto.jwt.sign({ sub: "x", exp: 1 }, secret); // exp = 1970
const old = crypto.jwt.validate(expired, secret);
runtime.log("expired:    ", old.valid, "->", old.reason);

const mismatched = crypto.jwt.validate(tok, secret, { audience: "OTHER" });
runtime.log("wrong aud:  ", mismatched.valid, "->", mismatched.reason);

runtime.log("");
runtime.log("=== asymmetric (Ed25519) — PEM keys, validate with public ===");
// Test fixture only — NEVER reuse these keys for anything real.
// Generated fresh per demo with `openssl genpkey -algorithm ed25519`.
const privPEM = `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIAA9V2mRKgGRIXo7xJqiEQyqUOIXecmRbDY+xUpeOY6R
-----END PRIVATE KEY-----`;
const pubPEM = `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAmIx3ECV1fjAJEHcN8SFXe3zwqb3Qk5W3X2wBrfT+eck=
-----END PUBLIC KEY-----`;

const edTok = crypto.jwt.sign({ sub: "bob", iss: "ed-issuer" }, privPEM, {
  algorithm: "EdDSA",
});
runtime.log("EdDSA alg in header:", crypto.jwt.view(edTok).header.alg);

const edVerdict = crypto.jwt.validate(edTok, pubPEM, { algorithm: "EdDSA" });
runtime.log("EdDSA valid:", edVerdict.valid, "sub:", edVerdict.claims?.sub);

// RS*/PS*/ES* work the same way — supply a matching PEM private key on
// sign and the corresponding public key on validate. PEM detection looks
// for the literal `-----BEGIN` prefix; plain HMAC bytes are passed through
// untouched.

runtime.log("");
runtime.log("=== PEM with HMAC algo = clean cross-check throw ===");
try {
  crypto.jwt.sign({ sub: "x" }, privPEM); // default HS256 + PEM → bug
} catch (e) {
  runtime.log("caught:", String(e).slice(0, 90) + "…");
}

runtime.log("");
runtime.log("=== unsupported algorithm = clean throw ===");
try {
  crypto.jwt.sign({ sub: "x" }, secret, { algorithm: "none" });
} catch (e) {
  runtime.log("caught:", String(e).slice(0, 90) + "…");
}

runtime.log("");
runtime.log("=== JWK key shape (JSON Web Key) ===");
// secret can also be a JWK JSON object — handy when keys come from a
// JWKS endpoint or a config file in JWK form. The `kty` field picks the
// key type; the algorithm just has to match. This Ed25519 keypair is a
// test fixture (NEVER reuse for anything real).
const jwkPriv = '{"crv":"Ed25519","d":"gd2QdqfiWS0cn6D12OyCLzpLPgs25hlpYvuf_OCqLY0","kty":"OKP","x":"40sJMJtKz5ozPQNymkG1MF2B3SQ7pp65WNLrcmYVowg"}';
const jwkPub  = '{"crv":"Ed25519","kty":"OKP","x":"40sJMJtKz5ozPQNymkG1MF2B3SQ7pp65WNLrcmYVowg"}';
const jwkTok = crypto.jwt.sign({ sub: "carol" }, jwkPriv, { algorithm: "EdDSA" });
const jwkVerdict = crypto.jwt.validate(jwkTok, jwkPub, { algorithm: "EdDSA" });
runtime.log("signed + validated via JWK:", jwkVerdict.valid, "sub:", jwkVerdict.claims?.sub);
