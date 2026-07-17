// cmd/sercon/codec_yaml.go
package main

import (
	"fmt"

	"github.com/dop251/goja"
	yaml "go.yaml.in/yaml/v3"
)

// yamlNamespace wires codec.yaml.* — YAML text ↔ value. parse maps a YAML
// document into a JS value (mappings become objects, sequences arrays, scalars
// the matching JS primitive); stringify marshals a JS value to YAML text. Both
// are synchronous and throw on error, mirroring codec.toml.
//
// Only the first document of a multi-document stream ("---"-separated) is
// parsed — yaml.Unmarshal decodes a single document. This is a documented v1
// limitation.
func yamlNamespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value { panic(vm.NewGoError(err)) }
	return map[string]any{
		"parse": func(call goja.FunctionCall) goja.Value {
			// Decode into `any` (not map) so top-level scalars and sequences
			// parse too. yaml.v3 decodes mappings with string keys into
			// map[string]any, which goja exposes as a plain object; non-string
			// keys are coerced to their string form (see coerceYAMLKeys).
			var v any
			if err := yaml.Unmarshal([]byte(call.Argument(0).String()), &v); err != nil {
				return throw(fmt.Errorf("codec.yaml.parse: %w", err))
			}
			return vm.ToValue(coerceYAMLKeys(v))
		},
		"stringify": func(call goja.FunctionCall) goja.Value {
			out, err := yaml.Marshal(call.Argument(0).Export())
			if err != nil {
				return throw(fmt.Errorf("codec.yaml.stringify: %w", err))
			}
			return vm.ToValue(string(out))
		},
	}
}

// coerceYAMLKeys walks a decoded YAML value and rewrites any non-string map
// keys to their string form. yaml.v3 decodes a mapping whose keys are all
// strings into map[string]any (which goja turns into a plain object), but a
// mapping with a non-string key (e.g. `1: a`, `true: b`) decodes into
// map[string]any only for the string-keyed entries and map[any]any otherwise —
// goja can't expose a map[any]any as an object. Converting those keys to
// strings keeps every mapping reachable from JS as an object.
func coerceYAMLKeys(v any) any {
	switch m := v.(type) {
	case map[string]any:
		for k, val := range m {
			m[k] = coerceYAMLKeys(val)
		}
		return m
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			out[fmt.Sprintf("%v", k)] = coerceYAMLKeys(val)
		}
		return out
	case []any:
		for i, val := range m {
			m[i] = coerceYAMLKeys(val)
		}
		return m
	default:
		return v
	}
}
