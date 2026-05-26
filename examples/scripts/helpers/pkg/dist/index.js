// Decoy compiled output. The engine should never load this because
// package.json's `source` field points at src/lib.ts. If sercon ever
// regresses, this string will surface in the demo and the assertion
// in pkg-resolution.ts will fail.
module.exports = { v: "from-dist", greet: function (n) { return "this should not be reached"; } };
