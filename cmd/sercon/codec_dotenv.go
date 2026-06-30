// cmd/sercon/codec_dotenv.go
package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dop251/goja"
)

// dotenvParse folds parseEnvFile output into a map (a later duplicate key
// overrides an earlier one).
func dotenvParse(text string) (map[string]string, error) {
	kvs, err := parseEnvFile([]byte(text))
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		out[kv.key] = kv.val
	}
	return out, nil
}

// dotenvValueString coerces a JS-exported value to its .env string form.
func dotenvValueString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case int64:
		return strconv.FormatInt(t, 10), nil
	case int:
		return strconv.Itoa(t), nil
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64), nil
	default:
		return "", fmt.Errorf("value must be a string, number, or boolean, got %T", v)
	}
}

// dotenvQuote wraps a value in double quotes when raw emission would not round
// trip through parseEnvFile (which TrimSpaces then strips one outer matching
// quote pair). Wrapping protects empty values, leading/trailing whitespace,
// spaces, '#'/'=', and values that themselves begin and end with a quote char.
func dotenvQuote(val string) string {
	needs := val == "" ||
		strings.ContainsAny(val, " \t#=") ||
		val != strings.TrimSpace(val) ||
		(len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0])
	if needs {
		return `"` + val + `"`
	}
	return val
}

// dotenvStringify serializes obj to .env text that round-trips through
// parseEnvFile. Errors on newline values and invalid keys. Keys are emitted in
// sorted order for deterministic output.
func dotenvStringify(obj map[string]any) (string, error) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		if k == "" || strings.ContainsAny(k, "= \t\r\n") || k != strings.TrimSpace(k) || strings.HasPrefix(k, "#") {
			return "", fmt.Errorf("invalid key %q", k)
		}
		val, err := dotenvValueString(obj[k])
		if err != nil {
			return "", fmt.Errorf("key %q: %w", k, err)
		}
		if strings.ContainsAny(val, "\r\n") {
			return "", fmt.Errorf("value for key %q contains a newline (not representable in .env)", k)
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(dotenvQuote(val))
		b.WriteByte('\n')
	}
	return b.String(), nil
}

// dotenvNamespace returns the codec.dotenv sub-namespace.
func dotenvNamespace(vm *goja.Runtime) map[string]any {
	throw := func(err error) goja.Value { panic(vm.NewGoError(err)) }
	return map[string]any{
		"parse": func(call goja.FunctionCall) goja.Value {
			s, ok := call.Argument(0).Export().(string)
			if !ok {
				panic(vm.NewTypeError("codec.dotenv.parse: text must be a string"))
			}
			m, err := dotenvParse(s)
			if err != nil {
				return throw(fmt.Errorf("codec.dotenv.parse: %w", err))
			}
			return vm.ToValue(m)
		},
		"stringify": func(call goja.FunctionCall) goja.Value {
			obj, ok := call.Argument(0).Export().(map[string]any)
			if !ok {
				panic(vm.NewTypeError("codec.dotenv.stringify: expected an object of string/number/boolean values"))
			}
			out, err := dotenvStringify(obj)
			if err != nil {
				return throw(fmt.Errorf("codec.dotenv.stringify: %w", err))
			}
			return vm.ToValue(out)
		},
	}
}
