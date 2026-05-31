package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func runtimeDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"log": {
			Summary: "Print one space-separated line of the arguments to stdout. Primitives print raw; objects/arrays render as JSON (circular refs fall back to [object Object]). The script-side equivalent of console.log.",
			Params: []scriptengine.Param{
				{Name: "args", Type: "unknown[]", Desc: "Zero or more values to print. They are joined with single spaces; primitives stringify directly, objects/arrays are JSON-encoded."},
			},
			ReturnType: "void",
			Returns:    "void — writes a single newline-terminated line to stdout.",
			Errors:     "Never throws; unserialisable objects degrade to [object Object] rather than erroring.",
			Example:    `runtime.log("count", 3, { ok: true }); // count 3 {"ok":true}`,
		},
		"assert.equal": {
			Summary: "Throw when actual != expected (strict equality on primitives, deep equality on objects). Optional msg appears in the error.",
			Params: []scriptengine.Param{
				{Name: "actual", Type: "unknown", Desc: "The value produced by the code under test."},
				{Name: "expected", Type: "unknown", Desc: "The value to compare against. Primitives use strict equality; objects/arrays use deep structural equality (key order ignored)."},
				{Name: "msg", Type: "string", Optional: true, Desc: "Optional message prefixed onto the thrown error; defaults to \"assert.equal failed\"."},
			},
			ReturnType: "void",
			Returns:    "void — returns nothing when the values match.",
			Errors:     "Throws an Error (\"<msg>: expected <expected>, got <actual>\") when the values are not equal.",
			Example:    `runtime.assert.equal(1 + 1, 2, "math still works");`,
		},
		"assert.ok": {
			Summary: "Throw when cond is falsy. Optional msg appears in the error.",
			Params: []scriptengine.Param{
				{Name: "cond", Type: "unknown", Desc: "A value tested for truthiness (JS coercion: 0, \"\", null, undefined, NaN and false are falsy)."},
				{Name: "msg", Type: "string", Optional: true, Desc: "Optional message used as the thrown error text; defaults to \"assert.ok failed\"."},
			},
			ReturnType: "void",
			Returns:    "void — returns nothing when cond is truthy.",
			Errors:     "Throws an Error carrying msg (or \"assert.ok failed\") when cond is null or coerces to false.",
			Example:    `runtime.assert.ok(user.id, "user must have an id");`,
		},
		"time.nowMs": {
			Summary:    "Wall-clock milliseconds since the Unix epoch.",
			ReturnType: "number",
			Returns:    "number — integer milliseconds since 1970-01-01T00:00:00Z (host wall clock).",
			Errors:     "Never throws.",
			Example:    `const t0 = runtime.time.nowMs();`,
		},
		"time.sleep": {
			Summary: "Resolve after `ms` milliseconds. Cancellable via the engine timeout.",
			Params: []scriptengine.Param{
				{Name: "ms", Type: "number", Desc: "Delay in milliseconds. Coerced to an integer; non-positive values resolve effectively immediately."},
			},
			ReturnType: "void",
			Returns:    "Promise<void> — resolves once the delay elapses.",
			Errors:     "Rejects if the run is cancelled or hits its timeout before the delay elapses (the underlying context is cancelled).",
			Example:    `await runtime.time.sleep(250);`,
		},
		"time.format": {
			Summary: "Format a unix-ms timestamp through strftime tokens. Optional IANA tz (e.g. 'Europe/Stockholm'); default is the host's local zone.",
			Params: []scriptengine.Param{
				{Name: "ms", Type: "number", Desc: "Milliseconds since the Unix epoch (e.g. from time.nowMs). Coerced to an integer."},
				{Name: "layout", Type: "string", Desc: "strftime-style layout. Supported tokens: %Y %y %m %d %H %M %S %T %F %j %A %a %B %b %z %Z and %% (literal percent). Unknown %X tokens pass through verbatim."},
				{Name: "tz", Type: "string", Optional: true, Desc: "IANA timezone name (e.g. \"Europe/Stockholm\", \"UTC\"). Defaults to the host's local zone when omitted/null/undefined."},
			},
			ReturnType: "string",
			Returns:    "string — the timestamp rendered in tz with the given layout.",
			Errors:     "Throws (\"time.format: ...\") if tz is not a loadable IANA timezone name.",
			Example:    `const s = runtime.time.format(runtime.time.nowMs(), "%F %T", "UTC");`,
		},
		"env.get": {
			Summary: "Read an environment variable. Returns undefined when unset (not empty string).",
			Params: []scriptengine.Param{
				{Name: "name", Type: "string", Desc: "Environment variable name to look up."},
			},
			ReturnType: "string | undefined",
			Returns:    "string — the variable's value, or undefined when the variable is not set. A variable set to the empty string returns \"\", which is distinct from undefined.",
			Errors:     "Never throws.",
			Example:    `const home = runtime.env.get("HOME") ?? "/tmp";`,
		},
		"argv": {
			Summary:    "Per-script argument vector: [programName, scriptPath, ...userArgs]. argv[0] is the program name (sercon), argv[1] is the running script path, and any args after `--` on the command line start at argv[2].",
			ReturnType: "string[]",
			Returns:    "string[] — the per-run argument vector. argv[0] is the program name (\"sercon\"), argv[1] is the running script's path, and entries from index 2 onward are the user arguments passed after `--` on the command line. This is a value (property), not a function.",
			Errors:     "Not callable — accessing it never throws; reading an out-of-range index yields undefined per normal array semantics.",
			Example:    `const target = runtime.argv[2] ?? "default-host";`,
		},
	}
}
