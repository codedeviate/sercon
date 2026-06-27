// Demonstrates image.stego — hide and recover a payload in a PNG (with a password).
// Offline + self-contained (embeds a tiny PNG carrier); runs in make demo.

// A real 16x16 PNG (RGB), 89 bytes.
const PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,16,0,0,0,16,8,2,0,0,0,144,145,104,54,0,0,0,32,73,68,65,84,120,156,98,97,96,104,16,96,96,32,30,177,128,8,82,192,168,134,81,13,67,71,3,32,0,0,255,255,39,58,2,161,212,168,74,89,0,0,0,0,73,69,78,68,174,66,96,130]);

const room = image.stego.capacity(PNG).bytes;
runtime.assert.ok(room > 0, "carrier has capacity");

const secret = "meet at noon";
const out = image.stego.embed(PNG, secret, { password: "s3cret" });
const back = image.stego.extract(out.bytes, { password: "s3cret" });
runtime.assert.equal(back, secret, "stego round-trip recovers the secret");

runtime.log("stego OK:", room, "bytes capacity; recovered:", back);
