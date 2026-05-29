// Demonstrates text.* — charset detection, encoding, decoding.
// All three are Promise-returning bindings backed by saintfish/chardet
// (detection) and golang.org/x/text/encoding (round-tripping).

// Round-trip a few representative charsets to prove encode/decode are
// inverses of each other and that goja's UTF-8 -> bytes -> charset ->
// bytes -> UTF-8 path works end-to-end.
const samples: Array<[string, string]> = [
  ["UTF-8",        "hello world 1234 — UTF-8"],
  ["ISO-8859-1",   "café crème — 1985"],
  ["Windows-1252", "smart “quotes” and €5"],
  ["Shift_JIS",    "こんにちは"],
  ["GBK",          "你好"],
];

runtime.log("=== text.charset.encode / decode round-trip ===");
for (const [charset, value] of samples) {
  const encoded = await text.charset.encode(value, charset);
  const decoded = await text.charset.decode(encoded, charset);
  const bytes = new Uint8Array(encoded);
  const ok = decoded === value;
  runtime.log(
    `  ${charset.padEnd(13)} ${ok ? "✓" : "✗"} ` +
    `${bytes.length.toString().padStart(3)} bytes  ${JSON.stringify(decoded)}`,
  );
  if (!ok) throw new Error("round-trip mismatch for " + charset);
}

// Detection: feed Latin-1-encoded bytes and confirm chardet doesn't
// classify them as UTF-8 (byte 0xE9 isn't a valid UTF-8 lead byte
// without a continuation).
runtime.log("");
runtime.log("=== text.charset.detect on Latin-1 sample ===");
const sample = await text.charset.encode(
  "café crème — un éléphant marche dans la rue. ".repeat(20),
  "ISO-8859-1",
);
const result = await text.charset.detect(sample);
runtime.log("top charset:", result.charset, " confidence:", result.confidence,
        result.language ? " language: " + result.language : "");
runtime.log("candidates :", result.candidates.length);
for (const c of result.candidates.slice(0, 4)) {
  runtime.log("  -", c.charset.padEnd(14), "conf=" + c.confidence,
          c.language ? " (" + c.language + ")" : "");
}
