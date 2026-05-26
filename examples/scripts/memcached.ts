// Demonstrates api.memcached.* — text-protocol client over
// bradfitz/gomemcache. Gracefully degrades without a server.

const addr = api.env.get("MEMCACHED_ADDR") ?? "localhost:11211";
api.log("connecting to", addr, "...");

const m = await api.memcached.open(addr);
try {
  await m.set("sercon:k", "hello from sercon");
  api.log("get ->", await m.get("sercon:k"));
  api.log("miss ->", await m.get("sercon:absent"));   // null
  api.log("delete ->", await m.delete("sercon:k"));    // true
  api.log("delete miss ->", await m.delete("sercon:k")); // false
  // set with a 60-second expiry
  await m.set("sercon:ttl", "expires", 60);
  api.log("ttl get ->", await m.get("sercon:ttl"));
  await m.delete("sercon:ttl");
} catch (e) {
  api.log("no memcached reachable — skipping:", String(e).slice(0, 60));
  api.log("(set MEMCACHED_ADDR or run `memcached` to see it live)");
}
