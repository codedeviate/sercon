// Demonstrates api.crypto.jwt.* — HMAC-only sign / view / validate.
// Pure stdlib (golang-jwt/jwt/v5) — no network, runs in CI.

const secret = "topsecret-only-shared-with-the-verifier";

api.runtime.log("=== sign + view + validate (HS256 default) ===");
const tok = api.crypto.jwt.sign(
  {
    sub: "alice",
    iat: Math.floor(Date.now() / 1000),
    exp: Math.floor(Date.now() / 1000) + 3600,
    aud: "sercon-demo",
  },
  secret,
);
api.runtime.log("token (truncated):", tok.slice(0, 40) + "…", "len:", tok.length);

const view = api.crypto.jwt.view(tok);
api.runtime.log("header:", JSON.stringify(view.header));
api.runtime.log("payload sub:", view.payload.sub);
api.runtime.log("payload aud:", view.payload.aud);

const verdict = api.crypto.jwt.validate(tok, secret);
api.runtime.log("valid:", verdict.valid, "claims.sub:", verdict.claims.sub);

api.runtime.log("");
api.runtime.log("=== HS384 / HS512 also supported ===");
for (const alg of ["HS384", "HS512"]) {
  const t = api.crypto.jwt.sign({ sub: "x" }, secret, { algorithm: alg });
  api.runtime.log(alg, "alg header:", api.crypto.jwt.view(t).header.alg);
}

api.runtime.log("");
api.runtime.log("=== validate fails as data (not as a throw) ===");
const badSig = api.crypto.jwt.validate(tok, "wrong-secret");
api.runtime.log("wrong secret:", badSig.valid, "->", badSig.reason);

const expired = api.crypto.jwt.sign({ sub: "x", exp: 1 }, secret); // exp = 1970
const old = api.crypto.jwt.validate(expired, secret);
api.runtime.log("expired:    ", old.valid, "->", old.reason);

const mismatched = api.crypto.jwt.validate(tok, secret, { audience: "OTHER" });
api.runtime.log("wrong aud:  ", mismatched.valid, "->", mismatched.reason);

api.runtime.log("");
api.runtime.log("=== asymmetric (Ed25519) — PEM keys, validate with public ===");
// Test fixture only — NEVER reuse these keys for anything real.
// Generated fresh per demo with `openssl genpkey -algorithm ed25519`.
const privPEM = `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIAA9V2mRKgGRIXo7xJqiEQyqUOIXecmRbDY+xUpeOY6R
-----END PRIVATE KEY-----`;
const pubPEM = `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAmIx3ECV1fjAJEHcN8SFXe3zwqb3Qk5W3X2wBrfT+eck=
-----END PUBLIC KEY-----`;

const edTok = api.crypto.jwt.sign({ sub: "bob", iss: "ed-issuer" }, privPEM, {
  algorithm: "EdDSA",
});
api.runtime.log("EdDSA alg in header:", api.crypto.jwt.view(edTok).header.alg);

const edVerdict = api.crypto.jwt.validate(edTok, pubPEM, { algorithm: "EdDSA" });
api.runtime.log("EdDSA valid:", edVerdict.valid, "sub:", edVerdict.claims?.sub);

// RS*/PS*/ES* work the same way — supply a matching PEM private key on
// sign and the corresponding public key on validate. PEM detection looks
// for the literal `-----BEGIN` prefix; plain HMAC bytes are passed through
// untouched.

api.runtime.log("");
api.runtime.log("=== PEM with HMAC algo = clean cross-check throw ===");
try {
  api.crypto.jwt.sign({ sub: "x" }, privPEM); // default HS256 + PEM → bug
} catch (e) {
  api.runtime.log("caught:", String(e).slice(0, 90) + "…");
}

api.runtime.log("");
api.runtime.log("=== unsupported algorithm = clean throw ===");
try {
  api.crypto.jwt.sign({ sub: "x" }, secret, { algorithm: "none" });
} catch (e) {
  api.runtime.log("caught:", String(e).slice(0, 90) + "…");
}

api.runtime.log("");
api.runtime.log("=== JWK key shape (JSON Web Key) ===");
// secret can also be a JWK JSON object — handy when keys come from a
// JWKS endpoint or a config file in JWK form. The `kty` field picks the
// key type; the algorithm just has to match. This Ed25519 keypair is a
// test fixture (NEVER reuse for anything real).
const jwkPriv = '{"crv":"Ed25519","d":"gd2QdqfiWS0cn6D12OyCLzpLPgs25hlpYvuf_OCqLY0","kty":"OKP","x":"40sJMJtKz5ozPQNymkG1MF2B3SQ7pp65WNLrcmYVowg"}';
const jwkPub  = '{"crv":"Ed25519","kty":"OKP","x":"40sJMJtKz5ozPQNymkG1MF2B3SQ7pp65WNLrcmYVowg"}';
const jwkTok = api.crypto.jwt.sign({ sub: "carol" }, jwkPriv, { algorithm: "EdDSA" });
const jwkVerdict = api.crypto.jwt.validate(jwkTok, jwkPub, { algorithm: "EdDSA" });
api.runtime.log("signed + validated via JWK:", jwkVerdict.valid, "sub:", jwkVerdict.claims?.sub);
