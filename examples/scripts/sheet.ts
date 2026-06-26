// codec.sheet — write a workbook to XLSX + CSV, read both back, check types.
const wb = { sheets: [{ name: "Inventory", rows: [
  ["Name", "Qty", "InStock"],
  ["Widget", 42, true],
  ["Gadget", 7, false],
] }] };

const xlsx = codec.sheet.write(wb, { format: "xlsx" });
const backX = codec.sheet.read(xlsx.bytes);
runtime.assert.equal(backX.sheets[0].name, "Inventory", "sheet name");
runtime.assert.equal(backX.sheets[0].rows[1][1], 42, "xlsx keeps numbers typed");
runtime.assert.equal(backX.sheets[0].rows[1][2], true, "xlsx keeps bools typed");

const csv = codec.sheet.write(wb, { format: "csv" });
const backC = codec.sheet.read(csv.bytes, { format: "csv" });
runtime.assert.equal(backC.sheets[0].rows[1][1], "42", "csv cells are strings");
runtime.log("sheet demo OK:", backX.format, "/", backC.format);

// --- ODS (OpenDocument Spreadsheet) ---
const odsOut = codec.sheet.write(
  { sheets: [{ name: "Sales", rows: [["Item", "Qty", "Active"], ["Widget", 42, true]] }] },
  { format: "ods" },
);
const odsBack = codec.sheet.read(odsOut.bytes);
runtime.assert.equal(odsBack.format, "ods", "sniffed as ods");
runtime.assert.equal(odsBack.sheets[0].rows[1][1], 42, "ods keeps numbers typed");
runtime.assert.equal(odsBack.sheets[0].rows[1][2], true, "ods keeps bools typed");
runtime.log("sheet ODS round-trip OK:", odsBack.sheets[0].name);
