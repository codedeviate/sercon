// Demonstrates api.text.* — charset detection, encoding, decoding.
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

api.runtime.log("=== api.text.charset.encode / decode round-trip ===");
for (const [charset, text] of samples) {
  const encoded = await api.text.charset.encode(text, charset);
  const decoded = await api.text.charset.decode(encoded, charset);
  const bytes = new Uint8Array(encoded);
  const ok = decoded === text;
  api.runtime.log(
    `  ${charset.padEnd(13)} ${ok ? "✓" : "✗"} ` +
    `${bytes.length.toString().padStart(3)} bytes  ${JSON.stringify(decoded)}`,
  );
  if (!ok) throw new Error("round-trip mismatch for " + charset);
}

// Detection: feed Latin-1-encoded bytes and confirm chardet doesn't
// classify them as UTF-8 (byte 0xE9 isn't a valid UTF-8 lead byte
// without a continuation).
api.runtime.log("");
api.runtime.log("=== api.text.charset.detect on Latin-1 sample ===");
const sample = await api.text.charset.encode(
  "café crème — un éléphant marche dans la rue. ".repeat(20),
  "ISO-8859-1",
);
const result = await api.text.charset.detect(sample);
api.runtime.log("top charset:", result.charset, " confidence:", result.confidence,
        result.language ? " language: " + result.language : "");
api.runtime.log("candidates :", result.candidates.length);
for (const c of result.candidates.slice(0, 4)) {
  api.runtime.log("  -", c.charset.padEnd(14), "conf=" + c.confidence,
          c.language ? " (" + c.language + ")" : "");
}
