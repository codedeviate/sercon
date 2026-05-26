// Demonstrates api.sqlite.* — pure-Go SQLite (modernc.org/sqlite, no cgo).
// First stateful-handle binding: open() returns an object whose methods
// (exec / query / queryValue / close) are bound to the connection.

// :memory: keeps everything in RAM — evaporates when the handle closes.
const db = await api.sqlite.open(":memory:");

// === schema + insert ===
await db.exec(`
  CREATE TABLE users (
    id    INTEGER PRIMARY KEY,
    name  TEXT NOT NULL,
    age   INTEGER,
    email TEXT
  )
`);

// Parameters bind as ? placeholders, in order. Mixed types are fine —
// goja exports JS numbers/strings/null straight through to the driver.
const alice = await db.exec(
  "INSERT INTO users (name, age, email) VALUES (?, ?, ?)",
  "Alice", 30, "alice@example.com",
);
api.log("inserted Alice as id", alice.lastInsertId, "(rows:", alice.rowsAffected + ")");

await db.exec("INSERT INTO users (name, age, email) VALUES (?, ?, ?)", "Bob", 27, "bob@example.com");
await db.exec("INSERT INTO users (name, age) VALUES (?, ?)", "Carol", 35);

// === query returns row objects keyed by column name ===
api.log("");
api.log("=== all users, oldest first ===");
const rows = await db.query("SELECT name, age, email FROM users ORDER BY age DESC");
for (const r of rows) {
  api.log(`  ${r.name.padEnd(6)} age ${r.age}  ${r.email ?? "(no email)"}`);
}

// === queryValue: single scalar, or null when no row matches ===
api.log("");
api.log("=== scalars via queryValue ===");
api.log("user count:", await db.queryValue("SELECT count(*) FROM users"));
api.log("avg age:   ", await db.queryValue("SELECT round(avg(age), 1) FROM users"));
api.log("Bob's email:", await db.queryValue("SELECT email FROM users WHERE name = ?", "Bob"));
api.log("missing:   ", await db.queryValue("SELECT email FROM users WHERE name = ?", "Nobody"));

// === update + delete report rowsAffected ===
api.log("");
api.log("=== mutations ===");
const upd = await db.exec("UPDATE users SET age = age + 1 WHERE age < 30");
api.log("birthday for under-30s:", upd.rowsAffected, "rows");
const del = await db.exec("DELETE FROM users WHERE email IS NULL");
api.log("removed emailless users:", del.rowsAffected, "rows");

// === BLOB columns round-trip as Uint8Array ===
api.log("");
api.log("=== binary BLOB ===");
await db.exec("CREATE TABLE files (name TEXT, data BLOB)");
await db.exec("INSERT INTO files (name, data) VALUES (?, ?)", "logo.png", new Uint8Array([0x89, 0x50, 0x4e, 0x47]));
const data = await db.queryValue("SELECT data FROM files WHERE name = ?", "logo.png");
api.log("PNG magic bytes:", Array.from(data).map((b) => b.toString(16)).join(" "));

// Always close — there's no finalizer; an un-closed handle leaks the
// connection until the process exits.
await db.close();
api.log("");
api.log("handle closed");
