// Demonstrates db.redis.* — RESP client over redis/go-redis.
// Gracefully degrades when no Redis server is reachable, so it stays
// green in `make demo` without a running server.

const url = runtime.env.get("REDIS_URL") ?? "redis://localhost:6379/0";
runtime.log("connecting to", url, "...");

let r;
try {
  r = await db.redis.open(url);
} catch (e) {
  runtime.log("no Redis reachable — skipping demo:", String(e).slice(0, 60));
  runtime.log("(set REDIS_URL or run `redis-server` to see it live)");
}

if (r) {
  runtime.log("PING ->", await r.ping());

  // `do` runs any command — the binding stays small by not mirroring
  // hundreds of methods.
  await r.do("SET", "sercon:greeting", "hello from sercon");
  runtime.log("GET ->", await r.do("GET", "sercon:greeting"));

  await r.do("RPUSH", "sercon:list", "a", "b", "c");
  runtime.log("LRANGE ->", (await r.do("LRANGE", "sercon:list", "0", "-1")).join(", "));

  await r.do("HSET", "sercon:hash", "field", "value");
  runtime.log("HGET ->", await r.do("HGET", "sercon:hash", "field"));

  // Missing key returns null, not an error.
  runtime.log("missing key ->", await r.do("GET", "sercon:absent"));

  // Clean up + close.
  await r.do("DEL", "sercon:greeting", "sercon:list", "sercon:hash");
  await r.close();
  runtime.log("closed");
}
