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
		"setDeadline": {
			Summary: "Set the running script's wall-clock kill deadline to now + ms (replacing any prior deadline; ms<=0 disables it). This is the same deadline the -timeout flag sets — NOT the JS global setTimeout (which schedules a callback). Use it to extend a long task or add a deadline to a `-timeout 0` run.",
			Params: []scriptengine.Param{
				{Name: "ms", Type: "number", Desc: "Milliseconds from now until the run is killed; <= 0 disables the deadline (same as clearDeadline())."},
			},
			ReturnType: "void",
			Returns:    "void — applies immediately to the in-flight run.",
			Errors:     "Does not throw; a non-numeric argument coerces to 0 (disable).",
			Example:    `runtime.setDeadline(30000); // give this run 30s from now`,
		},
		"clearDeadline": {
			Summary:    "Remove the running script's wall-clock kill deadline entirely (equivalent to -timeout 0 / setDeadline(0)). The script then runs without a timeout.",
			ReturnType: "void",
			Returns:    "void.",
			Errors:     "Does not throw.",
			Example:    `runtime.clearDeadline(); // run without a timeout`,
		},
		"getDeadline": {
			Summary:    "Return the milliseconds remaining until the running script's kill deadline, or null when no deadline is active (disabled or started with -timeout 0).",
			ReturnType: "number | null",
			Returns:    "number — ms remaining (>= 0) — or null when there is no active deadline.",
			Errors:     "Does not throw.",
			Example:    `const left = runtime.getDeadline(); // e.g. 9871, or null`,
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
		"secrets.available": {
			Summary:    "True when an OS keystore backend (macOS Keychain, Linux Secret Service, Windows Credential Manager) is plausibly reachable this run. Cheap advisory hint — does not touch the keystore; gate calls on it to self-skip on headless boxes.",
			ReturnType: "boolean",
			Returns:    "boolean — false on a host with no reachable keystore (e.g. a headless Linux box without a D-Bus session). The authoritative signal is whether get/set/delete throw.",
			Errors:     "Never throws.",
			Example:    `if (!runtime.secrets.available) runtime.log("no keystore — skipping");`,
		},
		"secrets.get": {
			Summary: "Read a string secret from the OS keystore. The keystore service is the configured prefix + name (default \"sercon/\"), so reads are confined to sercon's namespace. Async (keystore I/O).",
			Params: []scriptengine.Param{
				{Name: "name", Type: "string", Desc: "Secret name within the sercon prefix namespace (the keystore service is prefix+name, e.g. \"sercon/devshop\")."},
				{Name: "account", Type: "string", Desc: "Account/user the secret belongs to (may be an empty string for a single-secret name)."},
			},
			ReturnType: "Promise<string | null>",
			Returns:    "Promise resolving to the stored secret string, or null when no such item exists.",
			Errors:     "Rejects if name is missing/empty or account is missing (pass \"\" for a single-secret name), when the keystore backend is unreachable, or the read fails (a clean \"runtime.secrets.get: …\" error). Bounded by a 10s timeout (a blocking macOS consent prompt rejects rather than hangs).",
			Example:    `const pw = await runtime.secrets.get("devshop", "tess@example.com");`,
		},
		"secrets.set": {
			Summary: "Store or overwrite a string secret in the OS keystore under prefix + name / account. Async (keystore I/O).",
			Params: []scriptengine.Param{
				{Name: "name", Type: "string", Desc: "Secret name within the sercon prefix namespace (keystore service is prefix+name)."},
				{Name: "account", Type: "string", Desc: "Account/user the secret belongs to."},
				{Name: "secret", Type: "string", Desc: "The secret value to store."},
			},
			ReturnType: "Promise<void>",
			Returns:    "Promise resolving when the secret is written.",
			Errors:     "Rejects if name is missing/empty, account is missing (pass \"\" for a single-secret name), or secret is missing; when the keystore backend is unreachable; or the write fails (\"runtime.secrets.set: …\"). Bounded by a 10s timeout.",
			Example:    `await runtime.secrets.set("devshop", "tess@example.com", "hunter2");`,
		},
		"secrets.delete": {
			Summary: "Remove a secret from the OS keystore under prefix + name / account. Async (keystore I/O).",
			Params: []scriptengine.Param{
				{Name: "name", Type: "string", Desc: "Secret name within the sercon prefix namespace (keystore service is prefix+name)."},
				{Name: "account", Type: "string", Desc: "Account/user the secret belongs to."},
			},
			ReturnType: "Promise<boolean>",
			Returns:    "Promise resolving true when an item was removed, false when there was nothing to remove.",
			Errors:     "Rejects if name is missing/empty or account is missing (pass \"\" for a single-secret name), when the keystore backend is unreachable, or the delete fails (\"runtime.secrets.delete: …\"). Bounded by a 10s timeout.",
			Example:    `const removed = await runtime.secrets.delete("devshop", "tess@example.com");`,
		},
		"clipboard.available": {
			Summary:    "True when a host clipboard backend is on PATH (macOS pbcopy/pbpaste; Linux wl-clipboard or xclip/xsel; Windows clip + PowerShell). Cheap, side-effect-free advisory — does not touch the clipboard; gate calls on it to self-skip on headless boxes.",
			ReturnType: "boolean",
			Returns:    "boolean — false when no clipboard CLI is installed (e.g. a headless server). The authoritative signal is whether read/write throw.",
			Errors:     "Never throws.",
			Example:    `if (!runtime.clipboard.available) runtime.log("no clipboard — skipping");`,
		},
		"clipboard.read": {
			Summary:    "Read the host OS system clipboard as UTF-8 text. Async (shells out to the platform clipboard tool). An empty clipboard resolves with \"\".",
			ReturnType: "Promise<string>",
			Returns:    "Promise resolving to the clipboard text (\"\" when the clipboard is empty or holds no text).",
			Errors:     "Rejects when no clipboard backend is on PATH (a clean \"runtime.clipboard: no clipboard backend …\" message) or the underlying command fails / times out (~5s).",
			Example:    `const text = await runtime.clipboard.read();`,
		},
		"clipboard.write": {
			Summary: "Replace the host OS system clipboard with the given text. Async (shells out to the platform clipboard tool); text is passed via stdin (no shell-injection risk).",
			Params: []scriptengine.Param{
				{Name: "text", Type: "string", Desc: "The text to place on the clipboard (non-string values are String()-coerced)."},
			},
			ReturnType: "Promise<void>",
			Returns:    "Promise resolving when the clipboard has been set.",
			Errors:     "Rejects when no clipboard backend is on PATH or the underlying command fails / times out (~5s).",
			Example:    `await runtime.clipboard.write("copied from sercon");`,
		},
		"clipboard.imageAvailable": {
			Summary:    "True when a PNG image clipboard backend is usable on this host (macOS pngpaste; Linux wl-clipboard or xclip — not xsel; Windows PowerShell). Cheap advisory; does not touch the clipboard.",
			ReturnType: "boolean",
			Returns:    "boolean — false when no image backend is installed (e.g. macOS without pngpaste). Gate readImage/writeImage on it.",
			Errors:     "Never throws.",
			Example:    `if (runtime.clipboard.imageAvailable) { /* … */ }`,
		},
		"clipboard.readImage": {
			Summary:    "Read the host clipboard image as PNG bytes. Async (shells out). Resolves null when the clipboard holds no image.",
			ReturnType: "Promise<Uint8Array | null>",
			Returns:    "Promise resolving to the clipboard image as PNG bytes, or null when no image is present.",
			Errors:     "Rejects when no image backend is available (see imageAvailable) or the underlying command fails / times out (~5s).",
			Example:    `const png = await runtime.clipboard.readImage();`,
		},
		"clipboard.writeImage": {
			Summary: "Set the host clipboard image from PNG bytes. Async (shells out). The input must be a PNG (validated by signature) — other formats reject.",
			Params: []scriptengine.Param{
				{Name: "png", Type: "Uint8Array", Desc: "PNG image bytes (must begin with the PNG signature)."},
			},
			ReturnType: "Promise<void>",
			Returns:    "Promise resolving when the clipboard image is set.",
			Errors:     "Rejects when the data is not a PNG, no image backend is available, or the command fails / times out (~5s).",
			Example:    `await runtime.clipboard.writeImage(pngBytes);`,
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
