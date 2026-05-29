// Demonstrates text.preg2.* — the PCRE-flavoured regex engine
// (dlclark/regexp2). Same /pattern/flags syntax and { match, groups,
// index } shape as text.preg, but with the features RE2 can't do:
// lookahead, lookbehind, backreferences, possessive quantifiers.
// Trade-off: no linear-time guarantee, so don't run untrusted patterns
// without a timeout.

runtime.log("=== lookahead — 'foo' only when followed by 'bar' ===");
runtime.log(JSON.stringify(text.preg2.match("/foo(?=bar)/", "foobaz foobar")));

runtime.log("");
runtime.log("=== lookbehind — digits preceded by a $ ===");
runtime.log(text.preg2.match("/(?<=\\$)\\d+(\\.\\d+)?/", "total: $42.50")?.match);

runtime.log("");
runtime.log("=== backreference — a doubled word ===");
runtime.log(text.preg2.match("/\\b(\\w+)\\s+\\1\\b/", "the the quick brown")?.match);

runtime.log("");
runtime.log("=== matchAll + replace (same shape as text.preg) ===");
runtime.log("all:", text.preg2.matchAll("/\\d+/", "a1 b22 c333").map((m) => m.match).join(","));
runtime.log("swap:", text.preg2.replace("/(\\w+)@(\\w+)/", "$2/$1", "alice@corp"));

runtime.log("");
runtime.log("=== x flag — whitespace-insensitive patterns (RE2 can't) ===");
runtime.log("matches:", text.preg2.match("/ \\d{3} - \\d{4} /x", "555-1234") !== null);

runtime.log("");
runtime.log("Use text.preg (RE2) when you don't need these features — it has a");
runtime.log("linear-time guarantee. text.preg2 backtracks and can blow up on");
runtime.log("pathological patterns, so keep a timeout around untrusted input.");
