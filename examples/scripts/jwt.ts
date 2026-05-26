// Demonstrates api.jwt.* — HMAC-only sign / view / validate.
// Pure stdlib (golang-jwt/jwt/v5) — no network, runs in CI.

const secret = "topsecret-only-shared-with-the-verifier";

api.log("=== sign + view + validate (HS256 default) ===");
const tok = api.jwt.sign(
  {
    sub: "alice",
    iat: Math.floor(Date.now() / 1000),
    exp: Math.floor(Date.now() / 1000) + 3600,
    aud: "sercon-demo",
  },
  secret,
);
api.log("token (truncated):", tok.slice(0, 40) + "…", "len:", tok.length);

const view = api.jwt.view(tok);
api.log("header:", JSON.stringify(view.header));
api.log("payload sub:", view.payload.sub);
api.log("payload aud:", view.payload.aud);

const verdict = api.jwt.validate(tok, secret);
api.log("valid:", verdict.valid, "claims.sub:", verdict.claims.sub);

api.log("");
api.log("=== HS384 / HS512 also supported ===");
for (const alg of ["HS384", "HS512"]) {
  const t = api.jwt.sign({ sub: "x" }, secret, { algorithm: alg });
  api.log(alg, "alg header:", api.jwt.view(t).header.alg);
}

api.log("");
api.log("=== validate fails as data (not as a throw) ===");
const badSig = api.jwt.validate(tok, "wrong-secret");
api.log("wrong secret:", badSig.valid, "->", badSig.reason);

const expired = api.jwt.sign({ sub: "x", exp: 1 }, secret); // exp = 1970
const old = api.jwt.validate(expired, secret);
api.log("expired:    ", old.valid, "->", old.reason);

const mismatched = api.jwt.validate(tok, secret, { audience: "OTHER" });
api.log("wrong aud:  ", mismatched.valid, "->", mismatched.reason);

api.log("");
api.log("=== asymmetric (Ed25519) — PEM keys, validate with public ===");
// Test fixture only — NEVER reuse these keys for anything real.
// Generated fresh per demo with `openssl genpkey -algorithm ed25519`.
const privPEM = `-----BEGIN PRIVATE KEY-----
MC4CAQAwBQYDK2VwBCIEIAA9V2mRKgGRIXo7xJqiEQyqUOIXecmRbDY+xUpeOY6R
-----END PRIVATE KEY-----`;
const pubPEM = `-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAmIx3ECV1fjAJEHcN8SFXe3zwqb3Qk5W3X2wBrfT+eck=
-----END PUBLIC KEY-----`;

const edTok = api.jwt.sign({ sub: "bob", iss: "ed-issuer" }, privPEM, {
  algorithm: "EdDSA",
});
api.log("EdDSA alg in header:", api.jwt.view(edTok).header.alg);

const edVerdict = api.jwt.validate(edTok, pubPEM, { algorithm: "EdDSA" });
api.log("EdDSA valid:", edVerdict.valid, "sub:", edVerdict.claims?.sub);

// RS*/PS*/ES* work the same way — supply a matching PEM private key on
// sign and the corresponding public key on validate. PEM detection looks
// for the literal `-----BEGIN` prefix; plain HMAC bytes are passed through
// untouched.

api.log("");
api.log("=== PEM with HMAC algo = clean cross-check throw ===");
try {
  api.jwt.sign({ sub: "x" }, privPEM); // default HS256 + PEM → bug
} catch (e) {
  api.log("caught:", String(e).slice(0, 90) + "…");
}

api.log("");
api.log("=== unsupported algorithm = clean throw ===");
try {
  api.jwt.sign({ sub: "x" }, secret, { algorithm: "none" });
} catch (e) {
  api.log("caught:", String(e).slice(0, 90) + "…");
}
