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
