// Recipe: author a multi-sheet XLSX workbook with typed cells. Combine two
// single-sheet sources (sales.csv, regions.tsv) into one workbook with named
// tabs and numeric cells — something CSV/TSV can't hold — then read it back to
// prove the sheets and number types survive. Writes the XLSX to $TMPDIR.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

// CSV/TSV cells come back as untyped strings.
const sales = codec.sheet.read(await fs.readBytes(data("sales.csv")), { format: "csv" });
const regions = codec.sheet.read(await fs.readBytes(data("regions.tsv")), { format: "tsv" });

// Build a two-sheet workbook; coerce units/revenue to numbers so XLSX stores
// them as numeric cells (the header row is passed through unchanged).
const salesRows = sales.sheets[0].rows.map((r, i) =>
  i === 0 ? r : [r[0], r[1], r[2], Number(r[3]), Number(r[4])]);
const book = {
  sheets: [
    { name: "Sales", rows: salesRows },
    { name: "Regions", rows: regions.sheets[0].rows },
  ],
};

// A multi-sheet workbook can't be written to CSV (single-sheet only).
let csvRejected = false;
try { codec.sheet.write(book, { format: "csv" }); } catch { csvRejected = true; }
runtime.assert.ok(csvRejected, "CSV is single-sheet; a 2-sheet write throws");

const xlsxPath = `${tmp}/sercon-workbook.xlsx`;
await fs.writeBytes(xlsxPath, codec.sheet.write(book, { format: "xlsx" }).bytes);

// Read back: both named sheets survive and numeric cells return as numbers.
const back = codec.sheet.read(await fs.readBytes(xlsxPath));
runtime.assert.equal(back.sheets.length, 2, "two sheets preserved");
runtime.assert.equal(back.sheets[0].name, "Sales", "sheet name preserved");
const revenue = back.sheets[0].rows[1][4];
runtime.assert.equal(typeof revenue, "number", "XLSX preserves numeric type (CSV would not)");

runtime.log(
  `xlsx-workbook: wrote a ${back.sheets.length}-sheet workbook to ${xlsxPath}`,
  `— first revenue cell = ${revenue} (${typeof revenue})`,
);
