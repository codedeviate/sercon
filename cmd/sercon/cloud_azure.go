package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
)

// azureConfig is the resolved config for a cloud.azure(...) handle. clientSecret
// and any token material are NEVER logged.
type azureConfig struct {
	subscriptionID, tenantID, clientID, clientSecret string
}

// azureTestSeam (tests only) supplies a stub token credential + a custom
// transport so clients hit an httptest server without real Azure auth. nil in
// production.
type azureTestSeam struct {
	transport policy.Transporter
	cred      azcore.TokenCredential
}

var azureTestOptions *azureTestSeam

// parseAzureConfig reads the optional first-argument options object. Runs on
// the event loop (inside the host call), so touching goja values is safe here.
// Type assertions are guarded: a JS array or other non-object value must
// produce a catchable error, not a panic (Phase 1/2 lesson).
func parseAzureConfig(vm *goja.Runtime, call goja.FunctionCall) (azureConfig, error) {
	var cfg azureConfig
	arg := call.Argument(0)
	if goja.IsUndefined(arg) || goja.IsNull(arg) {
		return cfg, nil
	}
	obj, ok := arg.(*goja.Object)
	if !ok {
		return cfg, errors.New("cloud.azure: options must be an object")
	}
	opts, ok := obj.Export().(map[string]any)
	if !ok {
		return cfg, errors.New("cloud.azure: options must be an object")
	}
	cfg.subscriptionID = optString(opts, "subscriptionId", "")
	cfg.tenantID = optString(opts, "tenantId", "")
	cfg.clientID = optString(opts, "clientId", "")
	cfg.clientSecret = optString(opts, "clientSecret", "")
	return cfg, nil
}

// credential resolves the token credential: the test stub when the seam is
// active, else an explicit client-secret credential, else the default chain
// (env / managed identity / az login).
func (c azureConfig) credential() (azcore.TokenCredential, error) {
	if azureTestOptions != nil {
		return azureTestOptions.cred, nil
	}
	if c.tenantID != "" && c.clientID != "" && c.clientSecret != "" {
		cred, err := azidentity.NewClientSecretCredential(c.tenantID, c.clientID, c.clientSecret, nil)
		if err != nil {
			return nil, mapAzureError(err)
		}
		return cred, nil
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, mapAzureError(err)
	}
	return cred, nil
}

// subscription resolves the subscription ID: explicit config, else
// AZURE_SUBSCRIPTION_ID, else an error (ARM services + the call escape hatch
// all call this).
func (c azureConfig) subscription() (string, error) {
	if c.subscriptionID != "" {
		return c.subscriptionID, nil
	}
	if s := os.Getenv("AZURE_SUBSCRIPTION_ID"); s != "" {
		return s, nil
	}
	return "", errors.New("cloud.azure: subscriptionId is required (set it in cloud.azure({subscriptionId}) or AZURE_SUBSCRIPTION_ID)")
}

// armClientOptions injects the test transport when the seam is active, else
// nil (SDK defaults).
func (c azureConfig) armClientOptions() *arm.ClientOptions {
	if azureTestOptions != nil && azureTestOptions.transport != nil {
		return &arm.ClientOptions{ClientOptions: azcore.ClientOptions{Transport: azureTestOptions.transport}}
	}
	return nil
}

// String renders a redacted summary; clientSecret is NEVER included.
func (c azureConfig) String() string {
	auth := "default-chain"
	if c.clientSecret != "" {
		auth = "client-secret(redacted)"
	}
	return fmt.Sprintf("azureConfig{subscription:%q tenant:%q client:%q auth:%s}", c.subscriptionID, c.tenantID, c.clientID, auth)
}

// azureHandle builds the object returned by cloud.azure(...): one accessor
// per service namespace, plus the generic call escape hatch. Temporary stubs
// — real implementations land in Tasks 3-8.
func azureHandle(vm *goja.Runtime, loop *eventloop.EventLoop, cfg azureConfig) map[string]any {
	noop := func(goja.FunctionCall) goja.Value { return goja.Undefined() }
	return map[string]any{
		"resourceGroups":  noop,
		"compute":         noop,
		"resources":       noop,
		"blob":            noop,
		"keyvaultSecrets": noop,
		"call":            noop,
	}
}

// mapAzureError is a temporary passthrough shim; replaced with real ARM
// error-mapping in Task 2.
func mapAzureError(err error) error { return err }
