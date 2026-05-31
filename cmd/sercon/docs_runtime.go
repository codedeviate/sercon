package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func runtimeDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"log":          {Summary: "Print one space-separated line of the arguments to stdout. Primitives print raw; objects/arrays render as JSON (circular refs fall back to [object Object]). The script-side equivalent of console.log."},
		"assert.equal": {Summary: "Throw when actual != expected (strict equality on primitives, deep equality on objects). Optional msg appears in the error."},
		"assert.ok":    {Summary: "Throw when cond is falsy. Optional msg appears in the error."},
		"time.nowMs":   {Summary: "Wall-clock milliseconds since the Unix epoch."},
		"time.sleep":   {Summary: "Resolve after `ms` milliseconds. Cancellable via the engine timeout."},
		"time.format":  {Summary: "Format a unix-ms timestamp through strftime tokens. Optional IANA tz (e.g. 'Europe/Stockholm'); default is the host's local zone."},
		"env.get":      {Summary: "Read an environment variable. Returns undefined when unset (not empty string)."},
		"argv":         {Summary: "Per-script argument vector: [programName, scriptPath, ...userArgs]. argv[0] is the program name (sercon), argv[1] is the running script path, and any args after `--` on the command line start at argv[2]."},
	}
}
