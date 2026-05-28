// Demonstrates api.crypto.hash.* — nine algorithms, all returning lowercase hex.
// Inputs are interpreted as UTF-8 byte sequences.

api.runtime.log("=== api.crypto.hash.* ===");
api.runtime.log("md5:     ", api.crypto.hash.md5("abc"));
api.runtime.log("sha1:    ", api.crypto.hash.sha1("abc"));
api.runtime.log("sha256:  ", api.crypto.hash.sha256("abc"));
api.runtime.log("sha384:  ", api.crypto.hash.sha384("abc"));
api.runtime.log("sha512:  ", api.crypto.hash.sha512("abc"));
api.runtime.log("sha3_256:", api.crypto.hash.sha3_256("abc"));
api.runtime.log("sha3_512:", api.crypto.hash.sha3_512("abc"));
api.runtime.log("blake3:  ", api.crypto.hash.blake3("abc"));
api.runtime.log("crc32:   ", api.crypto.hash.crc32("abc"));

// Known-vector sanity check.
api.runtime.assert.equal(
  api.crypto.hash.sha256("abc"),
  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
);
