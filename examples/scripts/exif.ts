// Demonstrates image.exif — read/write/replace/clear EXIF metadata in-memory.
// Self-contained: builds a JPEG from a tiny embedded PNG; no files or network needed.

// 16×16 RGB PNG (89 bytes) — same minimal image used in image.ts
const PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,16,0,0,0,16,8,2,0,0,0,144,145,104,54,0,0,0,32,73,68,65,84,120,156,98,97,96,104,16,96,96,32,30,177,128,8,82,192,168,134,81,13,67,71,3,32,0,0,255,255,39,58,2,161,212,168,74,89,0,0,0,0,73,69,78,68,174,66,96,130]);

// Encode to JPEG so we have a write-capable format.
const jpegBytes = image.decode(PNG).bytes("jpeg");

// 1. exif.read on a fresh JPEG (no EXIF yet) → {}
const empty = image.exif.read(jpegBytes);
runtime.assert.ok(typeof empty === "object", "read returns object");
runtime.log("fresh JPEG EXIF:", JSON.stringify(empty));

// 2. exif.replace — set a complete EXIF block
const out1 = image.exif.replace(jpegBytes, {
  image: { Make: "sercon", Model: "demo" },
});
runtime.assert.ok(out1.bytes != null, "replace returns bytes");
runtime.assert.ok(out1.format === "jpeg", "replace format is jpeg");
const e1 = image.exif.read(out1.bytes);
runtime.assert.equal(e1.image?.Make, "sercon", "Make after replace");
runtime.assert.equal(e1.image?.Model, "demo", "Model after replace");
runtime.log("after replace:", JSON.stringify(e1.image));

// 3. exif.write — merge: add Artist, null-delete Model, keep Make untouched.
//    This is what distinguishes write (merge) from replace (whole block).
const out2 = image.exif.write(out1.bytes, { image: { Artist: "Alice", Model: null } });
const e2 = image.exif.read(out2.bytes);
runtime.assert.equal(e2.image?.Make, "sercon", "Make preserved after write");
runtime.assert.equal(e2.image?.Artist, "Alice", "Artist added by write");
runtime.assert.ok(e2.image?.Model === undefined, "write null-deletes Model");
runtime.log("after write:", JSON.stringify(e2.image));

// 4. exif.clear — strip all EXIF
const out3 = image.exif.clear(out2.bytes);
runtime.assert.ok(out3.bytes != null, "clear returns bytes");
const e3 = image.exif.read(out3.bytes);
const hasKeys = Object.keys(e3).length > 0;
runtime.assert.ok(!hasKeys, "EXIF cleared (empty object)");
runtime.log("after clear:", JSON.stringify(e3));

runtime.log("exif demo OK");
