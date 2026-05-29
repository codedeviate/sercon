// Demonstrates db.sqlite.* — pure-Go SQLite (modernc.org/sqlite, no cgo).
// First stateful-handle binding: open() returns an object whose methods
// (exec / query / queryValue / close) are bound to the connection.

// :memory: keeps everything in RAM — evaporates when the handle closes.
const conn = await db.sqlite.open(":memory:");

// === schema + insert ===
await conn.exec(`
  CREATE TABLE users (
    id    INTEGER PRIMARY KEY,
    name  TEXT NOT NULL,
    age   INTEGER,
    email TEXT
  )
`);

// Parameters bind as ? placeholders, in order. Mixed types are fine —
// goja exports JS numbers/strings/null straight through to the driver.
const alice = await conn.exec(
  "INSERT INTO users (name, age, email) VALUES (?, ?, ?)",
  "Alice", 30, "alice@example.com",
);
runtime.log("inserted Alice as id", alice.lastInsertId, "(rows:", alice.rowsAffected + ")");

await conn.exec("INSERT INTO users (name, age, email) VALUES (?, ?, ?)", "Bob", 27, "bob@example.com");
await conn.exec("INSERT INTO users (name, age) VALUES (?, ?)", "Carol", 35);

// === query returns row objects keyed by column name ===
runtime.log("");
runtime.log("=== all users, oldest first ===");
const rows = await conn.query("SELECT name, age, email FROM users ORDER BY age DESC");
for (const r of rows) {
  runtime.log(`  ${r.name.padEnd(6)} age ${r.age}  ${r.email ?? "(no email)"}`);
}

// === queryValue: single scalar, or null when no row matches ===
runtime.log("");
runtime.log("=== scalars via queryValue ===");
runtime.log("user count:", await conn.queryValue("SELECT count(*) FROM users"));
runtime.log("avg age:   ", await conn.queryValue("SELECT round(avg(age), 1) FROM users"));
runtime.log("Bob's email:", await conn.queryValue("SELECT email FROM users WHERE name = ?", "Bob"));
runtime.log("missing:   ", await conn.queryValue("SELECT email FROM users WHERE name = ?", "Nobody"));

// === update + delete report rowsAffected ===
runtime.log("");
runtime.log("=== mutations ===");
const upd = await conn.exec("UPDATE users SET age = age + 1 WHERE age < 30");
runtime.log("birthday for under-30s:", upd.rowsAffected, "rows");
const del = await conn.exec("DELETE FROM users WHERE email IS NULL");
runtime.log("removed emailless users:", del.rowsAffected, "rows");

// === BLOB columns round-trip as Uint8Array ===
runtime.log("");
runtime.log("=== binary BLOB ===");
await conn.exec("CREATE TABLE files (name TEXT, data BLOB)");
await conn.exec("INSERT INTO files (name, data) VALUES (?, ?)", "logo.png", new Uint8Array([0x89, 0x50, 0x4e, 0x47]));
const data = await conn.queryValue("SELECT data FROM files WHERE name = ?", "logo.png");
runtime.log("PNG magic bytes:", Array.from(data).map((b) => b.toString(16)).join(" "));

// === transactions: begin() returns a nested handle ===
runtime.log("");
runtime.log("=== transactions ===");
// commit makes the batch visible atomically.
const tx = await conn.begin();
await tx.exec("INSERT INTO users (name, age, email) VALUES (?, ?, ?)", "Dave", 50, "dave@example.com");
await tx.exec("INSERT INTO users (name, age, email) VALUES (?, ?, ?)", "Eve", 22, "eve@example.com");
runtime.log("inside tx, user count:", await tx.queryValue("SELECT count(*) FROM users"));
await tx.commit();
runtime.log("after commit, user count:", await conn.queryValue("SELECT count(*) FROM users"));

// rollback discards everything since begin().
const tx2 = await conn.begin();
await tx2.exec("DELETE FROM users");
runtime.log("inside tx2 (after DELETE):", await tx2.queryValue("SELECT count(*) FROM users"));
await tx2.rollback();
runtime.log("after rollback:", await conn.queryValue("SELECT count(*) FROM users"));

// A constraint violation throws; roll back and the table is untouched.
const tx3 = await conn.begin();
try {
  await tx3.exec("INSERT INTO users (id, name) VALUES (?, ?)", 1, "duplicate-id"); // PK clash
} catch (e) {
  runtime.log("constraint caught:", String(e).slice(0, 60) + "…");
}
await tx3.rollback();

// === prepared statements: compile once, run many ===
runtime.log("");
runtime.log("=== prepared statements ===");
const insert = await conn.prepare("INSERT INTO users (name, age) VALUES (?, ?)");
for (const [name, age] of [["Frank", 40], ["Grace", 33], ["Heidi", 28]] as const) {
  await insert.exec(name, age);
}
await insert.close();
runtime.log("after batch insert, count:", await conn.queryValue("SELECT count(*) FROM users"));

// Prepared query / queryValue take only bind params — no SQL string.
const byName = await conn.prepare("SELECT age FROM users WHERE name = ?");
runtime.log("Grace's age:", await byName.queryValue("Grace"));
runtime.log("Frank's age:", await byName.queryValue("Frank"));
await byName.close();

// Always close — there's no finalizer; an un-closed handle leaks the
// connection until the process exits. Same for transactions (every
// begin() must reach commit/rollback) and prepared statements (every
// prepare() must be closed).
await conn.close();
runtime.log("");
runtime.log("handle closed");
