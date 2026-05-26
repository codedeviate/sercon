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
