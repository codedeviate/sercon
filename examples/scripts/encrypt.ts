// Demonstrates api.crypto.encrypt.* — age X25519 keygen + encrypt + decrypt.
// Pure stdlib + filippo.io/age (pure-Go); runs in CI.

api.runtime.log("=== keygen ===");
const alice = api.crypto.encrypt.keygen();
const bob = api.crypto.encrypt.keygen();
api.runtime.log("alice pub: ", alice.publicKey.slice(0, 16) + "…  (share with senders)");
api.runtime.log("alice priv:", alice.privateKey.slice(0, 16) + "…  (keep secret)");

api.runtime.log("");
api.runtime.log("=== single-recipient round-trip ===");
const ct = api.crypto.encrypt.encrypt("hello, alice", alice.publicKey);
api.runtime.log("ciphertext:", ct.length, "bytes");

const plain = api.crypto.encrypt.decrypt(ct, alice.privateKey);
api.runtime.log("plaintext: ", await api.text.charset.decode(plain, "utf-8"));

api.runtime.log("");
api.runtime.log("=== multi-recipient: any listed identity decrypts ===");
const shared = api.crypto.encrypt.encrypt("shared message", [alice.publicKey, bob.publicKey]);
api.runtime.log("alice reads:", await api.text.charset.decode(api.crypto.encrypt.decrypt(shared, alice.privateKey), "utf-8"));
api.runtime.log("bob reads:  ", await api.text.charset.decode(api.crypto.encrypt.decrypt(shared, bob.privateKey), "utf-8"));

api.runtime.log("");
api.runtime.log("=== wrong identity = clean throw ===");
const eve = api.crypto.encrypt.keygen();
try {
  api.crypto.encrypt.decrypt(ct, eve.privateKey);
} catch (e) {
  api.runtime.log("caught:", String(e).slice(0, 90) + "…");
}

api.runtime.log("");
api.runtime.log("=== public-as-identity cross-check throws ===");
try {
  api.crypto.encrypt.decrypt(ct, alice.publicKey); // public where private was expected
} catch (e) {
  api.runtime.log("caught:", String(e).slice(0, 90) + "…");
}

api.runtime.log("");
api.runtime.log("=== binary payloads work too ===");
const binary = new Uint8Array([0, 1, 2, 3, 255, 128, 64]);
const sealed = api.crypto.encrypt.encrypt(binary, alice.publicKey);
const opened = api.crypto.encrypt.decrypt(sealed, alice.privateKey);
api.runtime.log("opened bytes:", Array.from(opened).join(","));

api.runtime.log("");
api.runtime.log("=== armored output for embedding in JSON / YAML / email ===");
// opts.armored wraps the binary age stream in age's ASCII armor banner.
// Same `decrypt` call reads both forms — auto-detected from the leading
// bytes (`-----BEGIN AGE ENCRYPTED FILE-----`).
const armored = api.crypto.encrypt.encrypt(
  "embed me in a JSON field",
  alice.publicKey,
  { armored: true },
);
const head = Array.from(armored).slice(0, 35).map((b) => String.fromCharCode(b)).join("");
api.runtime.log("banner:    ", head);
api.runtime.log("length:    ", armored.length, "bytes (vs binary's smaller form)");
api.runtime.log("decoded:   ", await api.text.charset.decode(api.crypto.encrypt.decrypt(armored, alice.privateKey), "utf-8"));

// Armored ciphertext as a JS string (the natural shape after pasting from
// JSON / email) round-trips too — jsArgToBytes converts the string to
// bytes, then armor detection kicks in.
const asString = await api.text.charset.decode(armored, "utf-8");
api.runtime.log("re-decoded:", await api.text.charset.decode(api.crypto.encrypt.decrypt(asString, alice.privateKey), "utf-8"));

api.runtime.log("");
api.runtime.log("=== rekey: rotate recipients without exposing plaintext ===");
// "alice's secret" was encrypted for alice. Rotate it to bob without ever
// seeing the plaintext in JS-land. The format defaults to whatever the
// input was (binary→binary here); opts.armored overrides.
const charlie = api.crypto.encrypt.keygen();
const originalForAlice = api.crypto.encrypt.encrypt("rotate me to charlie", alice.publicKey);
const nowForCharlie = api.crypto.encrypt.rekey(
  originalForAlice,
  alice.privateKey,
  charlie.publicKey,
);
api.runtime.log("charlie reads:", await api.text.charset.decode(api.crypto.encrypt.decrypt(nowForCharlie, charlie.privateKey), "utf-8"));

// alice is locked out of the rekeyed payload — the recipient set actually
// changed, not just expanded.
try {
  api.crypto.encrypt.decrypt(nowForCharlie, alice.privateKey);
  api.runtime.log("✗ alice should be locked out");
} catch (e) {
  api.runtime.log("alice locked out:", String(e).slice(0, 70) + "…");
}

api.runtime.log("");
api.runtime.log("=== detectBackend: classify recipient / identity strings ===");
// Classifier — useful when a script reads recipient strings from a
// config file and needs to know whether to call api.crypto.encrypt.* or
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
  const c = api.crypto.encrypt.detectBackend(input);
  api.runtime.log(label.padEnd(14), "->", c.backend, c.kind ? "(" + c.kind + ")" : "");
}

api.runtime.log("");
api.runtime.log("=== PGP backend (auto-dispatched from key format) ===");
// keygenPgp returns armored PGP key blocks. encrypt / decrypt route to
// the PGP path automatically when they see a PGP block — same API as age.
const pgp = api.crypto.encrypt.keygenPgp({ name: "Sercon Demo", email: "demo@example.com" });
api.runtime.log("key type:", api.crypto.encrypt.detectBackend(pgp.publicKey).backend);
const pgpCt = api.crypto.encrypt.encrypt("encrypted with PGP", pgp.publicKey);
api.runtime.log("ciphertext:", (await api.text.charset.decode(pgpCt, "utf-8")).split("\n")[0]);
api.runtime.log("decrypted:", await api.text.charset.decode(api.crypto.encrypt.decrypt(pgpCt, pgp.privateKey), "utf-8"));
