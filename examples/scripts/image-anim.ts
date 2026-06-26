// Demonstrates image.decodeFrames / image.encodeFrames (animated GIF + APNG).
// Self-contained: builds two frames from the embedded sample PNG, encodes a
// GIF and an APNG, then decodes each back and asserts the frame counts.
// Offline, no temp files — fully in-memory.

// A real 16×16 PNG (RGB), 89 bytes — same sample as image.ts.
const PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,16,0,0,0,16,8,2,0,0,0,144,145,104,54,0,0,0,32,73,68,65,84,120,156,98,97,96,104,16,96,96,32,30,177,128,8,82,192,168,134,81,13,67,71,3,32,0,0,255,255,39,58,2,161,212,168,74,89,0,0,0,0,73,69,78,68,174,66,96,130]);

// Decode the embedded PNG into a base frame.
const base = image.decode(PNG);
runtime.log("base frame:", base.width + "x" + base.height, base.format);

// Build a two-frame animation spec.
const spec = {
  width: base.width,
  height: base.height,
  loopCount: 0,
  frames: [
    { image: base,                 delayMs: 100 },
    { image: base.grayscale(),     delayMs: 200 },
  ],
};

// --- GIF round-trip ---
const gifOut = image.encodeFrames(spec, { format: "gif" });
runtime.assert.equal(gifOut.format, "gif", "encodeFrames gif format");
runtime.assert.ok(gifOut.bytes != null, "gif bytes present");
runtime.assert.ok(gifOut.bytes.length > 6, "gif bytes non-empty");

const gifIn = image.decodeFrames(gifOut.bytes);
runtime.assert.equal(gifIn.format, "gif", "decodeFrames gif format");
runtime.assert.equal(gifIn.frames.length, 2, "gif round-trip frame count");
runtime.log("gif:", gifIn.format, "frames=" + gifIn.frames.length, "delay0=" + gifIn.frames[0].delayMs + "ms");

// --- APNG round-trip ---
const apngOut = image.encodeFrames(spec, { format: "apng" });
runtime.assert.equal(apngOut.format, "apng", "encodeFrames apng format");
runtime.assert.ok(apngOut.bytes != null, "apng bytes present");
runtime.assert.ok(apngOut.bytes.length > 8, "apng bytes non-empty");

const apngIn = image.decodeFrames(apngOut.bytes);
runtime.assert.equal(apngIn.format, "apng", "decodeFrames apng format");
runtime.assert.equal(apngIn.frames.length, 2, "apng round-trip frame count");
runtime.log("apng:", apngIn.format, "frames=" + apngIn.frames.length, "delay0=" + apngIn.frames[0].delayMs + "ms");

// --- non-animated input → single frame ---
const single = image.decodeFrames(PNG);
runtime.assert.equal(single.frames.length, 1, "non-animated PNG → 1 frame");
runtime.log("single frame from PNG:", single.format, "frames=" + single.frames.length);

runtime.log("image-anim demo OK");
