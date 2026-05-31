package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func consoleDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"log":   {Summary: "Print a space-joined line of the arguments to stdout. Primitives print raw; objects/arrays render as JSON. Browser/Node-compatible; same output as runtime.log."},
		"info":  {Summary: "Alias of console.log — stringified arguments, space-joined, to stdout."},
		"debug": {Summary: "Alias of console.log — stringified arguments, space-joined, to stdout."},
		"warn":  {Summary: "Like console.log but writes to stderr."},
		"error": {Summary: "Like console.log but writes to stderr."},
	}
}
