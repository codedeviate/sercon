package main

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// consoleNamespace wires the `console` global — a browser/Node-style shim so
// scripts pasted from those environments run unchanged. `log` / `info` /
// `debug` print a clean, space-joined line to stdout (matching runtime.log);
// `warn` / `error` go to stderr. This deliberately replaces goja_nodejs's
// default console (which routes everything through Go's logger — timestamped
// and all on stderr): the CLI disables the engine console via
// Options.DisableConsole so this stream-correct, prefix-free one is the only
// `console` a script sees.
func consoleNamespace(vm *goja.Runtime) map[string]any {
	line := func(call goja.FunctionCall) string {
		parts := make([]string, 0, len(call.Arguments))
		for _, a := range call.Arguments {
			parts = append(parts, formatValue(vm, a))
		}
		return strings.Join(parts, " ")
	}
	toStdout := func(call goja.FunctionCall) goja.Value {
		fmt.Fprintln(stdioOut(), line(call))
		return goja.Undefined()
	}
	toStderr := func(call goja.FunctionCall) goja.Value {
		fmt.Fprintln(stdioErr(), line(call))
		return goja.Undefined()
	}
	table := func(call goja.FunctionCall) goja.Value {
		var columns []string
		if arg := call.Argument(1); arg != nil && !goja.IsUndefined(arg) && !goja.IsNull(arg) {
			if arr, ok := arg.Export().([]any); ok {
				columns = []string{} // non-nil: an explicit (even empty) restriction
				for _, it := range arr {
					if s, ok := it.(string); ok {
						columns = append(columns, s)
					}
				}
			}
		}
		if out, ok := consoleTableString(vm, call.Argument(0), columns); ok {
			fmt.Fprintln(stdioOut(), out)
		} else {
			// Non-tabular input (a primitive): behave like console.log(data).
			fmt.Fprintln(stdioOut(), formatValue(vm, call.Argument(0)))
		}
		return goja.Undefined()
	}
	return map[string]any{
		"log":   toStdout,
		"info":  toStdout,
		"debug": toStdout,
		"warn":  toStderr,
		"error": toStderr,
		"table": table,
	}
}

// formatValue renders a value for console.* / runtime.log. Primitives
// (including strings) print raw via String(); objects and arrays print as JSON
// (browser/Node-style expansion) via the runtime's JSON.stringify, falling
// back to String() for values JSON can't represent — functions (stringify
// yields undefined) and circular references (stringify throws). So
// console.log({a:1}) shows `{"a":1}` instead of `[object Object]`, while a
// cycle degrades to `[object Object]` rather than crashing.
func formatValue(vm *goja.Runtime, v goja.Value) string {
	if v == nil {
		return "undefined"
	}
	if _, isObj := v.(*goja.Object); !isObj {
		return v.String() // string / number / bool / null / undefined
	}
	if jsonObj := vm.Get("JSON"); jsonObj != nil {
		if fn, ok := goja.AssertFunction(jsonObj.ToObject(vm).Get("stringify")); ok {
			if res, err := fn(goja.Undefined(), v); err == nil && res != nil && !goja.IsUndefined(res) {
				return res.String()
			}
		}
	}
	return v.String()
}
