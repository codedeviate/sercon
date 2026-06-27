// Recipe: csv -> objects -> JSON -> XML round-trip; write JSON + XML to tmp.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const tmp = runtime.env.get("TMPDIR") ?? "/tmp";

const book = codec.sheet.read(await fs.readBytes(data("sales.csv")), { format: "csv" });
const [header, ...rows] = book.sheets[0].rows;
const records = rows.map((r) => Object.fromEntries(header.map((h, i) => [String(h), r[i]])));
const jsonText = JSON.stringify(records, null, 2);
const xml = codec.xml.encode({ sales: { row: records } });
const decoded = codec.xml.decode(xml);
const decodedRows = Array.isArray(decoded.sales.row) ? decoded.sales.row : [decoded.sales.row];
runtime.assert.ok(decodedRows.length === records.length, "xml round-trip row count");

const jsonPath = `${tmp}/sercon-sales.json`;
const xmlPath = `${tmp}/sercon-sales.xml`;
await fs.writeText(jsonPath, jsonText);
await fs.writeText(xmlPath, xml);
runtime.log("format-convert: wrote", jsonPath, "and", xmlPath, `(${records.length} rows)`);
