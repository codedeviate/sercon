// Demonstrates ESM default-export interop. esbuild emits a CommonJS module
// with `{ __esModule: true, default: 42 }`, and the entry rewriter unwraps
// `mod.__esModule ? mod.default : mod` so `answer` is the bare 42.

import answer from "./helpers/answer";

runtime.assert.equal(answer, 42);
runtime.log("default import OK:", answer);
