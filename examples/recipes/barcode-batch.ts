// Recipe: generate a Code128 barcode per region from regions.tsv, save the PNGs
// to tmp, then decode one back to prove it scans.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

const book = codec.sheet.read(await fs.readBytes(data("regions.tsv")), { format: "tsv" });
const regions = book.sheets[0].rows.slice(1).map((r: any) => String(r[0]));
let first = "";
for (const region of regions) {
  const code = `REGION-${region.toUpperCase()}`;
  const png = await codec.barcode.encode("code128", code);
  const p = `${tmp}/sercon-barcode-${region}.png`;
  await fs.writeBytes(p, png);
  if (!first) first = p;
}
const decoded = await codec.barcode.decode(await fs.readBytes(first));
runtime.assert.ok(String(decoded.text ?? decoded).includes("REGION-"), "decoded barcode");
runtime.log("barcode-batch: wrote", regions.length, "barcodes to", tmp, "— first decodes to", decoded.text ?? decoded);
