// Demonstrates two module-resolution behaviours:
//
//   1. A package.json with a `source` field pointing at a .ts file takes
//      precedence over `main` (which here points at a decoy compiled JS
//      file).
//   2. Failing that, the resolver's .js -> .ts swap would still find the
//      TS source if only the .js path was listed.
//
// The helpers/pkg fixture under this directory ships both a real TS
// source (src/lib.ts) and a decoy dist/index.js — if sercon ever picked
// the dist file, the assertion below would surface it.

import { v, greet } from "./helpers/pkg";

api.assert.equal(v, "from-source");
api.assert.ok(greet("world").includes("src/lib.ts"));
api.log("source preferred:", v);
api.log("greet:           ", greet("world"));
