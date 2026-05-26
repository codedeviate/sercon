// Demonstrates api.hash.* — nine algorithms, all returning lowercase hex.
// Inputs are interpreted as UTF-8 byte sequences.

api.log("=== api.hash.* ===");
api.log("md5:     ", api.hash.md5("abc"));
api.log("sha1:    ", api.hash.sha1("abc"));
api.log("sha256:  ", api.hash.sha256("abc"));
api.log("sha384:  ", api.hash.sha384("abc"));
api.log("sha512:  ", api.hash.sha512("abc"));
api.log("sha3_256:", api.hash.sha3_256("abc"));
api.log("sha3_512:", api.hash.sha3_512("abc"));
api.log("blake3:  ", api.hash.blake3("abc"));
api.log("crc32:   ", api.hash.crc32("abc"));

// Known-vector sanity check.
api.assert.equal(
  api.hash.sha256("abc"),
  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
);
