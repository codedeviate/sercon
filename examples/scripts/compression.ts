// Demonstrates api.format.compression.* — compress/decompress across nine pure-Go
// algorithms. Both functions return ArrayBuffer (which `new Uint8Array(...)`
// wraps for iteration); both accept strings (interpreted as UTF-8) or
// ArrayBuffer / Uint8Array inputs.

const payload = "the quick brown fox jumps over the lazy dog. ".repeat(20);
api.runtime.log("source length:", payload.length, "bytes");
api.runtime.log("supported:    ", api.format.compression.algos().join(", "));
api.runtime.log("");

for (const algo of api.format.compression.algos()) {
  const compressed = await api.format.compression.compress(algo, payload);
  const u8 = new Uint8Array(compressed);
  const back = await api.format.compression.decompress(algo, compressed);
  const backStr = Array.from(new Uint8Array(back)).map(b => String.fromCharCode(b)).join("");

  const ratio = (u8.length / payload.length * 100).toFixed(1);
  const ok = backStr === payload ? "✓" : "✗";
  api.runtime.log(`  ${algo.padEnd(8)} ${ok}  ${u8.length.toString().padStart(4)} bytes  (${ratio}%)`);
  if (backStr !== payload) throw new Error("round-trip mismatch for " + algo);
}
