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

// authSave (namespace-level): saves a login profile. Requires url, username,
// password. Password is fed via stdin. Session-independent (vault is global).
func authSave() func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		if !abAvailable() {
			return nil, errors.New("agent-browser CLI not found on PATH; install it to use services.agentBrowser")
		}
		name := strArg(call, 0)
		opts := optsArgMap(call, 1)
		password, _ := opts["password"].(string)
		if name == "" {
			return nil, errors.New("agentBrowser.auth.save: a profile name is required")
		}
		if u, _ := opts["url"].(string); u == "" {
			return nil, errors.New("agentBrowser.auth.save: opts.url is required")
		}
		if un, _ := opts["username"].(string); un == "" {
			return nil, errors.New("agentBrowser.auth.save: opts.username is required")
		}
		if password == "" {
			return nil, errors.New("agentBrowser.auth.save: opts.password is required")
		}
		stdout, stderr, code, err := abRunStdin(ctx, "", nil, abDefaultCallTimeout, password+"\n", authSaveArgs(name, opts)...)
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
}

// authSimple (namespace-level): list / show <name> / delete <name>.
func authSimple(sub string, needsName bool) func(context.Context, goja.FunctionCall) (any, error) {
	return func(ctx context.Context, call goja.FunctionCall) (any, error) {
		args := []string{"auth", sub}
		if needsName {
			name := strArg(call, 0)
			if name == "" {
				return nil, fmt.Errorf("agentBrowser.auth.%s: a profile name is required", sub)
			}
			args = append(args, name)
		}
		out, err := abRunChecked(ctx, "", nil, abDefaultCallTimeout, args...)
		if err != nil {
			return nil, err
		}
		return parseJSON(out)
	}
}

// authNamespace builds services.agentBrowser.auth.* (vault CRUD — session
// independent). login is NOT here; it is a handle method (acts on a session).
func authNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"save":   scriptengine.PromisifyAsync(vm, loop, authSave()).Func,
		"list":   scriptengine.PromisifyAsync(vm, loop, authSimple("list", false)).Func,
		"show":   scriptengine.PromisifyAsync(vm, loop, authSimple("show", true)).Func,
		"delete": scriptengine.PromisifyAsync(vm, loop, authSimple("delete", true)).Func,
	}
}

// authLogin (handle-level): logs in on this handle's session using a saved
// profile.
func (h *abHandle) authLogin(ctx context.Context, call goja.FunctionCall) (any, error) {
	name := strArg(call, 0)
	if name == "" {
		return nil, errors.New("agentBrowser.auth.login: a profile name is required")
	}
	return h.runJSON(ctx, "auth", "login", name)
}

// addAuthLogin wires the handle-level auth.login into the handle object.
func (h *abHandle) addAuthLogin(obj map[string]any, vm *goja.Runtime, loop *eventloop.EventLoop) {
	obj["auth"] = map[string]any{
		"login": h.p(vm, loop, h.authLogin),
	}
}
