package main

import (
	"context"
	"strings"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// valkeyNamespace wires `db.valkey.*`. Valkey is the RESP-compatible fork of
// Redis, so it reuses the exact same go-redis client and command machinery as
// `db.redis` (respOpen/respDo) — only the error label and the accepted URL
// schemes differ. `open(url)` connects and PINGs, then returns the same
// stateful handle `{ do, ping, close }`.
//
// In addition to redis:// / rediss://, it accepts the Valkey-idiomatic
// valkey:// / valkeys:// schemes, normalised to the redis ones go-redis's
// ParseURL understands (the wire protocol is identical).
func valkeyNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"open": scriptengine.PromisifyAsyncLegacy(vm, loop, func(ctx context.Context, call goja.FunctionCall) (map[string]any, error) {
			return respOpen(vm, loop, ctx, call, "valkey", normalizeValkeyURL)
		}),
	}
}

// normalizeValkeyURL rewrites a Valkey-scheme URL into the redis-scheme URL
// that go-redis's ParseURL accepts. valkey:// → redis://, valkeys:// →
// rediss:// (TLS). redis:// / rediss:// and anything else pass through
// unchanged (ParseURL then reports a clear error for genuinely bad input).
func normalizeValkeyURL(u string) string {
	switch {
	case strings.HasPrefix(u, "valkeys://"):
		return "rediss://" + strings.TrimPrefix(u, "valkeys://")
	case strings.HasPrefix(u, "valkey://"):
		return "redis://" + strings.TrimPrefix(u, "valkey://")
	default:
		return u
	}
}
