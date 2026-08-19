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
			ReturnType: "Promise<void>",
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
		"env.load": {
			Summary: "Load a .env file and apply it to the process environment, so subsequent runtime.env.get (and any spawned subprocess) see the values. Parses KEY=VALUE lines (# comments, blank lines, optional `export `, surrounding quotes stripped; no shell expansion). An already-set variable is left untouched unless opts.override is true. Async; resolves to the parsed pairs.",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Path to the .env file."},
				{Name: "opts", Type: "{ override?: boolean }", Optional: true, Desc: "override: overwrite variables already present in the environment (default false — existing values win)."},
			},
			ReturnType: "Promise<{ [key: string]: string }>",
			Returns:    "A promise resolving to the parsed key/value pairs from the file (all of them, regardless of whether each was applied).",
			Errors:     "Rejects if the file is missing or unreadable, or if a line is malformed (\"line N: …\"). Throws a TypeError if path is not a string.",
			Example:    "await runtime.env.load(\".env\");\nconst url = runtime.env.get(\"DATABASE_URL\");",
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
		"termSize": {
			Summary:    "Current terminal size of the controlling TTY (stdout) as { columns, rows, tty }. Synchronous (a single ioctl). When stdout is not a terminal (piped/redirected) tty is false and columns/rows fall back to $COLUMNS/$LINES, then 80x24 — so scripts can format tables/progress bars without special-casing the non-TTY path.",
			ReturnType: "{ columns: number; rows: number; tty: boolean }",
			Returns:    "{ columns, rows, tty } — terminal width/height in character cells; tty is true only when stdout is a real terminal (otherwise the values are the $COLUMNS/$LINES-or-80x24 fallback).",
			Errors:     "Never throws; a non-terminal stdout yields the fallback with tty:false.",
			Example:    `const { columns } = runtime.termSize(); const bar = "=".repeat(Math.min(columns, 40));`,
		},
		"open": {
			Summary: "Open a URL or file path in the OS default handler (the user's normal GUI browser for URLs) via the platform opener — macOS `open`, Linux `xdg-open`/`gnome-open`, Windows rundll32/`start` — feature-detected on PATH. Fire-and-forget: resolves once the opener is spawned, not when the browser closes. The target is passed as a single argument with no shell, so special characters can't inject.",
			Params: []scriptengine.Param{
				{Name: "target", Type: "string", Desc: "A URL or file path to hand to the OS default handler. Passed as one argument with no shell interpolation; must be non-empty."},
			},
			ReturnType: "Promise<void>",
			Returns:    "Promise<void> — resolves once the opener process has been spawned (the browser's lifetime is not awaited).",
			Errors:     "Throws (\"runtime.open: …\") if target is missing/empty, if no OS opener is found on PATH (see runtime.openAvailable), or if the opener fails to start.",
			Example:    `await runtime.open("https://example.com");`,
		},
		"openAvailable": {
			Summary:    "Whether an OS opener is available on PATH — an advisory for runtime.open. True when the platform opener (open / xdg-open / gnome-open / rundll32) is found. A value (property), not a function.",
			ReturnType: "boolean",
			Returns:    "boolean — true if runtime.open can launch a handler on this host; false otherwise (in which case runtime.open throws).",
			Errors:     "Not callable — accessing it never throws.",
			Example:    `if (runtime.openAvailable) await runtime.open(url);`,
		},

		// runtime.stdout / runtime.stderr — script-controlled stdio redirection.
		// The seven members are identical in shape between the two streams;
		// each entry below describes its own stream by name. A stream-name
		// target ("stdout"/"stderr") always folds onto the real PROCESS
		// stream, never onto whatever the other handle currently has pushed —
		// that is what makes a stdout<->stderr cycle impossible by
		// construction. Covers console.*, runtime.log, the default-export
		// JSON, PASS/FAIL, --verbose and the TUI non-TTY fallback. A child
		// process is out of scope for a different reason, not a uniform one:
		// services.exec.shell/.run, services.git, services.gh and
		// services.typst.* all CAPTURE the child's output into a buffer
		// handed back to the script, so the child never touches sercon's own
		// streams — redirection is simply irrelevant to it, though a script
		// that then prints the captured string itself has that print follow
		// the redirect like any other write. Only services.exec.interactive
		// genuinely bypasses redirection, by inheriting fd 1/2 (and, on the
		// input side, fd 0) directly.
		"stdout.to": {
			Summary: "Point stdout at a target and return a restore function. The target is \"stdout\"/\"stderr\" (fold onto that process stream), \"null\" (discard), { file, append? } (a file), or a function (called with each completed line). Redirects nest: the restore pops its own entry, so nested and out-of-order restores both behave, and calling it twice is a no-op.",
			Params: []scriptengine.Param{
				{Name: "target", Type: `"stdout" | "stderr" | "null" | { file: string, append?: boolean } | ((line: string) => void)`, Desc: "Where the stream should go. A stream name folds onto that PROCESS stream (so stdout→stderr and stderr→stdout cannot ping-pong); \"null\" discards; an object writes to a file, truncating unless append is true; a function receives each completed line without its newline, delivered on the next tick in write order."},
				{Name: "opts", Type: "{ tee?: boolean }", Optional: true, Desc: "tee also writes to the destination beneath this one in the stack — so teeing on top of a silence() still writes only to the new target. tee is rejected with the \"null\" target."},
			},
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that removes this redirect.",
			Errors:     "Throws on an unknown target name, an object target without a `file` property, a file that cannot be opened, or { tee: true } combined with the \"null\" target. A later write failure does NOT throw: the destination fails over to the process stream and one warning is printed on the real stderr. A function target has its own delivery caveats, none of which throw: the pending-line queue is bounded (1024 lines) — on overflow, or on a write that re-enters the handler (e.g. the handler's own console.log while it runs), the write falls through to the destination beneath instead of blocking or being lost twice; any line still queued when the redirect is popped or the run ends is written to the destination beneath rather than delivered; and a handler that throws has that one line reported once on the real stderr (not retried, not recovered) without aborting the rest of the run.",
			Example:    "const restore = runtime.stdout.to({ file: \"/tmp/out.log\" }, { tee: true });\nconsole.log(\"goes to both\");\nrestore();",
		},
		"stdout.toFile": {
			Summary: "Redirect stdout to a file, truncating unless append is true, and return a restore function. Shorthand for stdout.to({ file, append }, { tee }).",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Output file path. Opened immediately, at the call site — a bad path or a permission error surfaces here rather than at some later write."},
				{Name: "opts", Type: "{ append?: boolean, tee?: boolean }", Optional: true, Desc: "append opens with O_APPEND instead of truncating (default false). tee also writes to whatever destination was beneath this one when it was pushed."},
			},
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that closes the file and removes this redirect.",
			Errors:     "Throws (\"toFile: …\") if the file cannot be opened (missing parent directory, permission denied, etc). A later write failure does NOT throw: the destination fails over to the process stream and one warning is printed on the real stderr.",
			Example:    "const stop = runtime.stdout.toFile(\"/tmp/out.log\", { append: true, tee: true });\nconsole.log(\"on screen and in the file\");\nstop();",
		},
		"stdout.silence": {
			Summary:    "Discard everything written to stdout until the returned restore function is called. Shorthand for stdout.to(\"null\").",
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that removes the silence and reveals whatever was beneath it.",
			Errors:     "Never throws.",
			Example:    "const unsilence = runtime.stdout.silence();\nconsole.log(\"nobody sees this\");\nunsilence();",
		},
		"stdout.reset": {
			Summary:    "Drop every redirect this script has pushed onto stdout — the whole stack, not just the last push — closing any files they opened and reverting to the real process stdout. Called automatically at the start of every Run (including each script in `sercon a.ts b.ts` and each --watch re-run), so a script never inherits a previous script's redirect.",
			ReturnType: "void",
			Returns:    "void.",
			Errors:     "Never throws.",
			Example:    "runtime.stdout.to(\"null\");\nrunUntrusted();\nruntime.stdout.reset(); // back to the real stdout, however deep the stack got",
		},
		"stdout.target": {
			Summary:    "Inspect the effective stdout destination without changing it.",
			ReturnType: `{ kind: "stream" | "null" | "file" | "callback" | "buffer"; tee: boolean; depth: number; name?: "stdout" | "stderr"; path?: string; append?: boolean }`,
			Returns:    `The current top-of-stack destination: kind identifies its type (name is set only for kind "stream"; path/append only for kind "file"); tee reports whether it also writes to the destination beneath it; depth is how many redirects are currently pushed (0 means stdout is unredirected).`,
			Errors:     "Never throws.",
			Example:    `if (runtime.stdout.target().kind === "null") console.log("currently silenced");`,
		},
		"stdout.scoped": {
			Summary: "Apply a target to stdout for the duration of fn (sync or async), then restore — even if fn throws or its returned promise rejects. Two call shapes: scoped(target, fn) or scoped(target, opts, fn).",
			Params: []scriptengine.Param{
				{Name: "target", Type: `"stdout" | "stderr" | "null" | { file: string, append?: boolean } | ((line: string) => void)`, Desc: "Same target union as `to`."},
				{Name: "opts", Type: "{ tee?: boolean }", Optional: true, Desc: "Same as `to`'s opts; only meaningful in the three-argument form."},
				{Name: "fn", Type: "() => void | Promise<void>", Desc: "Called with no arguments. Its own return value is discarded — scoped always resolves to undefined."},
			},
			ReturnType: "Promise<void>",
			Returns:    "Promise<void> — resolves once fn (and any promise it returns) settles. The redirect has already been restored by the time this resolves.",
			Errors:     "Rejects with whatever fn threw or its promise rejected with, after restoring the redirect. Throws synchronously (before touching the stream) for the same reasons as `to`, or if the last argument is not a function.",
			Example:    "await runtime.stdout.scoped(\"null\", () => {\n  console.log(\"never printed\");\n});\nconsole.log(\"back to normal\");",
		},
		"stdout.capture": {
			Summary: "Run fn (sync or async) with stdout captured to an in-memory buffer, and resolve to everything it wrote. Always exclusive — unlike `to`/`scoped`, capture never tees; use scoped with { tee: true } if the terminal should also see the output.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "() => void | Promise<void>", Desc: "Called with no arguments; its own return value is ignored."},
			},
			ReturnType: "Promise<string>",
			Returns:    "Promise<string> — everything written to stdout while fn ran, in write order. The redirect has already been restored by the time this resolves.",
			Errors:     "Rejects with whatever fn threw or its promise rejected with, after restoring the redirect. Throws synchronously if the argument is not a function.",
			Example:    "const out = await runtime.stdout.capture(() => {\n  console.log(\"one\");\n  console.log(\"two\");\n});\nruntime.assert.equal(out, \"one\\ntwo\\n\");",
		},

		"stderr.to": {
			Summary: "Point stderr at a target and return a restore function. The target is \"stdout\"/\"stderr\" (fold onto that process stream), \"null\" (discard), { file, append? } (a file), or a function (called with each completed line). Redirects nest: the restore pops its own entry, so nested and out-of-order restores both behave, and calling it twice is a no-op.",
			Params: []scriptengine.Param{
				{Name: "target", Type: `"stdout" | "stderr" | "null" | { file: string, append?: boolean } | ((line: string) => void)`, Desc: "Where the stream should go. \"stdout\" folds stderr onto the PROCESS stdout (so stdout→stderr and stderr→stdout cannot ping-pong); \"null\" discards; an object writes to a file, truncating unless append is true; a function receives each completed line without its newline, delivered on the next tick in write order."},
				{Name: "opts", Type: "{ tee?: boolean }", Optional: true, Desc: "tee also writes to the destination beneath this one in the stack — so teeing on top of a silence() still writes only to the new target. tee is rejected with the \"null\" target."},
			},
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that removes this redirect.",
			Errors:     "Throws on an unknown target name, an object target without a `file` property, a file that cannot be opened, or { tee: true } combined with the \"null\" target. A later write failure does NOT throw: the destination fails over to the process stream and one warning is printed on the real stderr. A function target has its own delivery caveats, none of which throw: the pending-line queue is bounded (1024 lines) — on overflow, or on a write that re-enters the handler (e.g. the handler's own console.error while it runs), the write falls through to the destination beneath instead of blocking or being lost twice; any line still queued when the redirect is popped or the run ends is written to the destination beneath rather than delivered; and a handler that throws has that one line reported once on the real stderr (not retried, not recovered) without aborting the rest of the run.",
			Example:    "const restore = runtime.stderr.to(\"stdout\");\nconsole.error(\"now on stdout too\");\nrestore();",
		},
		"stderr.toFile": {
			Summary: "Redirect stderr to a file, truncating unless append is true, and return a restore function. Shorthand for stderr.to({ file, append }, { tee }).",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Output file path. Opened immediately, at the call site — a bad path or a permission error surfaces here rather than at some later write."},
				{Name: "opts", Type: "{ append?: boolean, tee?: boolean }", Optional: true, Desc: "append opens with O_APPEND instead of truncating (default false). tee also writes to whatever destination was beneath this one when it was pushed."},
			},
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that closes the file and removes this redirect.",
			Errors:     "Throws (\"toFile: …\") if the file cannot be opened (missing parent directory, permission denied, etc). A later write failure does NOT throw: the destination fails over to the process stream and one warning is printed on the real stderr.",
			Example:    "const stop = runtime.stderr.toFile(\"/tmp/err.log\", { append: true, tee: true });\nconsole.error(\"on screen and in the file\");\nstop();",
		},
		"stderr.silence": {
			Summary:    "Discard everything written to stderr until the returned restore function is called. Shorthand for stderr.to(\"null\").",
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that removes the silence and reveals whatever was beneath it.",
			Errors:     "Never throws.",
			Example:    "const unsilence = runtime.stderr.silence();\nconsole.error(\"nobody sees this\");\nunsilence();",
		},
		"stderr.reset": {
			Summary:    "Drop every redirect this script has pushed onto stderr — the whole stack, not just the last push — closing any files they opened and reverting to the real process stderr. Called automatically at the start of every Run (including each script in `sercon a.ts b.ts` and each --watch re-run), so a script never inherits a previous script's redirect.",
			ReturnType: "void",
			Returns:    "void.",
			Errors:     "Never throws.",
			Example:    "runtime.stderr.to(\"null\");\nrunUntrusted();\nruntime.stderr.reset(); // back to the real stderr, however deep the stack got",
		},
		"stderr.target": {
			Summary:    "Inspect the effective stderr destination without changing it.",
			ReturnType: `{ kind: "stream" | "null" | "file" | "callback" | "buffer"; tee: boolean; depth: number; name?: "stdout" | "stderr"; path?: string; append?: boolean }`,
			Returns:    `The current top-of-stack destination: kind identifies its type (name is set only for kind "stream"; path/append only for kind "file"); tee reports whether it also writes to the destination beneath it; depth is how many redirects are currently pushed (0 means stderr is unredirected).`,
			Errors:     "Never throws.",
			Example:    `if (runtime.stderr.target().kind === "null") console.error("currently silenced");`,
		},
		"stderr.scoped": {
			Summary: "Apply a target to stderr for the duration of fn (sync or async), then restore — even if fn throws or its returned promise rejects. Two call shapes: scoped(target, fn) or scoped(target, opts, fn).",
			Params: []scriptengine.Param{
				{Name: "target", Type: `"stdout" | "stderr" | "null" | { file: string, append?: boolean } | ((line: string) => void)`, Desc: "Same target union as `to`."},
				{Name: "opts", Type: "{ tee?: boolean }", Optional: true, Desc: "Same as `to`'s opts; only meaningful in the three-argument form."},
				{Name: "fn", Type: "() => void | Promise<void>", Desc: "Called with no arguments. Its own return value is discarded — scoped always resolves to undefined."},
			},
			ReturnType: "Promise<void>",
			Returns:    "Promise<void> — resolves once fn (and any promise it returns) settles. The redirect has already been restored by the time this resolves.",
			Errors:     "Rejects with whatever fn threw or its promise rejected with, after restoring the redirect. Throws synchronously (before touching the stream) for the same reasons as `to`, or if the last argument is not a function.",
			Example:    "await runtime.stderr.scoped(\"null\", () => {\n  console.error(\"never printed\");\n});\nconsole.error(\"back to normal\");",
		},
		"stderr.capture": {
			Summary: "Run fn (sync or async) with stderr captured to an in-memory buffer, and resolve to everything it wrote. Always exclusive — unlike `to`/`scoped`, capture never tees; use scoped with { tee: true } if the terminal should also see the output.",
			Params: []scriptengine.Param{
				{Name: "fn", Type: "() => void | Promise<void>", Desc: "Called with no arguments; its own return value is ignored."},
			},
			ReturnType: "Promise<string>",
			Returns:    "Promise<string> — everything written to stderr while fn ran, in write order. The redirect has already been restored by the time this resolves.",
			Errors:     "Rejects with whatever fn threw or its promise rejected with, after restoring the redirect. Throws synchronously if the argument is not a function.",
			Example:    "const out = await runtime.stderr.capture(() => {\n  console.error(\"one\");\n  console.error(\"two\");\n});\nruntime.assert.equal(out, \"one\\ntwo\\n\");",
		},

		// runtime.stdin — a swappable, readable input source. read/readBytes/
		// readLine/lines drain the CURRENT source (the real process stdin
		// unless from/fromFile/fromString has pushed something else). Reads
		// off different members serialise against each other (one shared read
		// lock), so two concurrent reads can never split a line or a chunk.
		"stdin.read": {
			Summary:    "Read every remaining byte from the current stdin source as a UTF-8 string, blocking until EOF. Concurrent calls with readBytes/readLine/lines serialise, so two readers can never split a read.",
			ReturnType: "Promise<string>",
			Returns:    "Promise<string> — the rest of the source's bytes, decoded as UTF-8, from the current position through EOF.",
			Errors:     "Rejects if the underlying source returns a read error. Blocking on the real process stdin cannot be interrupted by the run's deadline — the run itself is still killed, but this call parks until the pipe closes; a file or string source always reaches EOF. Note: when the script itself came from stdin (`sercon -`), stdin is already drained and this resolves to \"\" immediately.",
			Example:    "const body = await runtime.stdin.read();\nruntime.log(\"got\", body.length, \"bytes\");",
		},
		"stdin.readBytes": {
			Summary:    "Read every remaining byte from the current stdin source as raw bytes, blocking until EOF. Concurrent calls with read/readLine/lines serialise, so two readers can never split a read.",
			ReturnType: "Promise<Uint8Array>",
			Returns:    "Promise<Uint8Array> — the rest of the source's bytes from the current position through EOF. This is goja's Go-slice-backed wrapper around the underlying []byte, not a native JS Uint8Array: `instanceof Uint8Array` is false, though .length and indexing work as expected.",
			Errors:     "Rejects if the underlying source returns a read error. Blocking on the real process stdin cannot be interrupted by the run's deadline — the run itself is still killed, but this call parks until the pipe closes. Note: when the script itself came from stdin (`sercon -`), stdin is already drained and this resolves to a zero-length result immediately.",
			Example:    "const b = await runtime.stdin.readBytes();\nruntime.log(b.length, b[0]);",
		},
		"stdin.readLine": {
			Summary:    "Read one line from the current stdin source, without its trailing newline. Resolves to null at EOF. A final line with no trailing newline is still returned. Concurrent calls serialise, so two readers can never split a line.",
			ReturnType: "Promise<string | null>",
			Returns:    "Promise<string | null> — the next line without its newline, or null once the source is exhausted.",
			Errors:     "Rejects if the underlying source returns a read error. Note: when the script itself came from stdin (`sercon -`), stdin is already drained and this resolves to null immediately.",
			Example:    "let line;\nwhile ((line = await runtime.stdin.readLine()) !== null) {\n  runtime.log(\"got\", line);\n}",
		},
		"stdin.lines": {
			Summary:    "Async-iterate the current stdin source one line at a time (no trailing newline), stopping at EOF. Equivalent to calling readLine() in a loop; `for await` is just more idiomatic.",
			ReturnType: "AsyncIterable<string>",
			Returns:    "An async iterator yielding each line as a string. `for await (const line of runtime.stdin.lines())`; `break` simply stops calling readLine again — it does not close or reset the source.",
			Errors:     "The iterator's next() rejects if the underlying source returns a read error, propagating out of the `for await`.",
			Example:    "for await (const line of runtime.stdin.lines()) {\n  runtime.log(line.toUpperCase());\n}",
		},
		"stdin.from": {
			Summary: "Push a new stdin source and return a restore function that pops it back off. source is { file: string } (opened immediately), { text: string } (an in-memory string), or \"stdin\" (the real process stdin, pushed as a new entry above whatever is currently active).",
			Params: []scriptengine.Param{
				{Name: "source", Type: `{ file: string } | { text: string } | "stdin"`, Desc: "Which source becomes active. A { file } is opened immediately, at the call site — a missing file or a permission error surfaces here. { text } and \"stdin\" cannot fail to open."},
			},
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that closes the file (if any) and pops this source back off, uncovering whatever was active before.",
			Errors:     "Throws (\"from: …\") if source is missing, is an object with neither `file` nor `text`, is an unrecognised string, or (for a { file } source) the file cannot be opened.",
			Example:    "const restore = runtime.stdin.from({ text: \"a\\nb\\n\" });\nconst first = await runtime.stdin.readLine();\nrestore();",
		},
		"stdin.fromFile": {
			Summary: "Push a file as the stdin source and return a restore function. Shorthand for stdin.from({ file: path }).",
			Params: []scriptengine.Param{
				{Name: "path", Type: "string", Desc: "Path to the file to read from. Opened immediately, at the call site."},
			},
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that closes the file and pops this source back off.",
			Errors:     "Throws (\"fromFile: …\") if the file cannot be opened (missing, permission denied).",
			Example:    "const restore = runtime.stdin.fromFile(\"fixtures/input.txt\");\nconst all = await runtime.stdin.read();\nrestore();",
		},
		"stdin.fromString": {
			Summary: "Push an in-memory string as the stdin source and return a restore function. Shorthand for stdin.from({ text }).",
			Params: []scriptengine.Param{
				{Name: "text", Type: "string", Desc: "Content to serve as stdin from now on."},
			},
			ReturnType: "() => void",
			Returns:    "() => void — an idempotent restore function that pops this source back off.",
			Errors:     "Never throws.",
			Example:    "runtime.stdin.fromString(\"alpha\\nbeta\\n\");\nfor await (const line of runtime.stdin.lines()) runtime.log(line);",
		},
		"stdin.reset": {
			Summary:    "Drop every source swap this script has pushed onto stdin — the whole stack, not just the last push — closing any files they opened and reverting to the real process stdin. Called automatically at the start of every Run, same as the stdout/stderr reset.",
			ReturnType: "void",
			Returns:    "void.",
			Errors:     "Never throws.",
			Example:    "runtime.stdin.fromString(\"test input\\n\");\n// ... read it ...\nruntime.stdin.reset(); // back to the real stdin",
		},
		"stdin.source": {
			Summary:    "Describe the currently active stdin source, without reading from it.",
			ReturnType: `{ kind: "stdin" | "file" | "text"; path?: string; tty: boolean }`,
			Returns:    `{ kind, path?, tty } — kind is which source is active (path is set only for kind "file"); tty is true only when kind is "stdin" and the real process stdin is a terminal — a file or string source is never a terminal, and a script itself read from stdin (` + "`sercon -`" + `) leaves the real stdin already drained.`,
			Errors:     "Never throws.",
			Example:    `if (runtime.stdin.source().tty) console.log("waiting for interactive input…");`,
		},
		"stdin.scoped": {
			Summary: "Push a stdin source for the duration of fn (sync or async), then restore — even if fn throws or its returned promise rejects. Unlike the stdout/stderr scoped (which always resolves to undefined), this resolves to fn's own resolved value.",
			Params: []scriptengine.Param{
				{Name: "source", Type: `{ file: string } | { text: string } | "stdin"`, Desc: "Same source union as `from`."},
				{Name: "fn", Type: "() => unknown | Promise<unknown>", Desc: "Called with no arguments; its resolved value becomes scoped's own resolved value."},
			},
			ReturnType: "Promise<unknown>",
			Returns:    "Promise<unknown> — resolves to whatever fn returned (or its promise resolved with), after the source has already been restored.",
			Errors:     "Rejects with whatever fn threw or its promise rejected with, after restoring the source. Throws synchronously (before touching the stack) for the same reasons as `from`, or if the second argument is not a function.",
			Example:    "const total = await runtime.stdin.scoped({ text: \"1\\n2\\n3\\n\" }, async () => {\n  let sum = 0;\n  for await (const line of runtime.stdin.lines()) sum += Number(line);\n  return sum;\n});",
		},
	}
}
