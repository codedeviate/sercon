package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func consoleDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"log": {
			Summary:    "Print a space-joined line of the arguments to stdout. Primitives print raw; objects/arrays render as JSON. Browser/Node-compatible; same output as runtime.log.",
			Params:     []scriptengine.Param{{Name: "args", Type: "...unknown[]", Desc: "Values to print, joined by single spaces and terminated with a newline. Primitives (string/number/boolean/null/undefined) print raw; objects and arrays render as JSON via JSON.stringify, falling back to String() for functions and circular references."}},
			ReturnType: "void",
			Returns:    "void — output is written to stdout as a side effect.",
			Errors:     "Never throws; values JSON cannot serialise degrade to their String() form.",
			Example:    `console.log("user", { id: 1, name: "ada" }); // user {"id":1,"name":"ada"}`,
		},
		"info": {
			Summary:    "Alias of console.log — stringified arguments, space-joined, to stdout.",
			Params:     []scriptengine.Param{{Name: "args", Type: "...unknown[]", Desc: "Values to print; identical formatting and stdout destination as console.log."}},
			ReturnType: "void",
			Returns:    "void — output is written to stdout as a side effect.",
			Errors:     "Never throws; values JSON cannot serialise degrade to their String() form.",
			Example:    `console.info("listening on", 8080);`,
		},
		"debug": {
			Summary:    "Alias of console.log — stringified arguments, space-joined, to stdout.",
			Params:     []scriptengine.Param{{Name: "args", Type: "...unknown[]", Desc: "Values to print; identical formatting and stdout destination as console.log."}},
			ReturnType: "void",
			Returns:    "void — output is written to stdout as a side effect.",
			Errors:     "Never throws; values JSON cannot serialise degrade to their String() form.",
			Example:    `console.debug("cache hit", key);`,
		},
		"warn": {
			Summary:    "Like console.log but writes to stderr.",
			Params:     []scriptengine.Param{{Name: "args", Type: "...unknown[]", Desc: "Values to print; same space-joined / JSON formatting as console.log but routed to stderr."}},
			ReturnType: "void",
			Returns:    "void — output is written to stderr as a side effect.",
			Errors:     "Never throws; values JSON cannot serialise degrade to their String() form.",
			Example:    `console.warn("retrying in", 5, "seconds");`,
		},
		"error": {
			Summary:    "Like console.log but writes to stderr.",
			Params:     []scriptengine.Param{{Name: "args", Type: "...unknown[]", Desc: "Values to print; same space-joined / JSON formatting as console.log but routed to stderr."}},
			ReturnType: "void",
			Returns:    "void — output is written to stderr as a side effect.",
			Errors:     "Never throws; values JSON cannot serialise degrade to their String() form.",
			Example:    `console.error("request failed", { status: 500 });`,
		},
		"table": {
			Summary: "Render tabular data as an aligned, bordered table on stdout (Node/Bun/Deno parity). Accepts an array of objects (rows), an array of primitives, or an object of objects/primitives. Prints a leading (index) column, one column per property (union of keys across rows, first-seen order), and a Values column for primitive rows. Non-tabular input (a primitive) falls back to console.log-style output without throwing.",
			Params: []scriptengine.Param{
				{Name: "data", Type: "unknown", Desc: "The rows to tabulate: an array (indices become the (index) column) or an object (keys become the (index) column). Cells are formatted like console.log — primitives raw, objects/arrays as compact JSON (strings are not quoted)."},
				{Name: "columns", Type: "string[]", Optional: true, Desc: "Restrict and order the property columns to exactly these names; an absent column renders as an empty column. The (index) column is always shown."},
			},
			ReturnType: "void",
			Returns:    "void — the table is written to stdout as a side effect.",
			Errors:     "Never throws; non-tabular input degrades to a console.log-style line.",
			Example:    `console.table([{ name: "web", status: "ok" }, { name: "db", status: "down" }]);`,
		},
	}
}
