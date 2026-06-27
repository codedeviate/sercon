// cmd/sercon/codec_toml.go
package main

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/pelletier/go-toml/v2"
)

// tomlNamespace wires codec.toml.* — TOML text ↔ value. parse maps a TOML
// document into a JS object; stringify marshals a JS value (must be an
// object/table at the top level) to TOML text. Both are synchronous and throw
// on error.
func tomlNamespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value { panic(vm.NewGoError(err)) }
	return map[string]any{
		"parse": func(call goja.FunctionCall) goja.Value {
			var m map[string]any
			if err := toml.Unmarshal([]byte(call.Argument(0).String()), &m); err != nil {
				return throw(fmt.Errorf("codec.toml.parse: %w", err))
			}
			return vm.ToValue(m)
		},
		"stringify": func(call goja.FunctionCall) goja.Value {
			out, err := toml.Marshal(call.Argument(0).Export())
			if err != nil {
				return throw(fmt.Errorf("codec.toml.stringify: %w", err))
			}
			return vm.ToValue(string(out))
		},
	}
}
