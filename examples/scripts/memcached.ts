// Demonstrates api.db.memcached.* — text-protocol client over
// bradfitz/gomemcache. Gracefully degrades without a server.

const addr = api.runtime.env.get("MEMCACHED_ADDR") ?? "localhost:11211";
api.runtime.log("connecting to", addr, "...");

const m = await api.db.memcached.open(addr);
try {
  await m.set("sercon:k", "hello from sercon");
  api.runtime.log("get ->", await m.get("sercon:k"));
  api.runtime.log("miss ->", await m.get("sercon:absent"));   // null
  api.runtime.log("delete ->", await m.delete("sercon:k"));    // true
  api.runtime.log("delete miss ->", await m.delete("sercon:k")); // false
  // set with a 60-second expiry
  await m.set("sercon:ttl", "expires", 60);
  api.runtime.log("ttl get ->", await m.get("sercon:ttl"));
  await m.delete("sercon:ttl");
} catch (e) {
  api.runtime.log("no memcached reachable — skipping:", String(e).slice(0, 60));
  api.runtime.log("(set MEMCACHED_ADDR or run `memcached` to see it live)");
}
