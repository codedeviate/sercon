package main

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// googleConfig is the resolved configuration for a cloud.google(...) handle.
// Credentials fields are NEVER logged. Empty fields mean "use ADC / defaults".
type googleConfig struct {
	project         string
	location        string
	credentialsFile string
	credentialsJSON []byte
	quotaProject    string
	scopes          []string
}

// cloudNamespace builds the `cloud` global: one callable per provider. Google
// is the only provider in this cut; aws/azure land in follow-up plans.
func cloudNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"google": func(call goja.FunctionCall) goja.Value {
			cfg, err := parseGoogleConfig(vm, call)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(googleHandle(vm, loop, cfg))
		},
	}
}

// parseGoogleConfig reads the optional first-argument options object. Runs on
// the event loop (inside the host call), so touching goja values is safe here.
func parseGoogleConfig(vm *goja.Runtime, call goja.FunctionCall) (googleConfig, error) {
	var cfg googleConfig
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return cfg, nil
	}
	obj, ok := arg.(*goja.Object)
	if !ok {
		return cfg, errors.New("cloud.google: options must be an object")
	}
	opts, ok := obj.Export().(map[string]any)
	if !ok {
		return cfg, errors.New("cloud.google: options must be an object")
	}
	cfg.project = optString(opts, "project", "")
	cfg.location = optString(opts, "location", "")
	cfg.quotaProject = optString(opts, "quotaProject", "")
	cfg.scopes = optStringSlice(opts, "scopes")
	// credentials: a string is a path; an object is inline SA JSON.
	if raw, present := opts["credentials"]; present && raw != nil {
		switch v := raw.(type) {
		case string:
			cfg.credentialsFile = v
		case map[string]any:
			b, err := json.Marshal(v)
			if err != nil {
				return cfg, fmt.Errorf("cloud.google: credentials object is not serialisable: %w", err)
			}
			cfg.credentialsJSON = b
		default:
			return cfg, errors.New("cloud.google: credentials must be a file path (string) or an inline object")
		}
	}
	return cfg, nil
}

// googleHandle is a TEMPORARY stub: service accessors are no-ops until Task 4
// implements the real Google Cloud service surface.
func googleHandle(vm *goja.Runtime, loop *eventloop.EventLoop, cfg googleConfig) map[string]any {
	noop := func(goja.FunctionCall) goja.Value { return goja.Undefined() }
	return map[string]any{
		"storage": noop, "compute": noop, "iam": noop, "secrets": noop, "call": noop,
	}
}
