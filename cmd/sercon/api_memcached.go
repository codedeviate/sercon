package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/bradfitz/gomemcache/memcache"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// memcachedNamespace wires `db.memcached.*`. `open(addr)` connects
// to a memcached server (`host:port`) and returns a stateful handle
// `{ get, set, delete }`. gomemcache pools connections lazily, so
// there's no PING-on-open (the first operation surfaces a bad
// address) and no close (the pool is GC'd with the handle).
//
// Library: github.com/bradfitz/gomemcache/memcache (the de facto
// standard pure-Go client).
func memcachedNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (map[string]any, error) {
			return memcachedOpen(vm, loop, call)
		}),
	}
}

func memcachedOpen(vm *goja.Runtime, loop *eventloop.EventLoop, call goja.FunctionCall) (map[string]any, error) {
	addr := call.Argument(0).String()
	if addr == "" {
		return nil, errors.New("memcached.open: addr required (e.g. localhost:11211)")
	}
	client := memcache.New(addr)

	return map[string]any{
		// set(key, value, expirySeconds?) — value is stored as bytes.
		// expirySeconds 0 (default) means "never expire".
		"set": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			key := call.Argument(0).String()
			if key == "" {
				return nil, errors.New("memcached.set: key required")
			}
			val, err := jsArgToBytes(call.Argument(1))
			if err != nil {
				return nil, fmt.Errorf("memcached.set: value %w", err)
			}
			var expiry int32
			if len(call.Arguments) > 2 {
				expiry = int32(call.Argument(2).ToInteger())
			}
			if err := client.Set(&memcache.Item{Key: key, Value: val, Expiration: expiry}); err != nil {
				return nil, fmt.Errorf("memcached.set: %w", err)
			}
			return nil, nil
		}).Func,
		// get(key) — returns the stored string, or null on a cache miss.
		"get": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			key := call.Argument(0).String()
			item, err := client.Get(key)
			if err != nil {
				if errors.Is(err, memcache.ErrCacheMiss) {
					return nil, nil // miss is data, not an error
				}
				return nil, fmt.Errorf("memcached.get: %w", err)
			}
			return string(item.Value), nil
		}).Func,
		// delete(key) — returns true if the key existed, false on miss.
		"delete": scriptengine.PromisifyAsync(vm, loop, func(_ context.Context, call goja.FunctionCall) (any, error) {
			key := call.Argument(0).String()
			err := client.Delete(key)
			if err != nil {
				if errors.Is(err, memcache.ErrCacheMiss) {
					return false, nil
				}
				return nil, fmt.Errorf("memcached.delete: %w", err)
			}
			return true, nil
		}).Func,
	}, nil
}
