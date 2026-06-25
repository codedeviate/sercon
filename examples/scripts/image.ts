// Demonstrates the `image` global — decode, chainable transforms, encode.
// Offline + self-contained (embeds a tiny PNG); runs in make demo.

// A real 16x16 PNG (RGB), 89 bytes.
const PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,16,0,0,0,16,8,2,0,0,0,144,145,104,54,0,0,0,32,73,68,65,84,120,156,98,97,96,104,16,96,96,32,30,177,128,8,82,192,168,134,81,13,67,71,3,32,0,0,255,255,39,58,2,161,212,168,74,89,0,0,0,0,73,69,78,68,174,66,96,130]);

const im = image.decode(PNG);
runtime.log("decoded:", im.width + "x" + im.height, im.format);

const out = im.resize(8, 0).grayscale().blur(0.5);
runtime.assert.equal(out.width, 8, "resized width");
runtime.assert.equal(out.height, 8, "aspect-preserved height");

const png = out.bytes("png");
runtime.assert.equal(png[0], 137, "PNG magic");
runtime.assert.ok(png.length > 8, "encoded PNG bytes");

// Re-encode to WebP (lossless via nativewebp) — guard in case the encoder is
// unavailable in this build.
try {
  const webp = im.thumbnail(12, 12).bytes("webp");
  runtime.assert.ok(webp.length > 12, "encoded WebP bytes");
  runtime.log("webp bytes:", webp.length);
} catch (e) {
  runtime.log("webp encode unavailable — skipping:", String(e).slice(0, 60));
}

// save → re-open round-trip via a temp file.
const tmp = (runtime.env.get("TMPDIR") ?? "/tmp") + "/sercon-image-demo.png";
im.crop(2, 2, 10, 10).save(tmp);
const reread = image.open(tmp);
runtime.assert.equal(reread.width, 10, "cropped+saved width");

// Orientation: apply an EXIF orientation to pixels, and auto-orient on load.
const oriented = image.decode(PNG).orient(6); // 90° CW: 16×16 → 16×16 (square)
runtime.log("oriented:", oriented.width + "x" + oriented.height);
runtime.assert.equal(oriented.width, 16, "orient(6) width");
runtime.assert.equal(oriented.height, 16, "orient(6) height");
// autoOrient reads the source's EXIF Orientation (no-op when absent):
const up = image.decode(PNG, { autoOrient: true });
runtime.log("auto-oriented:", up.width + "x" + up.height);
runtime.assert.equal(up.width, 16, "autoOrient width (no-op, no EXIF in PNG)");

runtime.log("image demo OK");
