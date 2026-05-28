package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/itchyny/gojq"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// jqNamespace wires `api.text.jq.*`. The two members differ only in how much of
// the iterator they consume: `query` stops after the first result;
// `queryAll` drains the iterator. Both share the same parse / run plumbing
// via runJqQuery.
//
// Input is whatever JS hands over via `.Export()` — typically a
// `map[string]any` / `[]any` tree produced by `JSON.parse`. gojq's runtime
// works directly on those Go-side types, so most scripts can hand in a
// literal JS object without any pre-conversion.
func jqNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"query":    scriptengine.PromisifyAsync(vm, loop, jqQueryFirst),
		"queryAll": scriptengine.PromisifyAsync(vm, loop, jqQueryAll),
	}
}

// jqQueryFirst runs the filter and returns the first emitted value. When the
// filter yields nothing (e.g. `.does.not.exist?`), the result is `nil`,
// which goja surfaces to JS as `null`.
func jqQueryFirst(_ context.Context, call goja.FunctionCall) (any, error) {
	data, filter, err := parseJqArgs(call)
	if err != nil {
		return nil, err
	}
	results, err := runJqQuery(data, filter, 1)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	return results[0], nil
}

// jqQueryAll drains the iterator and returns every emitted value. Use this
// when the filter explodes a list (`.[]`) or otherwise emits multiple
// results.
func jqQueryAll(_ context.Context, call goja.FunctionCall) ([]any, error) {
	data, filter, err := parseJqArgs(call)
	if err != nil {
		return nil, err
	}
	return runJqQuery(data, filter, 0)
}

func parseJqArgs(call goja.FunctionCall) (any, string, error) {
	dataArg := call.Argument(0)
	if dataArg == nil || goja.IsUndefined(dataArg) {
		return nil, "", errors.New("jq: data is undefined")
	}
	data := dataArg.Export()
	filter := call.Argument(1).String()
	if filter == "" {
		return nil, "", errors.New("jq: filter is empty")
	}
	return data, filter, nil
}

// runJqQuery parses the filter, runs it against data, and returns up to
// `limit` results. limit == 0 means "no limit, drain the iterator".
// gojq emits errors as in-band values inside the result iterator — the
// loop type-asserts and converts the first encountered one into a Go
// error, which goja then surfaces as a JS throw.
func runJqQuery(data any, filter string, limit int) ([]any, error) {
	q, err := gojq.Parse(filter)
	if err != nil {
		return nil, fmt.Errorf("jq: parse %q: %w", filter, err)
	}
	iter := q.Run(normaliseForJq(data))
	var out []any
	for {
		v, ok := iter.Next()
		if !ok {
			break
		}
		if e, isErr := v.(error); isErr {
			return nil, fmt.Errorf("jq: %w", e)
		}
		out = append(out, v)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

// normaliseForJq walks the input tree converting any integer flavour to
// plain `int` and `float32` to `float64`. gojq's runtime panics on
// `int64` (and other sized integer types) because its arithmetic
// dispatch only knows the two canonical numeric types. goja's `.Export()`
// hands us `int64` for any JS-side integer, so without this normalisation
// the most innocuous query (`.users[].age`) blows up.
func normaliseForJq(v any) any {
	switch x := v.(type) {
	case int:
		return x
	case int8:
		return int(x)
	case int16:
		return int(x)
	case int32:
		return int(x)
	case int64:
		return int(x)
	case uint:
		return int(x)
	case uint8:
		return int(x)
	case uint16:
		return int(x)
	case uint32:
		return int(x)
	case uint64:
		return int(x)
	case float32:
		return float64(x)
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normaliseForJq(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normaliseForJq(val)
		}
		return out
	}
	return v
}
