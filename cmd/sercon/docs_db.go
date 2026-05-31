package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func dbDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"sqlite.open": {
			Summary: "Open a SQLite database (':memory:' or a file path; created if absent). Resolves to a handle { exec, query, queryValue, begin, prepare, close }. Connection is Ping-ed before resolving.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "\":memory:\" for an in-RAM database, or a filesystem path. Missing files are created by the modernc.org/sqlite (pure-Go, no cgo) driver."},
			},
			Returns: "Promise<handle> resolving to the shared SQL handle object: exec(sql, ...params) → Promise<{ rowsAffected: number, lastInsertId: number }>; query(sql, ...params) → Promise<object[]> (one ordered object per row, keyed by column name in column order); queryValue(sql, ...params) → Promise<any> (first column of the first row, or null when no rows match); begin() → Promise<tx> ({ exec, query, queryValue, commit, rollback }); prepare(sql) → Promise<stmt> ({ exec, query, queryValue, close }); close() → Promise<void>. SQLite uses ? positional placeholders. UTF-8 byte columns scan to strings; genuinely binary bytes surface as Uint8Array.",
			Errors:  "Throws if path is missing or empty, or if the connection ping fails (the *sql.DB is closed on ping failure rather than leaked). Subsequent exec/query/etc. throw the driver error on bad SQL or bind mismatch.",
			Example: `const db = await db.sqlite.open(":memory:");
await db.exec("CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)");
await db.exec("INSERT INTO t (name) VALUES (?)", "alice");
const rows = await db.query("SELECT * FROM t WHERE name = ?", "alice");
await db.close();`,
		},
		"postgres.open": {
			Summary: "Connect to PostgreSQL via the pure-Go pgx driver. Arg is a libpq DSN/URL string or an options object { host, port, user, password, database, sslmode }. Returns the shared SQL handle { exec, query, queryValue, begin, prepare, close }. Uses $1,$2,… placeholders. Pings on open.",
			Params: []scriptengine.Param{
				{Name: "dsn", Type: "string | { host?: string, port?: number, user?: string, password?: string, database?: string, sslmode?: string }", Desc: "A libpq DSN/URL string used verbatim, OR an options object assembled into a postgres:// URL (defaults: host localhost, port 5432, empty database; sslmode added to the query string when set). CockroachDB and other Postgres-wire engines connect through the same driver."},
			},
			Returns: "Promise<handle> resolving to the shared SQL handle: exec(sql, ...params) → Promise<{ rowsAffected, lastInsertId }>; query(sql, ...params) → Promise<object[]> (one ordered object per row, keyed by column name in column order); queryValue(sql, ...params) → Promise<any> (first column of the first row, or null); begin() → Promise<tx> ({ exec, query, queryValue, commit, rollback }); prepare(sql) → Promise<stmt> ({ exec, query, queryValue, close }); close() → Promise<void>. Postgres uses $1, $2, … positional placeholders.",
			Errors:  "Throws if no argument is given, the DSN string is empty, the argument is neither a string nor an object, or the connection ping fails (the pool is closed on ping failure).",
			Example: `const db = await db.postgres.open({ host: "localhost", user: "app", password: "s3cr3t", database: "shop", sslmode: "disable" });
const rows = await db.query("SELECT id, name FROM users WHERE id = $1", 42);
await db.close();`,
		},
		"mysql.open": {
			Summary: "Connect to MySQL/MariaDB via the pure-Go go-sql-driver. Arg is a go-sql-driver DSN string (user:pass@tcp(host:port)/db) or an options object { host, port, user, password, database }. Returns the shared SQL handle. Uses ? placeholders. Pings on open.",
			Params: []scriptengine.Param{
				{Name: "dsn", Type: "string | { host?: string, port?: number, user?: string, password?: string, database?: string }", Desc: "A go-sql-driver DSN string (user:pass@tcp(host:port)/db?params) used verbatim, OR an options object assembled into one (defaults: host localhost, port 3306, empty database). One driver serves both MySQL and MariaDB."},
			},
			Returns: "Promise<handle> resolving to the shared SQL handle: exec(sql, ...params) → Promise<{ rowsAffected, lastInsertId }>; query(sql, ...params) → Promise<object[]> (one ordered object per row); queryValue(sql, ...params) → Promise<any> (first column of the first row, or null); begin() → Promise<tx>; prepare(sql) → Promise<stmt>; close() → Promise<void>. MySQL uses ? positional placeholders.",
			Errors:  "Throws if no argument is given, the DSN string is empty, the argument is neither a string nor an object, or the connection ping fails.",
			Example: `const db = await db.mysql.open("app:s3cr3t@tcp(localhost:3306)/shop");
const n = await db.queryValue("SELECT COUNT(*) FROM orders WHERE status = ?", "paid");
await db.close();`,
		},
		"mssql.open": {
			Summary: "Connect to Microsoft SQL Server via the pure-Go go-mssqldb driver. Arg is a sqlserver:// URL DSN string or an options object { host, port, user, password, database }. Returns the shared SQL handle. Uses @p1,@p2,… placeholders. Pings on open.",
			Params: []scriptengine.Param{
				{Name: "dsn", Type: "string | { host?: string, port?: number, user?: string, password?: string, database?: string }", Desc: "A sqlserver:// URL DSN string used verbatim, OR an options object assembled into one (defaults: host localhost, port 1433; database goes in the URL query string per the go-mssqldb form)."},
			},
			Returns: "Promise<handle> resolving to the shared SQL handle: exec(sql, ...params) → Promise<{ rowsAffected, lastInsertId }>; query(sql, ...params) → Promise<object[]> (one ordered object per row); queryValue(sql, ...params) → Promise<any> (first column of the first row, or null); begin() → Promise<tx>; prepare(sql) → Promise<stmt>; close() → Promise<void>. SQL Server uses @p1, @p2, … placeholders.",
			Errors:  "Throws if no argument is given, the DSN string is empty, the argument is neither a string nor an object, or the connection ping fails.",
			Example: `const db = await db.mssql.open({ host: "localhost", user: "sa", password: "P@ss", database: "shop" });
const rows = await db.query("SELECT TOP 5 id, name FROM users WHERE region = @p1", "EU");
await db.close();`,
		},
		"clickhouse.open": {
			Summary: "Connect to ClickHouse via the pure-Go clickhouse-go v2 driver. Arg is a clickhouse:// URL DSN string or an options object { host, port, user, password, database, secure }. Returns the shared SQL handle { exec, query, queryValue, begin, prepare, close }. Uses ? placeholders; default native port 9000 (set secure:true for TLS). Pings on open.",
			Params: []scriptengine.Param{
				{Name: "dsn", Type: "string | { host?: string, port?: number, user?: string, password?: string, database?: string, secure?: boolean }", Desc: "A clickhouse:// URL DSN string used verbatim, OR an options object assembled into one (defaults: host localhost, port 9000 native protocol, empty database). secure:true appends secure=true to the URL for TLS (typically port 9440)."},
			},
			Returns: "Promise<handle> resolving to the shared SQL handle: exec(sql, ...params) → Promise<{ rowsAffected, lastInsertId }>; query(sql, ...params) → Promise<object[]> (one ordered object per row); queryValue(sql, ...params) → Promise<any> (first column of the first row, or null); begin() → Promise<tx>; prepare(sql) → Promise<stmt>; close() → Promise<void>. ClickHouse uses ? positional placeholders (or @name).",
			Errors:  "Throws if no argument is given, the DSN string is empty, the argument is neither a string nor an object, or the connection ping fails.",
			Example: `const db = await db.clickhouse.open({ host: "localhost", database: "metrics", secure: false });
const rows = await db.query("SELECT name, value FROM stats WHERE host = ?", "web1");
await db.close();`,
		},
		"oracle.open": {
			Summary: "Connect to Oracle via the pure-Go go-ora driver (no cgo). Arg is an oracle:// URL DSN string or an options object { host, port, user, password, database } where database is the service name. Returns the shared SQL handle. Uses :1,:2,… bind placeholders; default port 1521. Pings on open.",
			Params: []scriptengine.Param{
				{Name: "dsn", Type: "string | { host?: string, port?: number, user?: string, password?: string, database?: string }", Desc: "An oracle:// URL DSN string used verbatim, OR an options object assembled into one (defaults: host localhost, port 1521). database is the Oracle service name and goes in the URL path. The go-ora driver is pure Go, unlike the OCI-bound godror."},
			},
			Returns: "Promise<handle> resolving to the shared SQL handle: exec(sql, ...params) → Promise<{ rowsAffected, lastInsertId }>; query(sql, ...params) → Promise<object[]> (one ordered object per row); queryValue(sql, ...params) → Promise<any> (first column of the first row, or null); begin() → Promise<tx>; prepare(sql) → Promise<stmt>; close() → Promise<void>. Oracle uses :1, :2, … (or :name) bind placeholders.",
			Errors:  "Throws if no argument is given, the DSN string is empty, the argument is neither a string nor an object, or the connection ping fails.",
			Example: `const db = await db.oracle.open({ host: "localhost", user: "app", password: "s3cr3t", database: "ORCLPDB1" });
const rows = await db.query("SELECT id, name FROM users WHERE id = :1", 42);
await db.close();`,
		},
		"redis.open": {
			Summary: "Connect to Redis (redis://...). Returns { do, ping, close }. do(cmd, ...args) runs any RESP command; missing key -> null. Pings on open to surface bad addresses.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "A standard Redis URL: redis://[:password@]host:port/db (rediss:// for TLS), parsed by go-redis's ParseURL."},
			},
			Returns: "Promise<handle> resolving to { do, ping, close }: do(cmd, ...args) → Promise<any> runs an arbitrary RESP command (the first arg is the command name, the rest its arguments) and returns the reply coerced to a JS value — strings, numbers, arrays, or null; a nil reply (missing key) resolves to null rather than throwing. ping() → Promise<string> ('PONG'). close() → Promise<void>.",
			Errors:  "open throws if url is empty, the URL fails to parse, or the open-time ping fails (the client is closed on ping failure). do throws on Redis-level errors (WRONGTYPE, unknown command, etc.); a missing-key nil reply is data, not an error.",
			Example: `const r = await db.redis.open("redis://localhost:6379/0");
await r.do("SET", "greeting", "hi");
const v = await r.do("GET", "greeting"); // "hi"
const missing = await r.do("GET", "nope"); // null
await r.close();`,
		},
		"memcached.open": {
			Summary: "Connect to memcached (host:port). Returns { get, set, delete }. get -> string or null (miss); delete -> bool (existed). set(key, value, expirySeconds?). No ping on open; the pool is lazy.",
			Params: []scriptengine.Param{
				{Name: "addr", Type: "string", Desc: "A memcached server address, host:port (e.g. localhost:11211)."},
			},
			Returns: "Promise<handle> resolving to { get, set, delete }: get(key) → Promise<string | null> (null on a cache miss); set(key, value, expirySeconds?) → Promise<void> (value stored as bytes; expirySeconds 0 or omitted means never expire); delete(key) → Promise<boolean> (true if the key existed, false on a miss). gomemcache pools connections lazily, so there is no ping-on-open and no close method (the pool is GC'd with the handle).",
			Errors:  "open throws if addr is empty. set throws if key is empty or the value cannot be coerced to bytes. get / delete throw on transport errors; a cache miss is data (get → null, delete → false), not an error.",
			Example: `const mc = await db.memcached.open("localhost:11211");
await mc.set("session:42", "active", 300);
const v = await mc.get("session:42"); // "active" or null
const existed = await mc.delete("session:42"); // true`,
		},
		"ldap.open": {
			Summary: "Dial LDAP (ldap://host:port or ldaps://...), anonymous bind by default (or opts.bindDN/password). Returns { rootDSE, search, close }. search(baseDN, filter, attrs?) -> entries; rootDSE -> server metadata.",
			Params: []scriptengine.Param{
				{Name: "url", Type: "string", Desc: "An LDAP URL: ldap://host:port (or ldaps://... for TLS, e.g. ldap://localhost:389)."},
				{Name: "opts", Type: "{ bindDN?: string, password?: string }", Optional: true, Desc: "When bindDN is set, the connection binds with bindDN/password instead of doing an anonymous bind."},
			},
			Returns: "Promise<handle> resolving to { rootDSE, search, close }: rootDSE() → Promise<object> reads the server's Root DSE (an ordered { dn, <attr>: string[] } object advertising naming contexts, supported controls, vendor, etc.; an empty object when the server returns no entry); search(baseDN, filter, attrs?) → Promise<object[]> runs a whole-subtree search and returns one ordered { dn, <attr>: string[] } object per entry (multi-valued attributes stay arrays; filter defaults to (objectClass=*); attrs is an optional array of attribute names); close() → Promise<void>. A directory-inspection (read) binding, not a write/modify surface.",
			Errors:  "open throws if url is empty, the dial fails, or (when bindDN is set) the bind fails (the connection is closed on bind failure). rootDSE / search throw on the underlying LDAP search error.",
			Example: `const dir = await db.ldap.open("ldap://localhost:389");
const meta = await dir.rootDSE();
const people = await dir.search("ou=people,dc=example,dc=com", "(uid=alice)", ["cn", "mail"]);
await dir.close();`,
		},
		"dict.define": {
			Summary: "RFC 2229 DICT word lookup. define(host, word, opts?) -> { word, found, definitions: [{ db, dbName, text }] }. found:false on no match (not an error). One-shot: connect, query, QUIT.",
			Params: []scriptengine.Param{
				{Name: "host", Type: "string", Desc: "The DICT server hostname."},
				{Name: "word", Type: "string", Desc: "The word to look up."},
				{Name: "opts", Type: "{ database?: string, port?: string, timeout?: number }", Optional: true, Desc: "database selects a specific dictionary (default \"*\" = all); port is the DICT port (default \"2628\"); timeout is the dial/read deadline in milliseconds (default 10000)."},
			},
			Returns: "Promise<{ word: string, found: boolean, definitions: { db: string, dbName: string, text: string }[] }> — definitions carries one entry per matching dictionary (db is the dictionary code, dbName its human name, text the definition body). A word with no definitions resolves with found:false and an empty list.",
			Errors:  "Throws if host or word is empty, on dial/banner failure, or on an unexpected DICT status code (e.g. 550 invalid database). A 552 \"no match\" is NOT an error — it resolves with found:false.",
			Example: `const r = await db.dict.define("dict.org", "serendipity");
if (r.found) runtime.log(r.definitions[0].text);`,
		},
		"dict.match": {
			Summary: "RFC 2229 word match. match(host, word, opts?) -> { word, matches: [{ db, word }] }. opts.strategy (default prefix), opts.database (default *), opts.port (default 2628). One-shot: connect, query, QUIT.",
			Params: []scriptengine.Param{
				{Name: "host", Type: "string", Desc: "The DICT server hostname."},
				{Name: "word", Type: "string", Desc: "The word (or pattern) to match."},
				{Name: "opts", Type: "{ strategy?: string, database?: string, port?: string, timeout?: number }", Optional: true, Desc: "strategy is the match strategy (default \"prefix\"); database selects a specific dictionary (default \"*\" = all); port is the DICT port (default \"2628\"); timeout is the dial/read deadline in milliseconds (default 10000)."},
			},
			Returns: "Promise<{ word: string, matches: { db: string, word: string }[] }> — matches carries one entry per matched word (db is the dictionary it was found in, word the matched headword). No matches resolves with an empty matches list.",
			Errors:  "Throws if host or word is empty, on dial/banner failure, or on an unexpected DICT status code. A 552 \"no match\" is NOT an error — it resolves with an empty matches list.",
			Example: `const r = await db.dict.match("dict.org", "seren", { strategy: "prefix" });
runtime.log(r.matches.map(m => m.word));`,
		},
	}
}
