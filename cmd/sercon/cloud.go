package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"

	"github.com/codedeviate/sercon/pkg/scriptengine"
)

// googleConfig is the resolved configuration for a cloud.google(...) handle.
// Credentials fields are NEVER logged. Empty fields mean "use ADC / defaults".
type googleConfig struct {
	project         string
	credentialsFile string
	credentialsJSON []byte
	quotaProject    string
	scopes          []string
}

// cloudNamespace builds the `cloud` global: one callable per provider.
// Google and AWS are wired up in this cut; azure lands in a follow-up plan.
func cloudNamespace(vm *goja.Runtime, loop *eventloop.EventLoop) map[string]any {
	return map[string]any{
		"google": func(call goja.FunctionCall) goja.Value {
			cfg, err := parseGoogleConfig(vm, call)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(googleHandle(vm, loop, cfg))
		},
		"aws": func(call goja.FunctionCall) goja.Value {
			cfg, err := parseAWSConfig(vm, call)
			if err != nil {
				panic(vm.NewGoError(err))
			}
			return vm.ToValue(awsHandle(vm, loop, cfg))
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

// googleHandle builds the object returned by cloud.google(...): one accessor
// per service namespace, plus the generic path-based REST escape hatch
// `call`. storage/compute/iam/secrets are all real per Tasks 5-8 (see
// cloud_google_storage.go, cloud_google_compute.go, cloud_google_iam.go, and
// cloud_google_secrets.go).
//
// This map is built at script-run time (inside the cloud.google(...) call),
// past the engine's registration-time AsyncBinding unwrap — so `call`'s
// `.Func` must be unwrapped explicitly here (see the sqlHandle note in
// db_sql.go for the same pattern).
func googleHandle(vm *goja.Runtime, loop *eventloop.EventLoop, cfg googleConfig) map[string]any {
	return map[string]any{
		"storage": func(goja.FunctionCall) goja.Value { return vm.ToValue(googleStorage(vm, loop, cfg)) },
		"compute": func(goja.FunctionCall) goja.Value { return vm.ToValue(googleCompute(vm, loop, cfg)) },
		"iam":     func(goja.FunctionCall) goja.Value { return vm.ToValue(googleIAM(vm, loop, cfg)) },
		"secrets": func(goja.FunctionCall) goja.Value { return vm.ToValue(googleSecrets(vm, loop, cfg)) },
		"call": scriptengine.PromisifyAsync(vm, loop, googleCallExtract(cfg),
			func(ctx context.Context, a googleCallArgs) (any, error) { return googleCallWork(ctx, cfg, a) }).Func,
	}
}
