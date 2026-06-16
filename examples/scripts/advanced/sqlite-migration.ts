// Advanced demo: versioned schema migration + multi-table join over
// db.sqlite. Demonstrates an idempotent migration runner gated on a
// schema_version table, then a three-table JOIN (orders ⋈ customers ⋈
// regions) with GROUP BY aggregation. Self-contained and offline; runs in
// `make demo` and the CI offline subset (deterministic, like sqlite-etl.ts).
//
// Uses only the db.sqlite API: open / exec / query / queryValue / begin /
// prepare / close.

const conn = await db.sqlite.open(":memory:");

// schema_version holds a single row tracking the highest applied migration.
await conn.exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`);
const haveVersion = await conn.queryValue("SELECT count(*) FROM schema_version");
if (Number(haveVersion) === 0) {
  await conn.exec("INSERT INTO schema_version (version) VALUES (0)");
}

async function currentVersion(): Promise<number> {
  return Number(await conn.queryValue("SELECT version FROM schema_version"));
}
async function setVersion(v: number): Promise<void> {
  await conn.exec("UPDATE schema_version SET version = ?", v);
}

// Each migration is a function keyed by the version it brings the schema to.
// The runner applies only those greater than the recorded version, so a
// re-run is a no-op — the hallmark of a safe migration.
const migrations: Array<{ to: number; up: () => Promise<void> }> = [
  {
    to: 1,
    up: async () => {
      await conn.exec(`
        CREATE TABLE customers (
          id        INTEGER PRIMARY KEY,
          name      TEXT    NOT NULL,
          region_id INTEGER NOT NULL
        )`);
      await conn.exec(`
        CREATE TABLE orders (
          id          INTEGER PRIMARY KEY,
          customer_id INTEGER NOT NULL,
          amount      REAL    NOT NULL
        )`);
    },
  },
  {
    to: 2,
    up: async () => {
      // Additive ALTER + a new lookup table.
      await conn.exec(`ALTER TABLE customers ADD COLUMN tier TEXT NOT NULL DEFAULT 'standard'`);
      await conn.exec(`
        CREATE TABLE regions (
          id   INTEGER PRIMARY KEY,
          name TEXT    NOT NULL
        )`);
    },
  },
];

async function migrate(): Promise<number> {
  let applied = 0;
  for (const m of migrations) {
    if ((await currentVersion()) < m.to) {
      await m.up();
      await setVersion(m.to);
      applied++;
      runtime.log(`applied migration → v${m.to}`);
    }
  }
  return applied;
}

const firstRun = await migrate();
runtime.assert.equal(firstRun, 2, "first run applies both migrations");
runtime.assert.equal(await currentVersion(), 2, "schema is at version 2");

// Re-running is idempotent: nothing should apply the second time.
const secondRun = await migrate();
runtime.assert.equal(secondRun, 0, "re-running migrate() is a no-op");

// ── seed data in a transaction via prepared statements ─────────────────────
const tx = await conn.begin();
for (const [id, name] of [[1, "EU"], [2, "US"], [3, "APAC"]] as const) {
  await tx.exec("INSERT INTO regions (id, name) VALUES (?, ?)", id, name);
}
for (const [id, name, region, tier] of [
  [1, "Alice", 1, "gold"],
  [2, "Bob", 1, "silver"],
  [3, "Carol", 2, "gold"],
  [4, "Dan", 3, "silver"],
] as const) {
  await tx.exec(
    "INSERT INTO customers (id, name, region_id, tier) VALUES (?, ?, ?, ?)",
    id, name, region, tier,
  );
}
for (const [customer, amount] of [
  [1, 100], [1, 50],   // Alice → 150
  [2, 200],            // Bob   → 200   (EU total 350)
  [3, 300], [3, 25],   // Carol → 325   (US total 325)
  [4, 75],             // Dan   → 75    (APAC total 75)
] as const) {
  await tx.exec("INSERT INTO orders (customer_id, amount) VALUES (?, ?)", customer, amount);
}
await tx.commit();

// ── three-table JOIN + aggregation ─────────────────────────────────────────
const byRegion = await conn.query(`
  SELECT r.name              AS region,
         COUNT(o.id)         AS num_orders,
         ROUND(SUM(o.amount), 2) AS total
  FROM orders o
  JOIN customers c ON c.id = o.customer_id
  JOIN regions   r ON r.id = c.region_id
  GROUP BY r.name
  ORDER BY total DESC
`);

runtime.log("=== revenue by region (JOIN + GROUP BY) ===");
for (const row of byRegion) {
  runtime.log(`  ${String(row.region).padEnd(5)} orders:${row.num_orders} total:${row.total}`);
}

const totals: Record<string, number> = {};
for (const row of byRegion) totals[String(row.region)] = Number(row.total);
runtime.assert.equal(totals["EU"], 350, "EU total");
runtime.assert.equal(totals["US"], 325, "US total");
runtime.assert.equal(totals["APAC"], 75, "APAC total");

// ── per-tier customer count (uses the v2-added column) ─────────────────────
const goldCount = await conn.queryValue("SELECT count(*) FROM customers WHERE tier = ?", "gold");
runtime.assert.equal(Number(goldCount), 2, "two gold-tier customers");
runtime.log("gold-tier customers:", goldCount);

await conn.close();
runtime.log("done");
