// Demonstrates crypto.hash.* — nine algorithms, all returning lowercase hex.
// Inputs are interpreted as UTF-8 byte sequences.

runtime.log("=== crypto.hash.* ===");
runtime.log("md5:     ", crypto.hash.md5("abc"));
runtime.log("sha1:    ", crypto.hash.sha1("abc"));
runtime.log("sha256:  ", crypto.hash.sha256("abc"));
runtime.log("sha384:  ", crypto.hash.sha384("abc"));
runtime.log("sha512:  ", crypto.hash.sha512("abc"));
runtime.log("sha3_256:", crypto.hash.sha3_256("abc"));
runtime.log("sha3_512:", crypto.hash.sha3_512("abc"));
runtime.log("blake3:  ", crypto.hash.blake3("abc"));
runtime.log("crc32:   ", crypto.hash.crc32("abc"));

// Known-vector sanity check.
runtime.assert.equal(
  crypto.hash.sha256("abc"),
  "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
);
