// Regenerates the binary + dump corpus deterministically (no Math.random).
// Run via `make sample-data`. Text/CSV/TSV/JSON/TOML/XML files are authored by
// hand and committed; this script (re)writes the rest.

// runtime.argv[1] is the absolute path to this script.
const dir = fs.path.dirname(runtime.argv[1]);
const out = (name: string) => dir + "/" + name;

// --- products.xlsx (2 sheets, typed cells) ---
const wb = {
  sheets: [
    { name: "Products", rows: [
      ["sku", "name", "price", "stock"],
      ["A-100", "Widget", 19.99, 120],
      ["A-200", "Gadget", 29.5, 8],
      ["A-300", "Gizmo", 9.95, 0],
      ["A-400", "Sprocket", 4.25, 55],
    ] },
    { name: "Categories", rows: [
      ["sku", "category"],
      ["A-100", "Hardware"],
      ["A-200", "Hardware"],
      ["A-300", "Accessory"],
      ["A-400", "Hardware"],
    ] },
  ],
};
await fs.writeBytes(out("products.xlsx"), codec.sheet.write(wb, { format: "xlsx" }).bytes);

// --- images: deterministic gradients (all < 256 KB) ---
// A real 16x16 RGB PNG (89 bytes) — same minimal seed used in image.ts.
const SEED_PNG = new Uint8Array([137,80,78,71,13,10,26,10,0,0,0,13,73,72,68,82,0,0,0,16,0,0,0,16,8,2,0,0,0,144,145,104,54,0,0,0,32,73,68,65,84,120,156,98,97,96,104,16,96,96,32,30,177,128,8,82,192,168,134,81,13,67,71,3,32,0,0,255,255,39,58,2,161,212,168,74,89,0,0,0,0,73,69,78,68,174,66,96,130]);

// Resize the 16x16 seed to produce each target dimension.
// smooth upscale → repeating solid pixels → excellent PNG compression ratio
await fs.writeBytes(out("small.png"),  image.decode(SEED_PNG).resize(64, 64).bytes("png"));
await fs.writeBytes(out("medium.png"), image.decode(SEED_PNG).resize(400, 300).bytes("png"));
await fs.writeBytes(out("large.png"),  image.decode(SEED_PNG).resize(800, 600).bytes("png"));
await fs.writeBytes(out("photo.jpg"),  image.decode(SEED_PNG).resize(400, 300).bytes("jpeg"));

// --- barcode.png (Code128 of a known string) ---
// codec.barcode.encode(format, text, opts?) — format is first arg, text is second.
await fs.writeBytes(out("barcode.png"), new Uint8Array(await codec.barcode.encode("code128", "SERCON-12345")));

// --- tagged.jpg (small JPEG carrying EXIF) ---
// image.exif.write(bytes, tags) returns { bytes, format } — extract .bytes.
const baseJpeg = image.decode(SEED_PNG).resize(320, 240).bytes("jpeg");
const exifResult = image.exif.write(baseJpeg, { image: { Make: "sercon", Model: "sample" } });
await fs.writeBytes(out("tagged.jpg"), exifResult.bytes);

// --- php / perl dumps of a small structure ---
const sample = { id: 7, name: "demo", tags: ["a", "b"] };
await fs.writeText(out("sample.phpdump"), codec.php.serialize(sample));
await fs.writeText(out("sample.perldump"), codec.perl.dumper(sample));

runtime.log("regenerated sample-data binaries in", dir);
