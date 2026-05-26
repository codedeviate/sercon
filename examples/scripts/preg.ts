// Demonstrates api.preg.* — PHP-style /pattern/flags syntax over
// Go's RE2 engine. Three functions: match (first hit or null),
// matchAll (all hits as an array), replace (with $1/${1} backrefs).

api.log("=== match — first hit only ===");
const m = api.preg.match("/(\\w+)\\s+(\\d+)/", "alice 30 bob 27");
if (m) {
  api.log("full:", m.match);
  api.log("groups:", m.groups);   // ["alice", "30"]
  api.log("index:", m.index);     // 0
}

api.log("");
api.log("=== match returns null on no match ===");
api.log("none:", api.preg.match("/zzz/", "abc"));   // null

api.log("");
api.log("=== matchAll — every hit ===");
const all = api.preg.matchAll("/(\\w+)=(\\w+)/", "k1=v1 k2=v2 k3=v3");
for (const hit of all) {
  api.log(`  ${hit.groups[0]} -> ${hit.groups[1]}  (at index ${hit.index})`);
}

api.log("");
api.log("=== flags: i (case-insensitive), m (multiline), s (dotall) ===");
api.log("i:", api.preg.match("/HELLO/i", "Hello, world")?.match);  // Hello
api.log("m:", api.preg.matchAll("/^\\d+/m", "1\n22\n333").length); // 3
api.log("s:", api.preg.match("/a.b/s", "a\nb")?.match);            // "a\nb"

api.log("");
api.log("=== replace — Go's $1 / ${1} backref syntax ===");
const swapped = api.preg.replace(
  "/(\\w+)@(\\w+)/",
  "$2/$1",
  "alice@corp bob@dept",
);
api.log(swapped); // "corp/alice dept/bob"

api.log("");
api.log("=== unsupported PHP flag → clean error ===");
try {
  api.preg.match("/abc/u", "abc");
} catch (e) {
  api.log("caught:", String(e).slice(0, 80) + "…");
}
