// Demonstrates codec.barcode.encode — every supported symbology, default
// sizing. The output is a PNG payload (ArrayBuffer). Scripts typically
// either write it to disk via a custom binding or base64-encode it for
// embedding; here we just report the byte count.

runtime.log("supported formats:", codec.barcode.formats().join(", "));
runtime.log("");

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

for (const fmt of codec.barcode.formats()) {
  const payload = samples[fmt];
  const png = new Uint8Array(await codec.barcode.encode(fmt, payload));
  const sigHex = Array.from(png.slice(0, 8)).map(b => b.toString(16).padStart(2, "0")).join("");
  const isPng = sigHex === "89504e470d0a1a0a";
  const tag = isPng ? "✓ PNG" : "✗ not a PNG";
  runtime.log(`  ${fmt.padEnd(11)} ${tag}  ${png.length.toString().padStart(5)} bytes  payload=${JSON.stringify(payload)}`);
}

// Custom dimensions:
const big = new Uint8Array(await codec.barcode.encode("qr", "sercon", { width: 512, height: 512 }));
runtime.log("");
runtime.log("explicit 512x512 QR:", big.length, "bytes");

// === decode (the inverse) ===
runtime.log("");
runtime.log("decode formats:", codec.barcode.decodableFormats().join(", "));
runtime.log("");

// Round-trip a QR. Decoded text matches the encoded text byte-for-byte.
const qrPNG = await codec.barcode.encode("qr", "round-trip via gozxing");
const rt = await codec.barcode.decode(qrPNG);              // auto-detect path
runtime.log(`auto-detect:  ${rt.format} -> ${rt.text}`);

// Explicit format hint is faster (skips the per-reader fallback walk) and
// surfaces a clear "wrong format" error if the input doesn't match.
const c128 = await codec.barcode.encode("code128", "Sercon-128");
const hinted = await codec.barcode.decode(c128, "code128");
runtime.log(`format hint:  ${hinted.format} -> ${hinted.text}`);

// 1D quirks worth knowing:
//   - Code 39 decoded text appends the Mod-43 checksum char (G for HELLO-39).
//   - Codabar strips the A...A start/stop wrappers on decode.
//   - EAN/UPC need a quiet zone (white margin) which boombuler's encoder
//     doesn't add — sercon-encoded EAN PNGs won't round-trip without
//     post-processing. Real-world EAN scanners want that margin too.
const c39 = await codec.barcode.decode(await codec.barcode.encode("code39", "HELLO-39"), "code39");
runtime.log(`code39 quirk: ${c39.text}   (input was "HELLO-39"; G is the Mod-43 checksum)`);

// opts.quietZone fixes the EAN/UPC round-trip: it pads the bars with a
// white margin (the spec-required clear zone) so the decoder can lock on.
runtime.log("");
runtime.log("=== opts.quietZone makes EAN/UPC round-trip ===");
const eanRaw = await codec.barcode.encode("ean13", "5901234123457");
const eanPad = await codec.barcode.encode("ean13", "5901234123457", { quietZone: true });
try {
  await codec.barcode.decode(eanRaw, "ean13");
  runtime.log("raw EAN decoded (unexpected on this platform)");
} catch {
  runtime.log("raw EAN (no quiet zone):  decode fails — no clear margin");
}
const eanDecoded = await codec.barcode.decode(eanPad, "ean13");
runtime.log("padded EAN (quietZone):   decode ->", eanDecoded.text);
