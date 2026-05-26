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
api.log("=== unsupported algorithm = clean throw ===");
try {
  api.jwt.sign({ sub: "x" }, secret, { algorithm: "RS256" });
} catch (e) {
  api.log("caught:", String(e).slice(0, 90) + "…");
}
