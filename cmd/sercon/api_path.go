package main

import (
	"path"
	"strings"

	"github.com/dop251/goja"
)

// pathNamespace returns the `api.path.*` member map. Semantics are POSIX
// (forward slashes); on Windows pass already-normalised paths or convert
// separators yourself.
func pathNamespace(vm *goja.Runtime) map[string]any {
	requireString := func(label string, v goja.Value) string {
		if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
			panic(vm.NewTypeError(label + ": expected a string"))
		}
		return v.String()
	}
	return map[string]any{
		"dirname": func(call goja.FunctionCall) goja.Value {
			p := requireString("dirname", call.Argument(0))
			return vm.ToValue(path.Dir(p))
		},
		"basename": func(call goja.FunctionCall) goja.Value {
			p := requireString("basename", call.Argument(0))
			b := path.Base(p)
			if v := call.Argument(1); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
				suffix := v.String()
				if suffix != "" && strings.HasSuffix(b, suffix) && b != suffix {
					b = b[:len(b)-len(suffix)]
				}
			}
			return vm.ToValue(b)
		},
	}
}
