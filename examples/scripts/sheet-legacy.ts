// Demonstrates codec.sheet reading a legacy read-only format (SYLK) and
// converting it up to XLSX, plus the capability matrix. Offline + self-contained.

// A tiny SYLK (.slk) document, synthesized inline.
const slk = [
  "ID;PWXL;N;E",
  'C;Y1;X1;K"item"', 'C;Y1;X2;K"qty"',
  'C;Y2;X1;K"apples"', "C;Y2;X2;K3",
  'C;Y3;X1;K"pears"', "C;Y3;X2;K5",
  "E", "",
].join("\n");

// Build a Uint8Array from the SYLK string (TextEncoder is not available in goja).
const slkBytes = new Uint8Array(slk.length);
for (let i = 0; i < slk.length; i++) {
  slkBytes[i] = slk.charCodeAt(i);
}

const wb = codec.sheet.read(slkBytes, { format: "slk" });
runtime.assert.equal(wb.sheets[0].rows.length, 3, "read 3 rows from SYLK");
runtime.assert.equal(wb.sheets[0].rows[1][0], "apples", "first data cell");

// Convert up to XLSX (a writable format).
const xlsx = codec.sheet.write(wb, { format: "xlsx" });
const back = codec.sheet.read(xlsx.bytes, { format: "xlsx" });
runtime.assert.equal(back.sheets[0].rows[2][0], "pears", "round-trips through xlsx");

// Capability matrix: legacy formats are read-only.
const f = codec.sheet.formats();
runtime.assert.ok(f.slk.read && !f.slk.write, "slk is read-only");
runtime.assert.ok(f.xlsx.read && f.xlsx.write, "xlsx is read+write");

runtime.log("sheet-legacy OK: SYLK→XLSX converted; formats:", Object.keys(f).join(","));
