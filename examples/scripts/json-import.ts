// Demonstrates that .json files are loadable both via `import data from
// "./data.json"` (esbuild rewrites it to a require, the entry rewriter
// unwraps the default) and `require("./data.json")` (handled natively by
// goja_nodejs's require module via its JSON code path).

import data from "./helpers/data.json";

api.assert.equal(data.name, "abc");
api.assert.equal(data.n, 7);
api.assert.equal(data.tags.length, 3);
api.log("import OK:", JSON.stringify(data));

const r = require("./helpers/data.json");
api.assert.equal(r.name, "abc");
api.log("require OK:", r.tags.join(", "));
