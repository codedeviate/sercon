// Demonstrates db.memcached.* — text-protocol client over
// bradfitz/gomemcache. Gracefully degrades without a server.

const addr = runtime.env.get("MEMCACHED_ADDR") ?? "localhost:11211";
runtime.log("connecting to", addr, "...");

const m = await db.memcached.open(addr);
try {
  await m.set("sercon:k", "hello from sercon");
  runtime.log("get ->", await m.get("sercon:k"));
  runtime.log("miss ->", await m.get("sercon:absent"));   // null
  runtime.log("delete ->", await m.delete("sercon:k"));    // true
  runtime.log("delete miss ->", await m.delete("sercon:k")); // false
  // set with a 60-second expiry
  await m.set("sercon:ttl", "expires", 60);
  runtime.log("ttl get ->", await m.get("sercon:ttl"));
  await m.delete("sercon:ttl");
} catch (e) {
  runtime.log("no memcached reachable — skipping:", String(e).slice(0, 60));
  runtime.log("(set MEMCACHED_ADDR or run `memcached` to see it live)");
}
