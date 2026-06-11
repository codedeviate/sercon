package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/redis/go-redis/v9"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// redisNamespace wires `db.redis.*`. `open(url)` connects (a
// standard `redis://[:password@]host:port/db` URL) and PINGs to
// surface a bad address up front, then returns a stateful handle
// `{ do, ping, close }`. `do` runs an arbitrary command, so the
// binding stays small while covering the whole RESP surface —
// scripts compose GET / SET / HGETALL / whatever rather than the
// binding mirroring hundreds of methods.
//
// Library: github.com/redis/go-redis/v9 (the official client, pure
// Go). Offline-testable via alicebob/miniredis in the test suite.
//
// The connection + command machinery is shared with `db.valkey`
// (Valkey is the RESP-compatible Redis fork) via respOpen/respDo,
// parameterised by a `label` used in error messages.
func redisNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return respOpen(vm, loop, ctx, call, "redis", nil)
		}),
	}
}

// respOpen connects a go-redis client from a connection URL and PINGs it. label
// ("redis" / "valkey") prefixes the error messages; normalize, if non-nil,
// rewrites the URL into a scheme go-redis understands before parsing (Valkey
// uses this to accept valkey:// / valkeys://).
func respOpen(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, call goja.FunctionCall, label string, normalize func(string) string) (map[string]any, error) {
	url := call.Argument(0).String()
	if url == "" {
		return nil, fmt.Errorf("%s.open: url required (e.g. %s://localhost:6379/0)", label, label)
	}
	if normalize != nil {
		url = normalize(url)
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("%s.open: parse url: %w", label, err)
	}
	client := redis.NewClient(opt)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("%s.open: ping: %w", label, err)
	}
	return respHandle(vm, loop, client, label), nil
}

// respHandle builds the stateful { do, ping, close } handle over a connected
// client, with error messages prefixed by label.
func respHandle(vm *goja.Runtime, loop *eventloop.EventLoop, client *redis.Client, label string) map[string]any {
	return map[string]any{
		"do": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			return respDo(ctx, client, call, label)
		}).Func,
		"ping": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			res, err := client.Ping(ctx).Result()
			if err != nil {
				return nil, fmt.Errorf("%s.ping: %w", label, err)
			}
			return res, nil
		}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := client.Close(); err != nil {
				return nil, fmt.Errorf("%s.close: %w", label, err)
			}
			return nil, nil
		}).Func,
	}
}

// respDo runs an arbitrary command: `do("SET", "k", "v")`,
// `do("GET", "k")`, `do("HGETALL", "h")`, etc. The first arg is the
// command name; the rest are its arguments. go-redis returns the
// RESP reply as `any` — strings, int64, []any, nil — which goja
// exports to JS the obvious way. RESP-level errors (WRONGTYPE,
// unknown command) surface as thrown JS errors; a nil reply (missing
// key) is data, returned as JS null. label prefixes error messages.
func respDo(ctx context.Context, client *redis.Client, call goja.FunctionCall, label string) (any, error) {
	if len(call.Arguments) < 1 {
		return nil, fmt.Errorf("%s.do: command name required", label)
	}
	args := make([]any, 0, len(call.Arguments))
	for _, a := range call.Arguments {
		if a == nil || goja.IsUndefined(a) || goja.IsNull(a) {
			args = append(args, nil)
			continue
		}
		args = append(args, a.Export())
	}
	res, err := client.Do(ctx, args...).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			// A nil reply (key missing) is data, not an error — return
			// JS null so `await r.do("GET", "missing")` is `null`.
			return nil, nil
		}
		return nil, fmt.Errorf("%s.do: %w", label, err)
	}
	return redisCoerce(res), nil
}

// redisCoerce normalises go-redis reply values for goja. The client
// hands back []byte for bulk strings in some paths; promote those to
// string (RESP values are usually text). Nested slices recurse.
func redisCoerce(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = redisCoerce(e)
		}
		return out
	default:
		return v
	}
}
