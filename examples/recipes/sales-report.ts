// Recipe: read sales.csv, aggregate revenue by category, write an XLSX + ODS
// report. Outputs land in $TMPDIR (printed below).
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

const book = codec.sheet.read(await fs.readBytes(data("sales.csv")), { format: "csv" });
const rows = book.sheets[0].rows.slice(1); // drop header
const byCat: Record<string, number> = {};
for (const r of rows) {
  const cat = String(r[2]);
  byCat[cat] = (byCat[cat] ?? 0) + Number(r[4]);
}
const report = { sheets: [{ name: "ByCategory", rows: [["category", "revenue"], ...Object.entries(byCat)] }] };
const xlsxPath = `${tmp}/sercon-sales-report.xlsx`;
const odsPath = `${tmp}/sercon-sales-report.ods`;
await fs.writeBytes(xlsxPath, codec.sheet.write(report, { format: "xlsx" }).bytes);
await fs.writeBytes(odsPath, codec.sheet.write(report, { format: "ods" }).bytes);

const back = codec.sheet.read(await fs.readBytes(xlsxPath));
runtime.assert.equal(back.sheets[0].rows.length, Object.keys(byCat).length + 1, "report rows");
runtime.log("sales-report: wrote", xlsxPath, "and", odsPath, "—", Object.keys(byCat).length, "categories");
