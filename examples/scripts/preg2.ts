// Demonstrates api.text.preg2.* — the PCRE-flavoured regex engine
// (dlclark/regexp2). Same /pattern/flags syntax and { match, groups,
// index } shape as api.text.preg, but with the features RE2 can't do:
// lookahead, lookbehind, backreferences, possessive quantifiers.
// Trade-off: no linear-time guarantee, so don't run untrusted patterns
// without a timeout.

api.runtime.log("=== lookahead — 'foo' only when followed by 'bar' ===");
api.runtime.log(JSON.stringify(api.text.preg2.match("/foo(?=bar)/", "foobaz foobar")));

api.runtime.log("");
api.runtime.log("=== lookbehind — digits preceded by a $ ===");
api.runtime.log(api.text.preg2.match("/(?<=\\$)\\d+(\\.\\d+)?/", "total: $42.50")?.match);

api.runtime.log("");
api.runtime.log("=== backreference — a doubled word ===");
api.runtime.log(api.text.preg2.match("/\\b(\\w+)\\s+\\1\\b/", "the the quick brown")?.match);

api.runtime.log("");
api.runtime.log("=== matchAll + replace (same shape as api.text.preg) ===");
api.runtime.log("all:", api.text.preg2.matchAll("/\\d+/", "a1 b22 c333").map((m) => m.match).join(","));
api.runtime.log("swap:", api.text.preg2.replace("/(\\w+)@(\\w+)/", "$2/$1", "alice@corp"));

api.runtime.log("");
api.runtime.log("=== x flag — whitespace-insensitive patterns (RE2 can't) ===");
api.runtime.log("matches:", api.text.preg2.match("/ \\d{3} - \\d{4} /x", "555-1234") !== null);

api.runtime.log("");
api.runtime.log("Use api.text.preg (RE2) when you don't need these features — it has a");
api.runtime.log("linear-time guarantee. api.text.preg2 backtracks and can blow up on");
api.runtime.log("pathological patterns, so keep a timeout around untrusted input.");
