// Demonstrates api.dict.* — RFC 2229 DICT protocol word lookups.
// Gracefully degrades without a reachable server.

const host = api.env.get("DICT_HOST") ?? "dict.org";
api.log("querying", host, "...");

try {
  // define: definitions of a word across one or all databases.
  const def = await api.dict.define(host, "serendipity", { timeout: 5000 });
  api.log("found:", def.found, "definitions:", def.definitions.length);
  if (def.definitions.length > 0) {
    api.log("first db:", def.definitions[0].dbName);
    api.log(def.definitions[0].text.split("\n").slice(0, 3).join("\n"));
  }

  api.log("");
  // match: words matching a prefix.
  const m = await api.dict.match(host, "serend", { strategy: "prefix", timeout: 5000 });
  api.log("prefix matches:", m.matches.slice(0, 5).map((x) => x.word).join(", "));
} catch (e) {
  api.log("no DICT server reachable — skipping:", String(e).slice(0, 60));
}
