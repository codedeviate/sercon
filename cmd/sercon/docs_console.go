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
	}
}
