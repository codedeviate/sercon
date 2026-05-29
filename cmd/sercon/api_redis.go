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
func redisNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return redisOpen(vm, loop, ctx, call)
		}),
	}
}

func redisOpen(vm *goja.Runtime, loop *eventloop.EventLoop, ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
	url := call.Argument(0).String()
	if url == "" {
		return nil, errors.New("redis.open: url required (e.g. redis://localhost:6379/0)")
	}
	opt, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis.open: parse url: %w", err)
	}
	client := redis.NewClient(opt)
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis.open: ping: %w", err)
	}

	return map[string]any{
		"do": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			return redisDo(ctx, client, call)
		}).Func,
		"ping": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			res, err := client.Ping(ctx).Result()
			if err != nil {
				return nil, fmt.Errorf("redis.ping: %w", err)
			}
			return res, nil
		}).Func,
		"close": scriptengine.PromisifyAsync(vm, loop, func(ctx context.Context, call goja.FunctionCall) (any, error) {
			if err := client.Close(); err != nil {
				return nil, fmt.Errorf("redis.close: %w", err)
			}
			return nil, nil
		}).Func,
	}, nil
}

// redisDo runs an arbitrary command: `do("SET", "k", "v")`,
// `do("GET", "k")`, `do("HGETALL", "h")`, etc. The first arg is the
// command name; the rest are its arguments. go-redis returns the
// RESP reply as `any` — strings, int64, []any, nil — which goja
// exports to JS the obvious way. Redis-level errors (WRONGTYPE,
// unknown command) surface as thrown JS errors.
func redisDo(ctx context.Context, client *redis.Client, call goja.FunctionCall) (any, error) {
	if len(call.Arguments) < 1 {
		return nil, errors.New("redis.do: command name required")
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
		return nil, fmt.Errorf("redis.do: %w", err)
	}
	return redisCoerce(res), nil
}

// redisCoerce normalises go-redis reply values for goja. The client
// hands back []byte for bulk strings in some paths; promote those to
// string (Redis values are usually text). Nested slices recurse.
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
