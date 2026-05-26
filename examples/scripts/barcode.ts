// Demonstrates api.barcode.encode — every supported symbology, default
// sizing. The output is a PNG payload (ArrayBuffer). Scripts typically
// either write it to disk via a custom binding or base64-encode it for
// embedding; here we just report the byte count.

api.log("supported formats:", api.barcode.formats().join(", "));
api.log("");

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

for (const fmt of api.barcode.formats()) {
  const payload = samples[fmt];
  const png = new Uint8Array(await api.barcode.encode(fmt, payload));
  const sigHex = Array.from(png.slice(0, 8)).map(b => b.toString(16).padStart(2, "0")).join("");
  const isPng = sigHex === "89504e470d0a1a0a";
  const tag = isPng ? "✓ PNG" : "✗ not a PNG";
  api.log(`  ${fmt.padEnd(11)} ${tag}  ${png.length.toString().padStart(5)} bytes  payload=${JSON.stringify(payload)}`);
}

// Custom dimensions:
const big = new Uint8Array(await api.barcode.encode("qr", "sercon", { width: 512, height: 512 }));
api.log("");
api.log("explicit 512x512 QR:", big.length, "bytes");

// === decode (the inverse) ===
api.log("");
api.log("decode formats:", api.barcode.decodableFormats().join(", "));
api.log("");

// Round-trip a QR. Decoded text matches the encoded text byte-for-byte.
const qrPNG = await api.barcode.encode("qr", "round-trip via gozxing");
const rt = await api.barcode.decode(qrPNG);              // auto-detect path
api.log(`auto-detect:  ${rt.format} -> ${rt.text}`);

// Explicit format hint is faster (skips the per-reader fallback walk) and
// surfaces a clear "wrong format" error if the input doesn't match.
const c128 = await api.barcode.encode("code128", "Sercon-128");
const hinted = await api.barcode.decode(c128, "code128");
api.log(`format hint:  ${hinted.format} -> ${hinted.text}`);

// 1D quirks worth knowing:
//   - Code 39 decoded text appends the Mod-43 checksum char (G for HELLO-39).
//   - Codabar strips the A...A start/stop wrappers on decode.
//   - EAN/UPC need a quiet zone (white margin) which boombuler's encoder
//     doesn't add — sercon-encoded EAN PNGs won't round-trip without
//     post-processing. Real-world EAN scanners want that margin too.
const c39 = await api.barcode.decode(await api.barcode.encode("code39", "HELLO-39"), "code39");
api.log(`code39 quirk: ${c39.text}   (input was "HELLO-39"; G is the Mod-43 checksum)`);
