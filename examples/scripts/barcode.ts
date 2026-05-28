// Demonstrates api.format.barcode.encode — every supported symbology, default
// sizing. The output is a PNG payload (ArrayBuffer). Scripts typically
// either write it to disk via a custom binding or base64-encode it for
// embedding; here we just report the byte count.

api.runtime.log("supported formats:", api.format.barcode.formats().join(", "));
api.runtime.log("");

// Format-appropriate payloads. EAN/UPC need numeric content of a specific
// length; everything else takes free-form text.
const samples: Record<string, string> = {
  qr:         "https://github.com/codedeviate/sercon",
  datamatrix: "sercon-test",
  aztec:      "sercon-test",
  pdf417:     "sercon-test",
  code128:    "Sercon-128",
  code39:     "SERCON-39",
  codabar:    "A123456A",
  ean13:      "5901234123457",
  ean8:       "12345670",
  upca:       "012345678905",
};

for (const fmt of api.format.barcode.formats()) {
  const payload = samples[fmt];
  const png = new Uint8Array(await api.format.barcode.encode(fmt, payload));
  const sigHex = Array.from(png.slice(0, 8)).map(b => b.toString(16).padStart(2, "0")).join("");
  const isPng = sigHex === "89504e470d0a1a0a";
  const tag = isPng ? "✓ PNG" : "✗ not a PNG";
  api.runtime.log(`  ${fmt.padEnd(11)} ${tag}  ${png.length.toString().padStart(5)} bytes  payload=${JSON.stringify(payload)}`);
}

// Custom dimensions:
const big = new Uint8Array(await api.format.barcode.encode("qr", "sercon", { width: 512, height: 512 }));
api.runtime.log("");
api.runtime.log("explicit 512x512 QR:", big.length, "bytes");

// === decode (the inverse) ===
api.runtime.log("");
api.runtime.log("decode formats:", api.format.barcode.decodableFormats().join(", "));
api.runtime.log("");

// Round-trip a QR. Decoded text matches the encoded text byte-for-byte.
const qrPNG = await api.format.barcode.encode("qr", "round-trip via gozxing");
const rt = await api.format.barcode.decode(qrPNG);              // auto-detect path
api.runtime.log(`auto-detect:  ${rt.format} -> ${rt.text}`);

// Explicit format hint is faster (skips the per-reader fallback walk) and
// surfaces a clear "wrong format" error if the input doesn't match.
const c128 = await api.format.barcode.encode("code128", "Sercon-128");
const hinted = await api.format.barcode.decode(c128, "code128");
api.runtime.log(`format hint:  ${hinted.format} -> ${hinted.text}`);

// 1D quirks worth knowing:
//   - Code 39 decoded text appends the Mod-43 checksum char (G for HELLO-39).
//   - Codabar strips the A...A start/stop wrappers on decode.
//   - EAN/UPC need a quiet zone (white margin) which boombuler's encoder
//     doesn't add — sercon-encoded EAN PNGs won't round-trip without
//     post-processing. Real-world EAN scanners want that margin too.
const c39 = await api.format.barcode.decode(await api.format.barcode.encode("code39", "HELLO-39"), "code39");
api.runtime.log(`code39 quirk: ${c39.text}   (input was "HELLO-39"; G is the Mod-43 checksum)`);

// opts.quietZone fixes the EAN/UPC round-trip: it pads the bars with a
// white margin (the spec-required clear zone) so the decoder can lock on.
api.runtime.log("");
api.runtime.log("=== opts.quietZone makes EAN/UPC round-trip ===");
const eanRaw = await api.format.barcode.encode("ean13", "5901234123457");
const eanPad = await api.format.barcode.encode("ean13", "5901234123457", { quietZone: true });
try {
  await api.format.barcode.decode(eanRaw, "ean13");
  api.runtime.log("raw EAN decoded (unexpected on this platform)");
} catch {
  api.runtime.log("raw EAN (no quiet zone):  decode fails — no clear margin");
}
const eanDecoded = await api.format.barcode.decode(eanPad, "ean13");
api.runtime.log("padded EAN (quietZone):   decode ->", eanDecoded.text);
