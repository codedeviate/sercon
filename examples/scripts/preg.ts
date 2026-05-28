// Demonstrates api.text.preg.* — PHP-style /pattern/flags syntax over
// Go's RE2 engine. Three functions: match (first hit or null),
// matchAll (all hits as an array), replace (with $1/${1} backrefs).

api.runtime.log("=== match — first hit only ===");
const m = api.text.preg.match("/(\\w+)\\s+(\\d+)/", "alice 30 bob 27");
if (m) {
  api.runtime.log("full:", m.match);
  api.runtime.log("groups:", m.groups);   // ["alice", "30"]
  api.runtime.log("index:", m.index);     // 0
}

api.runtime.log("");
api.runtime.log("=== match returns null on no match ===");
api.runtime.log("none:", api.text.preg.match("/zzz/", "abc"));   // null

api.runtime.log("");
api.runtime.log("=== matchAll — every hit ===");
const all = api.text.preg.matchAll("/(\\w+)=(\\w+)/", "k1=v1 k2=v2 k3=v3");
for (const hit of all) {
  api.runtime.log(`  ${hit.groups[0]} -> ${hit.groups[1]}  (at index ${hit.index})`);
}

api.runtime.log("");
api.runtime.log("=== flags: i (case-insensitive), m (multiline), s (dotall) ===");
api.runtime.log("i:", api.text.preg.match("/HELLO/i", "Hello, world")?.match);  // Hello
api.runtime.log("m:", api.text.preg.matchAll("/^\\d+/m", "1\n22\n333").length); // 3
api.runtime.log("s:", api.text.preg.match("/a.b/s", "a\nb")?.match);            // "a\nb"

api.runtime.log("");
api.runtime.log("=== replace — Go's $1 / ${1} backref syntax ===");
const swapped = api.text.preg.replace(
  "/(\\w+)@(\\w+)/",
  "$2/$1",
  "alice@corp bob@dept",
);
api.runtime.log(swapped); // "corp/alice dept/bob"

api.runtime.log("");
api.runtime.log("=== unsupported PHP flag → clean error ===");
try {
  api.text.preg.match("/abc/u", "abc");
} catch (e) {
  api.runtime.log("caught:", String(e).slice(0, 80) + "…");
}
