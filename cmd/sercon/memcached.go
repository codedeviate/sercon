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
		"open": scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (string, error) {
				addr := call.Argument(0).String()
				if addr == "" {
					return "", errors.New("memcached.open: addr required (e.g. localhost:11211)")
				}
				return addr, nil
			},
			func(_ context.Context, addr string) (map[string]any, error) {
				return memcachedOpen(vm, loop, addr)
			}),
	}
}

// memcachedSetArgs is the plain-Go carrier for set(key, value, expirySeconds?),
// extracted on the event loop.
type memcachedSetArgs struct {
	key    string
	val    []byte
	expiry int32
}

func memcachedOpen(vm *goja.Runtime, loop *eventloop.EventLoop, addr string) (map[string]any, error) {
	client := memcache.New(addr)

	// keyExtract reads the single key argument (get / delete).
	keyExtract := func(call goja.FunctionCall) (string, error) {
		return call.Argument(0).String(), nil
	}

	return map[string]any{
		// set(key, value, expirySeconds?) — value is stored as bytes.
		// expirySeconds 0 (default) means "never expire".
		"set": scriptengine.PromisifyAsync(vm, loop,
			func(call goja.FunctionCall) (memcachedSetArgs, error) {
				key := call.Argument(0).String()
				if key == "" {
					return memcachedSetArgs{}, errors.New("memcached.set: key required")
				}
				val, err := jsArgToBytes(call.Argument(1))
				if err != nil {
					return memcachedSetArgs{}, fmt.Errorf("memcached.set: value %w", err)
				}
				var expiry int32
				if len(call.Arguments) > 2 {
					expiry = int32(call.Argument(2).ToInteger())
				}
				return memcachedSetArgs{key: key, val: val, expiry: expiry}, nil
			},
			func(_ context.Context, a memcachedSetArgs) (any, error) {
				if err := client.Set(&memcache.Item{Key: a.key, Value: a.val, Expiration: a.expiry}); err != nil {
					return nil, fmt.Errorf("memcached.set: %w", err)
				}
				return nil, nil
			}).Func,
		// get(key) — returns the stored string, or null on a cache miss.
		"get": scriptengine.PromisifyAsync(vm, loop, keyExtract,
			func(_ context.Context, key string) (any, error) {
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
		"delete": scriptengine.PromisifyAsync(vm, loop, keyExtract,
			func(_ context.Context, key string) (any, error) {
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
