package scriptengine

import "github.com/dop251/goja"

// asyncIteratorPolyfillProg defines Symbol.asyncIterator on the VM if
// the runtime didn't ship it. As of goja@2026-03-11 the `Symbol`
// constructor exists but `Symbol.asyncIterator` is undefined — which
// breaks anything that uses `for await … of …`, because esbuild's
// lowered helper (`__forAwait`) looks up `obj[Symbol.asyncIterator]`
// to discover the async-iterator protocol. esbuild has a fallback
// (`Symbol.for("Symbol.asyncIterator")`), but consumers that install
// the method via the *real* `Symbol.asyncIterator` (idiomatic JS)
// would never be picked up because the two symbols are distinct.
//
// Installing the well-known symbol here harmonises both sides: user
// code and lowered helper code agree on the same key. Compile once at
// package init; *goja.Program is safe to share across runtimes.
//
// Identity over `Symbol.for` (a globally-registered symbol) is the
// closest goja-compatible stand-in for the spec's intrinsic — same
// "if you ask for it by name you get the same value" property. The
// well-known async iterator in V8/SpiderMonkey isn't registered, but
// behaviour-wise the only thing scripts can observe is the equality
// `Symbol.asyncIterator === Symbol.asyncIterator`, which holds here.
var asyncIteratorPolyfillProg = goja.MustCompile("internal:polyfill-async-iter",
	`if (typeof Symbol !== "undefined" && Symbol.asyncIterator === undefined) {
		try { Object.defineProperty(Symbol, "asyncIterator", { value: Symbol.for("Symbol.asyncIterator") }); }
		catch (_) { Symbol.asyncIterator = Symbol.for("Symbol.asyncIterator"); }
	}`, false)

// installPolyfills runs the per-Run polyfill program once on the loop's
// VM. Called from Engine.Run inside the loop callback before user code
// has a chance to execute. Errors are non-fatal — a missing polyfill
// just degrades the affected features (currently: for-await-of) rather
// than failing the whole script.
func installPolyfills(vm *goja.Runtime) {
	_, _ = vm.RunProgram(asyncIteratorPolyfillProg)
}
