// Demonstrates the `console` global — a browser/Node-style shim so scripts
// pasted from those environments run unchanged. log/info/debug go to stdout;
// warn/error go to stderr. Output is clean and space-joined (no timestamp).
// runtime.log is the native equivalent of console.log.

console.log("console.log:", "to stdout", 1 + 2, { ok: true });
console.info("console.info:", "also stdout");
console.debug("console.debug:", "also stdout");

console.warn("console.warn:", "to stderr");
console.error("console.error:", "to stderr");

runtime.log("(runtime.log — the native stdout logger)");
