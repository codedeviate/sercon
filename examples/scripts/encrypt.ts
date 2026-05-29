// Demonstrates crypto.encrypt.* — age X25519 keygen + encrypt + decrypt.
// Pure stdlib + filippo.io/age (pure-Go); runs in CI.

runtime.log("=== keygen ===");
const alice = crypto.encrypt.keygen();
const bob = crypto.encrypt.keygen();
runtime.log("alice pub: ", alice.publicKey.slice(0, 16) + "…  (share with senders)");
runtime.log("alice priv:", alice.privateKey.slice(0, 16) + "…  (keep secret)");

runtime.log("");
runtime.log("=== single-recipient round-trip ===");
const ct = crypto.encrypt.encrypt("hello, alice", alice.publicKey);
runtime.log("ciphertext:", ct.length, "bytes");

const plain = crypto.encrypt.decrypt(ct, alice.privateKey);
runtime.log("plaintext: ", await text.charset.decode(plain, "utf-8"));

runtime.log("");
runtime.log("=== multi-recipient: any listed identity decrypts ===");
const shared = crypto.encrypt.encrypt("shared message", [alice.publicKey, bob.publicKey]);
runtime.log("alice reads:", await text.charset.decode(crypto.encrypt.decrypt(shared, alice.privateKey), "utf-8"));
runtime.log("bob reads:  ", await text.charset.decode(crypto.encrypt.decrypt(shared, bob.privateKey), "utf-8"));

runtime.log("");
runtime.log("=== wrong identity = clean throw ===");
const eve = crypto.encrypt.keygen();
try {
  crypto.encrypt.decrypt(ct, eve.privateKey);
} catch (e) {
  runtime.log("caught:", String(e).slice(0, 90) + "…");
}

runtime.log("");
runtime.log("=== public-as-identity cross-check throws ===");
try {
  crypto.encrypt.decrypt(ct, alice.publicKey); // public where private was expected
} catch (e) {
  runtime.log("caught:", String(e).slice(0, 90) + "…");
}

runtime.log("");
runtime.log("=== binary payloads work too ===");
const binary = new Uint8Array([0, 1, 2, 3, 255, 128, 64]);
const sealed = crypto.encrypt.encrypt(binary, alice.publicKey);
const opened = crypto.encrypt.decrypt(sealed, alice.privateKey);
runtime.log("opened bytes:", Array.from(opened).join(","));

runtime.log("");
runtime.log("=== armored output for embedding in JSON / YAML / email ===");
// opts.armored wraps the binary age stream in age's ASCII armor banner.
// Same `decrypt` call reads both forms — auto-detected from the leading
// bytes (`-----BEGIN AGE ENCRYPTED FILE-----`).
const armored = crypto.encrypt.encrypt(
  "embed me in a JSON field",
  alice.publicKey,
  { armored: true },
);
const head = Array.from(armored).slice(0, 35).map((b) => String.fromCharCode(b)).join("");
runtime.log("banner:    ", head);
runtime.log("length:    ", armored.length, "bytes (vs binary's smaller form)");
runtime.log("decoded:   ", await text.charset.decode(crypto.encrypt.decrypt(armored, alice.privateKey), "utf-8"));

// Armored ciphertext as a JS string (the natural shape after pasting from
// JSON / email) round-trips too — jsArgToBytes converts the string to
// bytes, then armor detection kicks in.
const asString = await text.charset.decode(armored, "utf-8");
runtime.log("re-decoded:", await text.charset.decode(crypto.encrypt.decrypt(asString, alice.privateKey), "utf-8"));

runtime.log("");
runtime.log("=== rekey: rotate recipients without exposing plaintext ===");
// "alice's secret" was encrypted for alice. Rotate it to bob without ever
// seeing the plaintext in JS-land. The format defaults to whatever the
// input was (binary→binary here); opts.armored overrides.
const charlie = crypto.encrypt.keygen();
const originalForAlice = crypto.encrypt.encrypt("rotate me to charlie", alice.publicKey);
const nowForCharlie = crypto.encrypt.rekey(
  originalForAlice,
  alice.privateKey,
  charlie.publicKey,
);
runtime.log("charlie reads:", await text.charset.decode(crypto.encrypt.decrypt(nowForCharlie, charlie.privateKey), "utf-8"));

// alice is locked out of the rekeyed payload — the recipient set actually
// changed, not just expanded.
try {
  crypto.encrypt.decrypt(nowForCharlie, alice.privateKey);
  runtime.log("✗ alice should be locked out");
} catch (e) {
  runtime.log("alice locked out:", String(e).slice(0, 70) + "…");
}

runtime.log("");
runtime.log("=== detectBackend: classify recipient / identity strings ===");
// Classifier — useful when a script reads recipient strings from a
// config file and needs to know whether to call crypto.encrypt.* or
// shell out to gpg / a different backend.
const samples: Array<[string, string]> = [
  ["age public",    alice.publicKey],
  ["age private",   alice.privateKey],
  ["ssh-rsa",       "ssh-rsa AAAAB3NzaC1yc2E..."],
  ["pgp public",    "-----BEGIN PGP PUBLIC KEY BLOCK-----\n..."],
  ["pgp private",   "-----BEGIN PGP PRIVATE KEY BLOCK-----\n..."],
  ["plain text",    "just some text"],
];
for (const [label, input] of samples) {
  const c = crypto.encrypt.detectBackend(input);
  runtime.log(label.padEnd(14), "->", c.backend, c.kind ? "(" + c.kind + ")" : "");
}

runtime.log("");
runtime.log("=== PGP backend (auto-dispatched from key format) ===");
// keygenPgp returns armored PGP key blocks. encrypt / decrypt route to
// the PGP path automatically when they see a PGP block — same API as age.
const pgp = crypto.encrypt.keygenPgp({ name: "Sercon Demo", email: "demo@example.com" });
runtime.log("key type:", crypto.encrypt.detectBackend(pgp.publicKey).backend);
const pgpCt = crypto.encrypt.encrypt("encrypted with PGP", pgp.publicKey);
runtime.log("ciphertext:", (await text.charset.decode(pgpCt, "utf-8")).split("\n")[0]);
runtime.log("decrypted:", await text.charset.decode(crypto.encrypt.decrypt(pgpCt, pgp.privateKey), "utf-8"));
