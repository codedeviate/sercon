// Demonstrates text.preg.* — PHP-style /pattern/flags syntax over
// Go's RE2 engine. Three functions: match (first hit or null),
// matchAll (all hits as an array), replace (with $1/${1} backrefs).

runtime.log("=== match — first hit only ===");
const m = text.preg.match("/(\\w+)\\s+(\\d+)/", "alice 30 bob 27");
if (m) {
  runtime.log("full:", m.match);
  runtime.log("groups:", m.groups);   // ["alice", "30"]
  runtime.log("index:", m.index);     // 0
}

runtime.log("");
runtime.log("=== match returns null on no match ===");
runtime.log("none:", text.preg.match("/zzz/", "abc"));   // null

runtime.log("");
runtime.log("=== matchAll — every hit ===");
const all = text.preg.matchAll("/(\\w+)=(\\w+)/", "k1=v1 k2=v2 k3=v3");
for (const hit of all) {
  runtime.log(`  ${hit.groups[0]} -> ${hit.groups[1]}  (at index ${hit.index})`);
}

runtime.log("");
runtime.log("=== flags: i (case-insensitive), m (multiline), s (dotall) ===");
runtime.log("i:", text.preg.match("/HELLO/i", "Hello, world")?.match);  // Hello
runtime.log("m:", text.preg.matchAll("/^\\d+/m", "1\n22\n333").length); // 3
runtime.log("s:", text.preg.match("/a.b/s", "a\nb")?.match);            // "a\nb"

runtime.log("");
runtime.log("=== replace — Go's $1 / ${1} backref syntax ===");
const swapped = text.preg.replace(
  "/(\\w+)@(\\w+)/",
  "$2/$1",
  "alice@corp bob@dept",
);
runtime.log(swapped); // "corp/alice dept/bob"

runtime.log("");
runtime.log("=== unsupported PHP flag → clean error ===");
try {
  text.preg.match("/abc/u", "abc");
} catch (e) {
  runtime.log("caught:", String(e).slice(0, 80) + "…");
}
