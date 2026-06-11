package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/zalando/go-keyring"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// secrets.go backs runtime.secrets: read/write string credentials in the OS
// keystore (macOS Keychain, Linux Secret Service, Windows Credential Manager)
// via github.com/zalando/go-keyring — pure-Go, no cgo. Every operation is
// confined to a sercon-owned prefix namespace: the keystore service used is
// PREFIX + name, and the script cannot influence PREFIX, so it can neither
// read nor clobber any secret outside the namespace.

// secretsPrefixOverride is set from the --secrets-prefix flag in main.go after
// flag parsing ("" = flag not given). Package-level to mirror the serve-mode
// override pattern (servePortOverride).
var secretsPrefixOverride string

// resolveSecretsPrefix picks the namespace prefix: --secrets-prefix flag, else
// the SERCON_SECRETS_PREFIX env var, else the default "sercon/".
func resolveSecretsPrefix() string {
	if secretsPrefixOverride != "" {
		return secretsPrefixOverride
	}
	if v := os.Getenv("SERCON_SECRETS_PREFIX"); v != "" {
		return v
	}
	return "sercon/"
}

// secretsAvailable is a cheap, side-effect-free advisory hint: does a keystore
// backend plausibly exist on this host? It does NOT touch the keystore (that
// would add a subprocess / D-Bus round-trip — and a possible macOS prompt — to
// every run). The authoritative answer is whether get/set/delete throw.
func secretsAvailable() bool {
	switch runtime.GOOS {
	case "darwin", "windows":
		return true
	case "linux":
		return linuxSecretsAvailable(os.Getenv("DBUS_SESSION_BUS_ADDRESS"), os.Getenv("XDG_RUNTIME_DIR"))
	default:
		return false
	}
}

// linuxSecretsAvailable reports whether a D-Bus session — the transport for the
// Secret Service — is plausibly reachable, from the session-bus address or the
// default session-bus socket under XDG_RUNTIME_DIR. Pure (no globals) so it is
// unit-testable off Linux.
func linuxSecretsAvailable(dbusAddr, runtimeDir string) bool {
	if dbusAddr != "" {
		return true
	}
	if runtimeDir != "" {
		if _, err := os.Stat(filepath.Join(runtimeDir, "bus")); err == nil {
			return true
		}
	}
	return false
}

// secretsOpTimeout bounds a single keystore call. On macOS go-keyring shells
// out to `security`, which can block on a Keychain consent dialog in a
// non-interactive session; the bound makes the op reject cleanly instead of
// hanging the awaiting script. (The op runs in a PromisifyAsync goroutine, so
// this never blocks the event loop; on timeout the inner goroutine is
// abandoned — acceptable for a per-run CLI process.)
const secretsOpTimeout = 10 * time.Second

// runBounded runs fn with secretsOpTimeout, returning a timeout error if it
// overruns.
func runBounded[T any](fn func() (T, error)) (T, error) {
	type res struct {
		v   T
		err error
	}
	ch := make(chan res, 1)
	go func() { v, err := fn(); ch <- res{v, err} }()
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(secretsOpTimeout):
		var zero T
		return zero, fmt.Errorf("timed out after %s (keystore prompt or unreachable backend?)", secretsOpTimeout)
	}
}

// secretArgs validates and extracts (name, account) from a get/delete/set
// call. name must be a present, non-empty string (it forms the keystore
// service together with the prefix); account must be present but may be an
// empty string for a single-secret name. Without this, a missing argument
// would stringify to the literal "undefined" and silently mis-key the
// keystore. The returned error becomes a thrown JS exception (the promise
// rejects). op names the binding for the message (e.g. "get").
func secretArgs(call goja.FunctionCall, op string) (name, account string, err error) {
	nameArg := call.Argument(0)
	if goja.IsUndefined(nameArg) || goja.IsNull(nameArg) || nameArg.String() == "" {
		return "", "", fmt.Errorf("runtime.secrets.%s: name is required (a non-empty string)", op)
	}
	if len(call.Arguments) < 2 || goja.IsUndefined(call.Argument(1)) || goja.IsNull(call.Argument(1)) {
		return "", "", fmt.Errorf("runtime.secrets.%s: account is required (pass \"\" for a single-secret name)", op)
	}
	return nameArg.String(), call.Argument(1).String(), nil
}

// secretsGet returns a work func that reads PREFIX+name / account. Resolves to
// the secret string, or nil (JS null) when the item is absent.
func secretsGet(prefix string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(_ context.Context, call goja.FunctionCall) (any, error) {
		name, account, err := secretArgs(call, "get")
		if err != nil {
			return nil, err
		}
		s, err := runBounded(func() (string, error) { return keyring.Get(prefix+name, account) })
		if errors.Is(err, keyring.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("runtime.secrets.get: %w", err)
		}
		return s, nil
	}
}

// secretsSet returns a work func that stores/overwrites PREFIX+name / account.
func secretsSet(prefix string) func(context.Context, goja.FunctionCall) (any, error) {
	return func(_ context.Context, call goja.FunctionCall) (any, error) {
		name, account, err := secretArgs(call, "set")
		if err != nil {
			return nil, err
		}
		if len(call.Arguments) < 3 || goja.IsUndefined(call.Argument(2)) || goja.IsNull(call.Argument(2)) {
			return nil, fmt.Errorf("runtime.secrets.set: secret is required (a string)")
		}
		secret := call.Argument(2).String()
		_, err = runBounded(func() (struct{}, error) {
			return struct{}{}, keyring.Set(prefix+name, account, secret)
		})
		if err != nil {
			return nil, fmt.Errorf("runtime.secrets.set: %w", err)
		}
		return nil, nil
	}
}

// secretsDelete returns a work func that removes PREFIX+name / account.
// Resolves true when an item was removed, false when there was nothing to
// remove.
func secretsDelete(prefix string) func(context.Context, goja.FunctionCall) (bool, error) {
	return func(_ context.Context, call goja.FunctionCall) (bool, error) {
		name, account, err := secretArgs(call, "delete")
		if err != nil {
			return false, err
		}
		_, err = runBounded(func() (struct{}, error) {
			return struct{}{}, keyring.Delete(prefix+name, account)
		})
		if errors.Is(err, keyring.ErrNotFound) {
			return false, nil
		}
		if err != nil {
			return false, fmt.Errorf("runtime.secrets.delete: %w", err)
		}
		return true, nil
	}
}

// secretsNamespace builds the runtime.secrets member map. Async get/set/delete
// via PromisifyAsync (keystore access is blocking I/O); available is a sync
// advisory bool. Safe for d.ts introspection with (nil, nil): PromisifyAsync
// captures vm/loop without dereferencing them, and the helpers don't touch vm.
func secretsNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	prefix := resolveSecretsPrefix()
	return map[string]any{
		"available": secretsAvailable(),
		"get":       scriptengine.PromisifyAsync(vm, loop, secretsGet(prefix)),
		"set":       scriptengine.PromisifyAsync(vm, loop, secretsSet(prefix)),
		"delete":    scriptengine.PromisifyAsync(vm, loop, secretsDelete(prefix)),
	}
}
