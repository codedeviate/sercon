// Demonstrates api.preg2.* — the PCRE-flavoured regex engine
// (dlclark/regexp2). Same /pattern/flags syntax and { match, groups,
// index } shape as api.preg, but with the features RE2 can't do:
// lookahead, lookbehind, backreferences, possessive quantifiers.
// Trade-off: no linear-time guarantee, so don't run untrusted patterns
// without a timeout.

api.log("=== lookahead — 'foo' only when followed by 'bar' ===");
api.log(JSON.stringify(api.preg2.match("/foo(?=bar)/", "foobaz foobar")));

api.log("");
api.log("=== lookbehind — digits preceded by a $ ===");
api.log(api.preg2.match("/(?<=\\$)\\d+(\\.\\d+)?/", "total: $42.50")?.match);

api.log("");
api.log("=== backreference — a doubled word ===");
api.log(api.preg2.match("/\\b(\\w+)\\s+\\1\\b/", "the the quick brown")?.match);

api.log("");
api.log("=== matchAll + replace (same shape as api.preg) ===");
api.log("all:", api.preg2.matchAll("/\\d+/", "a1 b22 c333").map((m) => m.match).join(","));
api.log("swap:", api.preg2.replace("/(\\w+)@(\\w+)/", "$2/$1", "alice@corp"));

api.log("");
api.log("=== x flag — whitespace-insensitive patterns (RE2 can't) ===");
api.log("matches:", api.preg2.match("/ \\d{3} - \\d{4} /x", "555-1234") !== null);

api.log("");
api.log("Use api.preg (RE2) when you don't need these features — it has a");
api.log("linear-time guarantee. api.preg2 backtracks and can blow up on");
api.log("pathological patterns, so keep a timeout around untrusted input.");
