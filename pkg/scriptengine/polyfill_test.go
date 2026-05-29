package scriptengine

import (
	"context"
	"strings"
	"testing"
)

// TestAsyncIteratorPolyfill confirms Symbol.asyncIterator is defined
// inside scripts. Without this, esbuild's lowered for-await helper
// keys off Symbol.for("Symbol.asyncIterator") while idiomatic JS that
// installs `obj[Symbol.asyncIterator] = …` would target the
// (undefined) intrinsic — the two halves wouldn't agree on the key
// and for-await would silently never call next(). The polyfill makes
// `Symbol.asyncIterator === Symbol.for("Symbol.asyncIterator")`, which
// is the same value esbuild's __forAwait helper looks up.
func TestAsyncIteratorPolyfill(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	script := `
if (typeof Symbol.asyncIterator !== "symbol") {
  throw new Error("Symbol.asyncIterator should be a symbol, got " + typeof Symbol.asyncIterator);
}
if (Symbol.asyncIterator !== Symbol.for("Symbol.asyncIterator")) {
  throw new Error("Symbol.asyncIterator should equal Symbol.for(\"Symbol.asyncIterator\")");
}
`
	if _, err := eng.Run(context.Background(), "polyfill.ts", script); err != nil {
		t.Fatalf("run: %v", err)
	}
}

// TestForAwaitOf_LoweredByEsbuild confirms that for-await-of is lowered
// by esbuild (via the Supported feature flag in transpile.go) and runs
// to completion in goja. Direct goja parsing of `for await` would
// throw a SyntaxError; this exercises both the esbuild lowering and
// the polyfill that makes the lowered helper find the iterator.
func TestForAwaitOf_LoweredByEsbuild(t *testing.T) {
	eng := New(Options{DisableConsole: true})
	script := `
const collected = [];
const it = {
  [Symbol.asyncIterator]() { return this; },
  i: 0,
  next() {
    if (this.i >= 3) return Promise.resolve({done: true, value: undefined});
    return Promise.resolve({done: false, value: this.i++});
  },
};
const run = async () => {
  for await (const v of it) collected.push(v);
};
await run();
if (collected.join(",") !== "0,1,2") {
  throw new Error("collected = " + collected.join(","));
}
`
	if _, err := eng.Run(context.Background(), "forawait.ts", script); err != nil {
		// surface esbuild syntax errors clearly so a future goja bump that
		// breaks lowering points the reader at the right place
		if strings.Contains(err.Error(), "SyntaxError") {
			t.Fatalf("for-await not lowered (esbuild Supported flag likely broken): %v", err)
		}
		t.Fatalf("run: %v", err)
	}
}
