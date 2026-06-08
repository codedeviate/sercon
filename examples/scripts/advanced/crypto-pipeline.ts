// Advanced demo: "secure payload" pipeline composing crypto.hash, crypto.jwt,
// crypto.encrypt (age), and text.str base64 encoding into a complete
// sign-then-encrypt-then-transport round-trip.
//
// Forward pass  : build payload → sha256 hash → JWT signed with hash claim
//               → age-encrypt → base64 for transport.
// Reverse pass  : base64-decode → age-decrypt → re-hash → jwt.validate
//               → assert plaintext and hash claim both match.
//
// APIs used (all verified against the existing demo scripts):
//   crypto.hash.sha256        — hex digest
//   crypto.jwt.sign / .validate  — HS256
//   crypto.encrypt.keygen / .encrypt / .decrypt  — age X25519
//   text.str.base64Encode / .base64Decode        — string←→base64
//   text.charset.decode       — Uint8Array / ArrayBuffer → UTF-8 string

// ── 1. Build the plaintext payload ────────────────────────────────────────
runtime.log("=== step 1: build payload ===");
const payload = JSON.stringify({
  userId: "u-42",
  action: "transfer",
  amount: 1234.56,
  currency: "SEK",
  ts: Math.floor(Date.now() / 1000),
});
runtime.log("payload:", payload);

// ── 2. Hash the plaintext (integrity anchor) ───────────────────────────────
runtime.log("");
runtime.log("=== step 2: sha256 of payload ===");
const payloadHash = crypto.hash.sha256(payload);
runtime.log("sha256:", payloadHash);
runtime.assert.equal(payloadHash.length, 64, "sha256 hex should be 64 chars");

// ── 3. Issue a JWT carrying the hash as a custom claim ────────────────────
runtime.log("");
runtime.log("=== step 3: sign JWT (HS256) with hash claim ===");
const jwtSecret = "pipeline-hmac-secret-do-not-reuse";
const now = Math.floor(Date.now() / 1000);
const token = crypto.jwt.sign(
  {
    sub:  "u-42",
    iss:  "sercon-pipeline-demo",
    iat:  now,
    exp:  now + 3600,
    phash: payloadHash,   // bind the payload hash into the token
  },
  jwtSecret,
);
runtime.log("token (first 48 chars):", token.slice(0, 48) + "…");

const view = crypto.jwt.view(token);
runtime.log("alg:", view.header.alg, "  sub:", view.payload.sub);

// ── 4. age-encrypt the raw payload (not the JWT — the JWT is the manifest) ─
runtime.log("");
runtime.log("=== step 4: age-encrypt payload ===");
const recipientKeys = crypto.encrypt.keygen();
runtime.log("recipient pub (first 16):", recipientKeys.publicKey.slice(0, 16) + "…");

const ciphertext = crypto.encrypt.encrypt(payload, recipientKeys.publicKey);
runtime.log("ciphertext:", ciphertext.length, "bytes");

// ── 5. Base64-encode the ciphertext for "transport" (e.g. JSON body) ──────
runtime.log("");
runtime.log("=== step 5: base64-encode ciphertext ===");
// age ciphertext is a Uint8Array; convert to a plain string for base64Encode.
const ctStr = Array.from(ciphertext).map((b) => String.fromCharCode(b)).join("");
const b64Ciphertext = text.str.base64Encode(ctStr);
runtime.log("base64 length:", b64Ciphertext.length, "chars");

// ── REVERSE PASS ──────────────────────────────────────────────────────────

// ── 6. Base64-decode to recover the ciphertext bytes ─────────────────────
runtime.log("");
runtime.log("=== step 6: base64-decode ===");
const ctStrBack = text.str.base64Decode(b64Ciphertext);
// Reconstruct Uint8Array from the decoded binary string.
const ctBack = new Uint8Array(ctStrBack.length);
for (let i = 0; i < ctStrBack.length; i++) {
  ctBack[i] = ctStrBack.charCodeAt(i);
}
runtime.log("recovered ciphertext:", ctBack.length, "bytes");
runtime.assert.equal(ctBack.length, ciphertext.length, "ciphertext length preserved across base64 round-trip");

// ── 7. age-decrypt to recover the plaintext ──────────────────────────────
runtime.log("");
runtime.log("=== step 7: age-decrypt ===");
const decryptedBytes = crypto.encrypt.decrypt(ctBack, recipientKeys.privateKey);
const decryptedPayload = await text.charset.decode(decryptedBytes, "utf-8");
runtime.log("decrypted:", decryptedPayload);
runtime.assert.equal(decryptedPayload, payload, "decrypted payload must match original");

// ── 8. Re-hash and compare ────────────────────────────────────────────────
runtime.log("");
runtime.log("=== step 8: re-hash and compare ===");
const rehash = crypto.hash.sha256(decryptedPayload);
runtime.assert.equal(rehash, payloadHash, "hash of decrypted payload must match original hash");
runtime.log("hash matches:", rehash === payloadHash);

// ── 9. Validate the JWT and check the phash claim ────────────────────────
runtime.log("");
runtime.log("=== step 9: validate JWT + verify phash claim ===");
const verdict = crypto.jwt.validate(token, jwtSecret);
runtime.log("jwt valid:", verdict.valid);
runtime.assert.ok(verdict.valid, "JWT must be valid");
runtime.assert.equal(verdict.claims.phash, payloadHash, "phash claim must match computed hash");
runtime.log("phash claim verified:", verdict.claims.phash === payloadHash);

// ── 10. Negative checks ───────────────────────────────────────────────────
runtime.log("");
runtime.log("=== step 10: negative checks ===");

// Wrong HMAC secret
const badVerdict = crypto.jwt.validate(token, "wrong-secret");
runtime.assert.ok(!badVerdict.valid, "tampered HMAC must be rejected");
runtime.log("wrong-secret verdict:", badVerdict.valid, "->", badVerdict.reason);

// Wrong private key for decryption
const intruder = crypto.encrypt.keygen();
try {
  crypto.encrypt.decrypt(ciphertext, intruder.privateKey);
  runtime.log("✗ intruder should have been rejected");
} catch (e) {
  runtime.log("intruder decrypt caught:", String(e).slice(0, 70) + "…");
}

runtime.log("");
runtime.log("full pipeline PASS");
