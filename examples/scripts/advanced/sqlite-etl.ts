// Advanced demo: ETL over db.sqlite — schema, bulk insert inside a
// transaction via a prepared statement, aggregate GROUP BY query, then
// export the result as both JSON and XML.
//
// Uses only the db.sqlite API shown in examples/scripts/sqlite.ts:
//   open / exec / query / queryValue / begin / prepare / close

const conn = await db.sqlite.open(":memory:");

// ── schema ────────────────────────────────────────────────────────────────
await conn.exec(`
  CREATE TABLE events (
    id        INTEGER PRIMARY KEY,
    region    TEXT    NOT NULL,
    product   TEXT    NOT NULL,
    quantity  INTEGER NOT NULL,
    revenue   REAL    NOT NULL
  )
`);

await conn.exec(`
  CREATE INDEX idx_events_region ON events (region)
`);

// ── bulk insert inside a transaction via a prepared statement ──────────────
const eventData = [
  ["EU",  "Widget A", 10,  99.90],
  ["EU",  "Widget B", 5,   49.95],
  ["EU",  "Widget A", 7,   69.93],
  ["US",  "Widget A", 20, 199.80],
  ["US",  "Widget C", 3,   29.97],
  ["US",  "Widget B", 12,  119.88],
  ["APAC","Widget C", 8,   79.92],
  ["APAC","Widget A", 15, 149.85],
  ["APAC","Widget B", 4,   39.96],
  ["EU",  "Widget C", 6,   59.94],
] as const;

// Use tx.exec for the bulk insert so all writes are atomic.
const tx = await conn.begin();
for (const [region, product, quantity, revenue] of eventData) {
  await tx.exec(
    "INSERT INTO events (region, product, quantity, revenue) VALUES (?, ?, ?, ?)",
    region, product, quantity, revenue
  );
}
await tx.commit();

// Prepared statement demo: reuse a compiled query for lookups.
const byRegion = await conn.prepare(
  "SELECT count(*) FROM events WHERE region = ?"
);
runtime.log("=== prepared-statement lookup ===");
for (const r of ["EU", "US", "APAC"]) {
  const n = await byRegion.queryValue(r);
  runtime.log(`  ${r}: ${n} events`);
}
await byRegion.close();

runtime.log("=== row count ===");
const total = await conn.queryValue("SELECT count(*) FROM events");
runtime.log("total rows:", total);
runtime.assert.equal(total, 10, "expected 10 event rows");

// ── aggregate query: revenue + quantity by region ─────────────────────────
runtime.log("");
runtime.log("=== revenue by region ===");
const summary = await conn.query(`
  SELECT
    region,
    count(*)          AS num_orders,
    sum(quantity)     AS total_units,
    round(sum(revenue), 2) AS total_revenue
  FROM events
  GROUP BY region
  ORDER BY region
`);

for (const r of summary) {
  runtime.log(
    `  ${String(r.region).padEnd(5)}  orders:${r.num_orders}  units:${r.total_units}  revenue:${r.total_revenue}`
  );
}

// Spot-check: EU had 3 + 5 + 7 + 6 + 10 = 28 units → wait, let's compute:
// EU: Widget A 10+7=17, Widget B 5, Widget C 6  → 28 units
const euRevenue = await conn.queryValue(
  "SELECT round(sum(revenue),2) FROM events WHERE region = ?", "EU"
);
runtime.log("");
runtime.log("EU total revenue:", euRevenue);
runtime.assert.ok(Number(euRevenue) > 0, "EU revenue must be positive");

// ── product pivot: most popular by total units sold ───────────────────────
runtime.log("");
runtime.log("=== product totals ===");
const products = await conn.query(`
  SELECT product, sum(quantity) AS units
  FROM events
  GROUP BY product
  ORDER BY units DESC
`);
for (const p of products) {
  runtime.log(`  ${String(p.product).padEnd(10)}  ${p.units} units`);
}

// ── export to JSON ─────────────────────────────────────────────────────────
runtime.log("");
runtime.log("=== JSON export ===");
const jsonOut = JSON.stringify({ summary, products }, null, 2);
runtime.log("JSON length:", jsonOut.length, "chars");
const parsed = JSON.parse(jsonOut);
runtime.assert.equal(parsed.summary.length, summary.length, "JSON export row count");

// ── export to XML ──────────────────────────────────────────────────────────
runtime.log("");
runtime.log("=== XML export ===");

// codec.xml.encode expects a single root-key object.
// Arrays of records: each element is an object with element-named keys.
const xmlDoc = {
  report: {
    "@generated": new Date().toISOString().slice(0, 10),
    region: summary.map((r) => ({
      "@name":    String(r.region),
      num_orders: String(r.num_orders),
      total_units: String(r.total_units),
      total_revenue: String(r.total_revenue),
    })),
  },
};

const xml = codec.xml.encode(xmlDoc, { indent: "  ", declaration: true });
runtime.log(xml);

// Round-trip the XML and check region count
const xmlBack = codec.xml.decode(xml) as any;
const regions  = xmlBack?.report?.region;
const regionArr = Array.isArray(regions) ? regions : [regions];
runtime.assert.equal(regionArr.length, 3, "XML should have 3 region elements");

// ── tidy up ───────────────────────────────────────────────────────────────
await conn.close();
runtime.log("connection closed — PASS");
