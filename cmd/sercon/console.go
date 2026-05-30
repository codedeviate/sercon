package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dop251/goja"
)

// consoleOut / consoleErr are the destinations for the console shim. They're
// package vars so tests can capture the routed output; production leaves them
// as the process streams.
var (
	consoleOut io.Writer = os.Stdout
	consoleErr io.Writer = os.Stderr
)

// consoleNamespace wires the `console` global — a browser/Node-style shim so
// scripts pasted from those environments run unchanged. `log` / `info` /
// `debug` print a clean, space-joined line to stdout (matching runtime.log);
// `warn` / `error` go to stderr. This deliberately replaces goja_nodejs's
// default console (which routes everything through Go's logger — timestamped
// and all on stderr): the CLI disables the engine console via
// Options.DisableConsole so this stream-correct, prefix-free one is the only
// `console` a script sees.
func consoleNamespace() map[string]any {
	line := func(call goja.FunctionCall) string {
		parts := make([]string, 0, len(call.Arguments))
		for _, a := range call.Arguments {
			parts = append(parts, a.String())
		}
		return strings.Join(parts, " ")
	}
	toStdout := func(call goja.FunctionCall) goja.Value {
		fmt.Fprintln(consoleOut, line(call))
		return goja.Undefined()
	}
	toStderr := func(call goja.FunctionCall) goja.Value {
		fmt.Fprintln(consoleErr, line(call))
		return goja.Undefined()
	}
	return map[string]any{
		"log":   toStdout,
		"info":  toStdout,
		"debug": toStdout,
		"warn":  toStderr,
		"error": toStderr,
	}
}
