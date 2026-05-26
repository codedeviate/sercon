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
