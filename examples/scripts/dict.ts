// Demonstrates db.dict.* — RFC 2229 DICT protocol word lookups.
// Gracefully degrades without a reachable server.

const host = runtime.env.get("DICT_HOST") ?? "dict.org";
runtime.log("querying", host, "...");

try {
  // define: definitions of a word across one or all databases.
  const def = await db.dict.define(host, "serendipity", { timeout: 5000 });
  runtime.log("found:", def.found, "definitions:", def.definitions.length);
  if (def.definitions.length > 0) {
    runtime.log("first db:", def.definitions[0].dbName);
    runtime.log(def.definitions[0].text.split("\n").slice(0, 3).join("\n"));
  }

  runtime.log("");
  // match: words matching a prefix.
  const m = await db.dict.match(host, "serend", { strategy: "prefix", timeout: 5000 });
  runtime.log("prefix matches:", m.matches.slice(0, 5).map((x) => x.word).join(", "));
} catch (e) {
  runtime.log("no DICT server reachable — skipping:", String(e).slice(0, 60));
}
