// Demonstrates api.encrypt.* — age X25519 keygen + encrypt + decrypt.
// Pure stdlib + filippo.io/age (pure-Go); runs in CI.

api.log("=== keygen ===");
const alice = api.encrypt.keygen();
const bob = api.encrypt.keygen();
api.log("alice pub: ", alice.publicKey.slice(0, 16) + "…  (share with senders)");
api.log("alice priv:", alice.privateKey.slice(0, 16) + "…  (keep secret)");

api.log("");
api.log("=== single-recipient round-trip ===");
const ct = api.encrypt.encrypt("hello, alice", alice.publicKey);
api.log("ciphertext:", ct.length, "bytes");

const plain = api.encrypt.decrypt(ct, alice.privateKey);
api.log("plaintext: ", await api.text.decode(plain, "utf-8"));

api.log("");
api.log("=== multi-recipient: any listed identity decrypts ===");
const shared = api.encrypt.encrypt("shared message", [alice.publicKey, bob.publicKey]);
api.log("alice reads:", await api.text.decode(api.encrypt.decrypt(shared, alice.privateKey), "utf-8"));
api.log("bob reads:  ", await api.text.decode(api.encrypt.decrypt(shared, bob.privateKey), "utf-8"));

api.log("");
api.log("=== wrong identity = clean throw ===");
const eve = api.encrypt.keygen();
try {
  api.encrypt.decrypt(ct, eve.privateKey);
} catch (e) {
  api.log("caught:", String(e).slice(0, 90) + "…");
}

api.log("");
api.log("=== public-as-identity cross-check throws ===");
try {
  api.encrypt.decrypt(ct, alice.publicKey); // public where private was expected
} catch (e) {
  api.log("caught:", String(e).slice(0, 90) + "…");
}

api.log("");
api.log("=== binary payloads work too ===");
const binary = new Uint8Array([0, 1, 2, 3, 255, 128, 64]);
const sealed = api.encrypt.encrypt(binary, alice.publicKey);
const opened = api.encrypt.decrypt(sealed, alice.privateKey);
api.log("opened bytes:", Array.from(opened).join(","));

api.log("");
api.log("=== armored output for embedding in JSON / YAML / email ===");
// opts.armored wraps the binary age stream in age's ASCII armor banner.
// Same `decrypt` call reads both forms — auto-detected from the leading
// bytes (`-----BEGIN AGE ENCRYPTED FILE-----`).
const armored = api.encrypt.encrypt(
  "embed me in a JSON field",
  alice.publicKey,
  { armored: true },
);
const head = Array.from(armored).slice(0, 35).map((b) => String.fromCharCode(b)).join("");
api.log("banner:    ", head);
api.log("length:    ", armored.length, "bytes (vs binary's smaller form)");
api.log("decoded:   ", await api.text.decode(api.encrypt.decrypt(armored, alice.privateKey), "utf-8"));

// Armored ciphertext as a JS string (the natural shape after pasting from
// JSON / email) round-trips too — jsArgToBytes converts the string to
// bytes, then armor detection kicks in.
const asString = await api.text.decode(armored, "utf-8");
api.log("re-decoded:", await api.text.decode(api.encrypt.decrypt(asString, alice.privateKey), "utf-8"));

api.log("");
api.log("=== rekey: rotate recipients without exposing plaintext ===");
// "alice's secret" was encrypted for alice. Rotate it to bob without ever
// seeing the plaintext in JS-land. The format defaults to whatever the
// input was (binary→binary here); opts.armored overrides.
const charlie = api.encrypt.keygen();
const originalForAlice = api.encrypt.encrypt("rotate me to charlie", alice.publicKey);
const nowForCharlie = api.encrypt.rekey(
  originalForAlice,
  alice.privateKey,
  charlie.publicKey,
);
api.log("charlie reads:", await api.text.decode(api.encrypt.decrypt(nowForCharlie, charlie.privateKey), "utf-8"));

// alice is locked out of the rekeyed payload — the recipient set actually
// changed, not just expanded.
try {
  api.encrypt.decrypt(nowForCharlie, alice.privateKey);
  api.log("✗ alice should be locked out");
} catch (e) {
  api.log("alice locked out:", String(e).slice(0, 70) + "…");
}

api.log("");
api.log("=== detectBackend: classify recipient / identity strings ===");
// Classifier — useful when a script reads recipient strings from a
// config file and needs to know whether to call api.encrypt.* or
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
  const c = api.encrypt.detectBackend(input);
  api.log(label.padEnd(14), "->", c.backend, c.kind ? "(" + c.kind + ")" : "");
}
