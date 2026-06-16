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
if (runtime.clipboard.imageAvailable) {
  const PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,1,0,0,0,1,8,6,0,0,0,31,21,196,137,0,0,0,13,73,68,65,84,120,156,99,250,207,0,0,0,3,1,1,0,24,221,141,219,0,0,0,0,73,69,78,68,174,66,96,130]);
  await runtime.clipboard.writeImage(PNG);
  const back = await runtime.clipboard.readImage();
  runtime.assert.ok(back && back.length >= 8, "readImage returned PNG bytes");
  runtime.assert.equal(back[0], 137, "PNG magic byte 0");
  runtime.log("clipboard image round-trip OK:", back.length, "bytes");
} else {
  runtime.log("clipboard image backend unavailable — skipping image round-trip.");
}
