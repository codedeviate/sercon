package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// authSaveStringOpts are auth-save options that take a value, in stable order.
// Password is intentionally NOT here — it is fed via --password-stdin.
var authSaveStringOpts = []struct{ key, flag string }{
	{"url", "--url"},
	{"username", "--username"},
	{"usernameSelector", "--username-selector"},
	{"passwordSelector", "--password-selector"},
	{"submitSelector", "--submit-selector"},
}

// authSaveArgs builds `auth save <name> [string-opts...] --password-stdin`.
// The password is never placed in argv; it is written to stdin by abRunStdin.
func authSaveArgs(name string, opts map[string]any) []string {
	args := []string{"auth", "save", name}
	for _, o := range authSaveStringOpts {
		if v, ok := opts[o.key]; ok {
			if s := fmt.Sprintf("%v", v); s != "" {
				args = append(args, o.flag, s)
			}
		}
	}
	return append(args, "--password-stdin")
}

// authSaveParams carries the on-loop-extracted auth-save arguments to the
// work goroutine. (Not <binding>Args: authSaveArgs is the argv builder above.)
type authSaveParams struct {
	name     string
	opts     map[string]any
	password string
}

// authSaveExtract validates the auth-save arguments on the event loop:
// requires a profile name plus url, username and password in opts. The
// availability check runs first, preserving the original error precedence.
func authSaveExtract(call goja.FunctionCall) (authSaveParams, error) {
	if !abAvailable() {
		return authSaveParams{}, errors.New("agent-browser CLI not found on PATH; install it to use services.agentBrowser")
	}
	name := strArg(call, 0)
	opts := optsArgMap(call, 1)
	password, _ := opts["password"].(string)
	if name == "" {
		return authSaveParams{}, errors.New("agentBrowser.auth.save: a profile name is required")
	}
	if u, _ := opts["url"].(string); u == "" {
		return authSaveParams{}, errors.New("agentBrowser.auth.save: opts.url is required")
	}
	if un, _ := opts["username"].(string); un == "" {
		return authSaveParams{}, errors.New("agentBrowser.auth.save: opts.username is required")
	}
	if password == "" {
		return authSaveParams{}, errors.New("agentBrowser.auth.save: opts.password is required")
	}
	return authSaveParams{name: name, opts: opts, password: password}, nil
}

// authSave (namespace-level): saves a login profile. Password is fed via
// stdin. Session-independent (vault is global).
func authSave(ctx context.Context, a authSaveParams) (any, error) {
	stdout, stderr, code, err := abRunStdin(ctx, "", nil, abDefaultCallTimeout, a.password+"\n", authSaveArgs(a.name, a.opts)...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		msg := stderr
		if msg == "" {
			msg = stdout
		}
		return nil, fmt.Errorf("agent-browser auth save: exited %d: %s", code, msg)
	}
	return parseJSON(stdout)
}

// authSimple (namespace-level): list / show <name> / delete <name>. The
// profile name (when required) is extracted and validated on the event loop;
// the CLI call runs in the work goroutine.
func authSimple(vm *goja.Runtime, loop *eventloop.EventLoop, sub string, needsName bool) func(goja.FunctionCall) goja.Value {
	return scriptengine.PromisifyAsync(vm, loop,
		func(call goja.FunctionCall) (string, error) {
			if !needsName {
				return "", nil
			}
			name := strArg(call, 0)
			if name == "" {
				return "", fmt.Errorf("agentBrowser.auth.%s: a profile name is required", sub)
			}
			return name, nil
		},
		func(ctx context.Context, name string) (any, error) {
			args := []string{"auth", sub}
			if needsName {
				args = append(args, name)
			}
			out, err := abRunChecked(ctx, "", nil, abDefaultCallTimeout, args...)
			if err != nil {
				return nil, err
			}
			return parseJSON(out)
		}).Func
}

// authNamespace builds services.agentBrowser.auth.* (vault CRUD — session
// independent). login is NOT here; it is a handle method (acts on a session).
func authNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"save":   scriptengine.PromisifyAsync(vm, loop, authSaveExtract, authSave).Func,
		"list":   authSimple(vm, loop, "list", false),
		"show":   authSimple(vm, loop, "show", true),
		"delete": authSimple(vm, loop, "delete", true),
	}
}

// authLoginExtract pulls and validates the profile name on the event loop.
func authLoginExtract(call goja.FunctionCall) (string, error) {
	name := strArg(call, 0)
	if name == "" {
		return "", errors.New("agentBrowser.auth.login: a profile name is required")
	}
	return name, nil
}

// authLogin (handle-level): logs in on this handle's session using a saved
// profile.
func (h *abHandle) authLogin(ctx context.Context, name string) (any, error) {
	return h.runJSON(ctx, "auth", "login", name)
}

// addAuthLogin wires the handle-level auth.login into the handle object.
func (h *abHandle) addAuthLogin(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["auth"] = map[string]any{
		"login": scriptengine.PromisifyAsync(vm, loop, authLoginExtract, h.authLogin).Func,
	}
}
