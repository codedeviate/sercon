// Recipe: read products.xlsx and report low-stock items.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const book = codec.sheet.read(await fs.readBytes(data("products.xlsx")));
const products = book.sheets.find((s: any) => s.name === "Products")!;
const [header, ...rows] = products.rows;
const stockCol = header.indexOf("stock");
const low = rows.filter((r: any) => Number(r[stockCol]) < 10);
runtime.assert.ok(low.length >= 1, "found low-stock items");
runtime.log("inventory: low stock —", low.map((r: any) => `${r[0]} (${r[stockCol]})`).join(", "));
