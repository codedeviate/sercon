// Recipe: rescue an old spreadsheet. Read a legacy SYLK (.slk) export — a
// read-only format — and convert it up to a modern, writable XLSX. The format
// is auto-detected from the file's content. Writes the XLSX to $TMPDIR.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

const wb = codec.sheet.read(await fs.readBytes(data("legacy.slk"))); // detected via the "ID;" magic
runtime.assert.equal(wb.format, "slk", "detected SYLK");
const rowCount = wb.sheets[0].rows.length;
runtime.assert.ok(rowCount >= 2, "read at least a header + one data row");

// Convert up to XLSX (a read+write format) and confirm the data survives.
const xlsxPath = `${tmp}/sercon-legacy.xlsx`;
await fs.writeBytes(xlsxPath, codec.sheet.write(wb, { format: "xlsx" }).bytes);
const back = codec.sheet.read(await fs.readBytes(xlsxPath));
runtime.assert.equal(back.sheets[0].rows.length, rowCount, "row count preserved through conversion");

// Legacy formats are read-only — writing one throws.
let rejected = false;
try { codec.sheet.write(wb, { format: "slk" }); } catch { rejected = true; }
runtime.assert.ok(rejected, "SYLK is read-only");

const fmts = codec.sheet.formats();
runtime.log(
  `sheet-legacy-convert: ${rowCount} rows SYLK→XLSX at ${xlsxPath}`,
  `— slk.write=${fmts.slk.write}, xlsx.write=${fmts.xlsx.write}`,
);
