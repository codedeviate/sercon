// Demonstrates codec.compression.* — compress/decompress across nine pure-Go
// algorithms. Both functions return ArrayBuffer (which `new Uint8Array(...)`
// wraps for iteration); both accept strings (interpreted as UTF-8) or
// ArrayBuffer / Uint8Array inputs.

const payload = "the quick brown fox jumps over the lazy dog. ".repeat(20);
runtime.log("source length:", payload.length, "bytes");
runtime.log("supported:    ", codec.compression.algos().join(", "));
runtime.log("");

for (const algo of codec.compression.algos()) {
  const compressed = await codec.compression.compress(algo, payload);
  const u8 = new Uint8Array(compressed);
  const back = await codec.compression.decompress(algo, compressed);
  const backStr = Array.from(new Uint8Array(back)).map(b => String.fromCharCode(b)).join("");

  const ratio = (u8.length / payload.length * 100).toFixed(1);
  const ok = backStr === payload ? "✓" : "✗";
  runtime.log(`  ${algo.padEnd(8)} ${ok}  ${u8.length.toString().padStart(4)} bytes  (${ratio}%)`);
  if (backStr !== payload) throw new Error("round-trip mismatch for " + algo);
}
