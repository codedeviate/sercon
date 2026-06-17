// Demonstrates runtime.clipboard — host OS system clipboard text I/O.
// Self-skips (exit 0) when no clipboard backend is on PATH (e.g. headless CI),
// so it's safe in `make demo`.

if (!runtime.clipboard.available) {
  runtime.log("no clipboard backend on PATH — skipping clipboard demo.");
} else {
  const sample = "sercon clipboard demo " + Date.now();
  await runtime.clipboard.write(sample);
  const got = await runtime.clipboard.read();
  runtime.assert.equal(got, sample, "clipboard round-trip");
  runtime.log("clipboard round-trip OK:", JSON.stringify(got));
}

// Image (PNG) round-trip — self-skips unless an image backend is present
// (macOS needs pngpaste; Linux needs wl-clipboard or xclip; Windows PowerShell).
// NB: a real 16x16 PNG, not a 1x1 — the macOS clipboard re-encodes via
// CoreGraphics, which fails to finalize a PNG from a degenerate 1x1 image.
// readImage returns a re-encoded PNG (its byte length may differ from the
// input), so assert the PNG signature survives rather than byte-equality.
if (runtime.clipboard.imageAvailable) {
  const PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,16,0,0,0,16,8,2,0,0,0,144,145,104,54,0,0,0,32,73,68,65,84,120,156,98,97,96,104,16,96,96,32,30,177,128,8,82,192,168,134,81,13,67,71,3,32,0,0,255,255,39,58,2,161,212,168,74,89,0,0,0,0,73,69,78,68,174,66,96,130]);
  await runtime.clipboard.writeImage(PNG);
  const back = await runtime.clipboard.readImage();
  runtime.assert.ok(back && back.length >= 8, "readImage returned PNG bytes");
  runtime.assert.equal(back[0], 137, "PNG magic byte 0");
  runtime.assert.equal(back[1], 80, "PNG magic byte 1 ('P')");
  runtime.log("clipboard image round-trip OK:", back.length, "bytes");
} else {
  runtime.log("clipboard image backend unavailable — skipping image round-trip.");
}
