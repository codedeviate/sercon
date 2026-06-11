// Demonstrates db.valkey.* — RESP client for Valkey (the open-source Redis
// fork) over redis/go-redis. Identical surface to db.redis; valkey:// and
// valkeys:// URLs are accepted (normalised to redis:// / rediss://).
// Gracefully degrades when no Valkey server is reachable, so it stays green
// in `make demo` without a running server.

const url = runtime.env.get("VALKEY_URL") ?? "valkey://localhost:6379/0";
runtime.log("connecting to", url, "...");

let r;
try {
  r = await db.valkey.open(url);
} catch (e) {
  runtime.log("no Valkey reachable — skipping demo:", String(e).slice(0, 60));
  runtime.log("(set VALKEY_URL or run a valkey-server to see it live)");
}

if (r) {
  runtime.log("PING ->", await r.ping());

  // `do` runs any RESP command — the binding stays small by not mirroring
  // hundreds of methods.
  await r.do("SET", "sercon:greeting", "hello from sercon");
  runtime.log("GET ->", await r.do("GET", "sercon:greeting"));

  await r.do("RPUSH", "sercon:list", "a", "b", "c");
  runtime.log("LRANGE ->", (await r.do("LRANGE", "sercon:list", "0", "-1")).join(", "));

  // Missing key returns null, not an error.
  runtime.log("missing key ->", await r.do("GET", "sercon:absent"));

  // Clean up + close.
  await r.do("DEL", "sercon:greeting", "sercon:list");
  await r.close();
  runtime.log("closed");
}
