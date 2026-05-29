// Demonstrates that .tsx modules are resolved through the source loader's
// extension fallback and transpiled by esbuild's LoaderTSX. The TSX file
// uses an @jsx pragma so JSX compiles to a local factory instead of
// React.createElement.

import { makeBox } from "./helpers/el";

const box = makeBox("hello");
runtime.assert.equal(box.tag, "div");
runtime.assert.equal(box.props.className, "box");
runtime.assert.equal(box.children[0], "hello");
runtime.log("tsx OK:", JSON.stringify(box));
